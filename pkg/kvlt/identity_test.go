// Copyright (c) 2026 John Dewey

// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:

// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.

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

func TestParseSSHRecipients_ValidLine(t *testing.T) {
	t.Parallel()
	_, pub := generateSSHKeyPair(t, nil)

	rec, err := ParseSSHRecipients(pub)
	if err != nil {
		t.Fatalf("ParseSSHRecipients: %v", err)
	}
	if len(rec) != 1 {
		t.Fatalf("got %d recipients, want 1", len(rec))
	}
}

func TestParseSSHRecipients_SkipsBlankAndComments(t *testing.T) {
	t.Parallel()
	_, pub := generateSSHKeyPair(t, nil)

	input := bytes.Join([][]byte{
		[]byte("# leading comment"),
		[]byte(""),
		bytes.TrimRight(pub, "\n"),
		[]byte("   "),
		[]byte("# trailing"),
	}, []byte("\n"))

	rec, err := ParseSSHRecipients(input)
	if err != nil {
		t.Fatalf("ParseSSHRecipients: %v", err)
	}
	if len(rec) != 1 {
		t.Fatalf("got %d recipients, want 1 (blank/comment lines should be filtered)", len(rec))
	}
}

func TestParseSSHRecipients_RejectsEmptyInput(t *testing.T) {
	t.Parallel()
	_, err := ParseSSHRecipients([]byte("# only a comment\n\n"))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ParseSSHRecipients with no recipients: got %v, want ErrInvalidConfig", err)
	}
}

func TestParseSSHRecipients_RejectsMalformed(t *testing.T) {
	t.Parallel()
	_, err := ParseSSHRecipients([]byte("not-an-ssh-key\n"))
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("ParseSSHRecipients with malformed line: got %v, want ErrInvalidConfig", err)
	}
}

func TestLoadSSHIdentity_UnencryptedKeyRoundTrip(t *testing.T) {
	t.Parallel()
	priv, pub := generateSSHKeyPair(t, nil)
	keyPath := writeKeyToTempFile(t, priv)

	id, err := LoadSSHIdentity(keyPath, nil)
	if err != nil {
		t.Fatalf("LoadSSHIdentity: %v", err)
	}
	rec, err := ParseSSHRecipients(pub)
	if err != nil {
		t.Fatalf("ParseSSHRecipients: %v", err)
	}
	roundTripWithIdentity(t, rec, []age.Identity{id})
}

func TestLoadSSHIdentity_EncryptedKeyPromptsForPassphrase(t *testing.T) {
	t.Parallel()
	const passphrase = "correct horse battery staple"
	priv, pub := generateSSHKeyPair(t, []byte(passphrase))
	keyPath := writeKeyToTempFile(t, priv)

	prompt := func(_ string) ([]byte, error) {
		return []byte(passphrase), nil
	}

	id, err := LoadSSHIdentity(keyPath, prompt)
	if err != nil {
		t.Fatalf("LoadSSHIdentity with passphrase: %v", err)
	}
	rec, err := ParseSSHRecipients(pub)
	if err != nil {
		t.Fatalf("ParseSSHRecipients: %v", err)
	}
	roundTripWithIdentity(t, rec, []age.Identity{id})
}

func TestLoadSSHIdentity_EncryptedKeyWithoutPromptIsRejected(t *testing.T) {
	t.Parallel()
	priv, _ := generateSSHKeyPair(t, []byte("hunter2"))
	keyPath := writeKeyToTempFile(t, priv)

	_, err := LoadSSHIdentity(keyPath, nil)
	if !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("LoadSSHIdentity on encrypted key with nil prompt: got %v, want ErrAuthFailed", err)
	}
}

func TestLoadSSHIdentities_SkipsMissingFiles(t *testing.T) {
	t.Parallel()
	priv, _ := generateSSHKeyPair(t, nil)
	real := writeKeyToTempFile(t, priv)
	missing := filepath.Join(filepath.Dir(real), "absent_key")

	ids, err := LoadSSHIdentities([]string{missing, real}, nil)
	if err != nil {
		t.Fatalf("LoadSSHIdentities: %v", err)
	}
	if len(ids) != 1 {
		t.Fatalf("got %d identities, want 1 (missing file should be skipped)", len(ids))
	}
}
