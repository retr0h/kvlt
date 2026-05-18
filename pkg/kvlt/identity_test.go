// Copyright (c) 2026 John Dewey
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING
// FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER
// DEALINGS IN THE SOFTWARE.

package kvlt

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"filippo.io/age"
	"golang.org/x/crypto/ssh"
)

// generateSSHKeyPair returns an OpenSSH-formatted ed25519 private key
// (optionally encrypted with the given passphrase) and the matching
// authorized_keys-format public key line. Tests use this instead of
// shelling out to ssh-keygen so they're hermetic — no host SSH state,
// no spawned processes, no race against /dev/random.
func generateSSHKeyPair(t *testing.T, passphrase []byte) (privPEM []byte, authorizedKey []byte) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("ed25519.GenerateKey: %v", err)
	}

	const comment = "kvlt-test"
	var block *pem.Block
	if len(passphrase) > 0 {
		block, err = ssh.MarshalPrivateKeyWithPassphrase(priv, comment, passphrase)
	} else {
		block, err = ssh.MarshalPrivateKey(priv, comment)
	}
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	privPEM = pem.EncodeToMemory(block)

	sshPub, err := ssh.NewPublicKey(pub)
	if err != nil {
		t.Fatalf("ssh.NewPublicKey: %v", err)
	}
	authorizedKey = ssh.MarshalAuthorizedKey(sshPub)
	return privPEM, authorizedKey
}

func writeKeyToTempFile(t *testing.T, data []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "key")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatalf("write temp key: %v", err)
	}
	return path
}

// roundTripWithIdentity encrypts a known plaintext to recipients and
// confirms it decrypts with the given identities. Used to assert
// that the SSH-loading helpers return an Identity that actually
// pairs with the matching public-key recipient — a parsing bug
// would surface as a wrong-key decrypt failure rather than the
// silent "looks fine but wrong" failure mode.
func roundTripWithIdentity(t *testing.T, recipients []age.Recipient, identities []age.Identity) {
	t.Helper()
	const want = "round-trip-payload"

	dir := filepath.Join(t.TempDir(), "v")
	resolver := func() ([]age.Identity, error) { return identities, nil }
	p, err := NewLocalProvider("rt", dir, recipients, resolver)
	if err != nil {
		t.Fatalf("NewLocalProvider: %v", err)
	}
	if err := p.Put(context.Background(), "k", want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := p.Get(context.Background(), "k")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("Get returned %q, want %q", got, want)
	}
}

func TestParseSSHRecipients(t *testing.T) {
	t.Parallel()
	_, validPub := generateSSHKeyPair(t, nil)
	withDecorations := bytes.Join([][]byte{
		[]byte("# leading comment"),
		[]byte(""),
		bytes.TrimRight(validPub, "\n"),
		[]byte("   "),
		[]byte("# trailing"),
	}, []byte("\n"))

	cases := []struct {
		name      string
		input     []byte
		wantCount int
		wantErr   error
	}{
		{
			name:      "single valid line parses to one recipient",
			input:     validPub,
			wantCount: 1,
			wantErr:   nil,
		},
		{
			name:      "blank lines and # comments are skipped",
			input:     withDecorations,
			wantCount: 1,
			wantErr:   nil,
		},
		{
			name:    "all-comment input returns ErrInvalidConfig",
			input:   []byte("# only a comment\n\n"),
			wantErr: ErrInvalidConfig,
		},
		{
			name:    "malformed line returns ErrInvalidConfig",
			input:   []byte("not-an-ssh-key\n"),
			wantErr: ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec, err := ParseSSHRecipients(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseSSHRecipients: %v", err)
			}
			if len(rec) != tc.wantCount {
				t.Fatalf("recipients: got %d, want %d", len(rec), tc.wantCount)
			}
		})
	}
}

func TestLoadSSHIdentity(t *testing.T) {
	t.Parallel()
	const passphrase = "correct horse battery staple"
	plainPriv, plainPub := generateSSHKeyPair(t, nil)
	encPriv, encPub := generateSSHKeyPair(t, []byte(passphrase))

	cases := []struct {
		name    string
		setup   func(t *testing.T) (keyPath string, prompt PassphrasePrompt, pub []byte)
		wantErr error
	}{
		{
			name: "unencrypted key loads and round-trips",
			setup: func(t *testing.T) (string, PassphrasePrompt, []byte) {
				t.Helper()
				return writeKeyToTempFile(t, plainPriv), nil, plainPub
			},
		},
		{
			name: "encrypted key with correct prompt loads and round-trips",
			setup: func(t *testing.T) (string, PassphrasePrompt, []byte) {
				t.Helper()
				prompt := func(_ string) ([]byte, error) { return []byte(passphrase), nil }
				return writeKeyToTempFile(t, encPriv), prompt, encPub
			},
		},
		{
			name: "encrypted key with nil prompt returns ErrAuthFailed",
			setup: func(t *testing.T) (string, PassphrasePrompt, []byte) {
				t.Helper()
				return writeKeyToTempFile(t, encPriv), nil, nil
			},
			wantErr: ErrAuthFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, prompt, pub := tc.setup(t)
			id, err := LoadSSHIdentity(path, prompt)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadSSHIdentity: %v", err)
			}
			rec, err := ParseSSHRecipients(pub)
			if err != nil {
				t.Fatalf("ParseSSHRecipients: %v", err)
			}
			roundTripWithIdentity(t, rec, []age.Identity{id})
		})
	}
}

func TestLoadSSHIdentities(t *testing.T) {
	t.Parallel()
	priv, _ := generateSSHKeyPair(t, nil)

	cases := []struct {
		name      string
		setup     func(t *testing.T) []string
		wantCount int
		wantErr   bool
	}{
		{
			name: "missing files are skipped, present file loads",
			setup: func(t *testing.T) []string {
				t.Helper()
				present := writeKeyToTempFile(t, priv)
				missing := filepath.Join(filepath.Dir(present), "absent_key")
				return []string{missing, present}
			},
			wantCount: 1,
		},
		{
			name:      "all-missing input returns empty slice without error",
			setup:     func(_ *testing.T) []string { return []string{"/nonexistent/a", "/nonexistent/b"} },
			wantCount: 0,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ids, err := LoadSSHIdentities(tc.setup(t), nil)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err: got %v, wantErr=%v", err, tc.wantErr)
			}
			if len(ids) != tc.wantCount {
				t.Fatalf("count: got %d, want %d", len(ids), tc.wantCount)
			}
		})
	}
}

func TestDefaultSSHKeyPaths(t *testing.T) {
	// Cannot t.Parallel: cases mutate process-wide HOME.
	cases := []struct {
		name      string
		setup     func(t *testing.T)
		wantNames []string // basenames of expected paths, in order
	}{
		{
			name:      "no ~/.ssh dir returns empty",
			setup:     func(t *testing.T) { t.Helper(); t.Setenv("HOME", t.TempDir()) },
			wantNames: nil,
		},
		{
			name: "only id_ed25519 present returns just that path",
			setup: func(t *testing.T) {
				t.Helper()
				home := t.TempDir()
				t.Setenv("HOME", home)
				sshDir := filepath.Join(home, ".ssh")
				if err := os.MkdirAll(sshDir, 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), []byte("x"), 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
			},
			wantNames: []string{"id_ed25519"},
		},
		{
			name: "all three key types present preserved in priority order",
			setup: func(t *testing.T) {
				t.Helper()
				home := t.TempDir()
				t.Setenv("HOME", home)
				sshDir := filepath.Join(home, ".ssh")
				if err := os.MkdirAll(sshDir, 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				for _, n := range []string{"id_ed25519", "id_ecdsa", "id_rsa"} {
					if err := os.WriteFile(filepath.Join(sshDir, n), []byte("x"), 0o600); err != nil {
						t.Fatalf("write %s: %v", n, err)
					}
				}
			},
			wantNames: []string{"id_ed25519", "id_ecdsa", "id_rsa"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tc.setup(t)
			got := DefaultSSHKeyPaths()
			if len(got) != len(tc.wantNames) {
				t.Fatalf("count: got %v, want names %v", got, tc.wantNames)
			}
			for i, name := range tc.wantNames {
				if filepath.Base(got[i]) != name {
					t.Fatalf("got[%d] basename = %q, want %q", i, filepath.Base(got[i]), name)
				}
			}
		})
	}
}

func TestDefaultIdentityResolver(t *testing.T) {
	// Cannot t.Parallel: subtests mutate HOME.
	priv, pub := generateSSHKeyPair(t, nil)
	cases := []struct {
		name    string
		setup   func(t *testing.T) (pub []byte)
		wantErr error
	}{
		{
			name: "loads ed25519 key from ~/.ssh and round-trips",
			setup: func(t *testing.T) []byte {
				t.Helper()
				home := t.TempDir()
				t.Setenv("HOME", home)
				sshDir := filepath.Join(home, ".ssh")
				if err := os.MkdirAll(sshDir, 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(sshDir, "id_ed25519"), priv, 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
				return pub
			},
		},
		{
			name: "no keys in ~/.ssh returns ErrAuthFailed",
			setup: func(t *testing.T) []byte {
				t.Helper()
				t.Setenv("HOME", t.TempDir())
				return nil
			},
			wantErr: ErrAuthFailed,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			pubKey := tc.setup(t)
			ids, err := DefaultIdentityResolver(nil)()
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolver: %v", err)
			}
			rec, err := ParseSSHRecipients(pubKey)
			if err != nil {
				t.Fatalf("ParseSSHRecipients: %v", err)
			}
			roundTripWithIdentity(t, rec, ids)
		})
	}
}
