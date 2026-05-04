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
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"filippo.io/age"
)

// newTestProvider builds a LocalProvider against a fresh tempdir
// using an age-native X25519 keypair. Tests intentionally avoid SSH
// keys — the round-trip semantics are identical, and X25519 keeps
// CI environments simple (no on-disk SSH state, no agent).
func newTestProvider(t *testing.T) (*LocalProvider, *age.X25519Identity) {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate test identity: %v", err)
	}
	dir := filepath.Join(t.TempDir(), "vault")
	resolver := func() ([]age.Identity, error) { return []age.Identity{id}, nil }
	p, err := NewLocalProvider("dev", dir, []age.Recipient{id.Recipient()}, resolver)
	if err != nil {
		t.Fatalf("NewLocalProvider: %v", err)
	}
	return p, id
}

func TestNewLocalProvider(t *testing.T) {
	t.Parallel()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	rec := []age.Recipient{id.Recipient()}
	resolver := func() ([]age.Identity, error) { return []age.Identity{id}, nil }
	dir := filepath.Join(t.TempDir(), "v")

	cases := []struct {
		name       string
		vaultName  string
		dir        string
		recipients []age.Recipient
		wantErr    error
	}{
		{
			name: "valid inputs construct successfully", vaultName: "dev", dir: dir,
			recipients: rec, wantErr: nil,
		},
		{
			name: "empty vault name returns ErrInvalidConfig", vaultName: "", dir: dir,
			recipients: rec, wantErr: ErrInvalidConfig,
		},
		{
			name: "empty dir returns ErrInvalidConfig", vaultName: "dev", dir: "",
			recipients: rec, wantErr: ErrInvalidConfig,
		},
		{
			name: "no recipients returns ErrInvalidConfig", vaultName: "dev", dir: dir,
			recipients: nil, wantErr: ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			subDir := tc.dir
			if subDir != "" {
				subDir = filepath.Join(t.TempDir(), "v") // unique per subtest
			}
			_, err := NewLocalProvider(tc.vaultName, subDir, tc.recipients, resolver)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewLocalProvider: %v", err)
			}
		})
	}
}

func TestLocalProvider_Name(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, want string
	}{
		{"returns the configured vault name verbatim", "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, _ := newTestProvider(t)
			if got := p.Name(); got != tc.want {
				t.Fatalf("Name(): got %q, want %q", got, tc.want)
			}
		})
	}
}

func TestLocalProvider_Recipients(t *testing.T) {
	t.Parallel()
	p, owner := newTestProvider(t)
	cases := []struct {
		name   string
		assert func(t *testing.T)
	}{
		{
			name: "returns one recipient when constructed with one",
			assert: func(t *testing.T) {
				t.Helper()
				if len(p.Recipients()) != 1 {
					t.Fatalf("len(Recipients()) = %d, want 1", len(p.Recipients()))
				}
			},
		},
		{
			name: "returned recipient round-trips back to the configured public key",
			assert: func(t *testing.T) {
				t.Helper()
				rec, ok := p.Recipients()[0].(*age.X25519Recipient)
				if !ok {
					t.Fatalf(
						"Recipients()[0] type: %T, want *age.X25519Recipient",
						p.Recipients()[0],
					)
				}
				if rec.String() != owner.Recipient().String() {
					t.Fatalf(
						"Recipients()[0] = %q, want %q",
						rec.String(),
						owner.Recipient().String(),
					)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t)
		})
	}
}

func TestLocalProvider_Describe(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		setup  func(t *testing.T) Describer
		assert func(t *testing.T, fields []DescribeField)
	}{
		{
			name: "factory-built provider returns recipient strings verbatim",
			setup: func(t *testing.T) Describer {
				t.Helper()
				store, id := newTestStore(t)
				if _, err := store.Create("dev", TypeLocalEncryption, []string{id.Recipient().String()}); err != nil {
					t.Fatalf("Create: %v", err)
				}
				p, err := store.Open("dev")
				if err != nil {
					t.Fatalf("Open: %v", err)
				}
				return p.(Describer)
			},
			assert: func(t *testing.T, fields []DescribeField) {
				t.Helper()
				if len(fields) != 1 || fields[0].Label != "recipients" {
					t.Fatalf("got %+v, want one `recipients` field", fields)
				}
				if len(fields[0].Values) != 1 {
					t.Fatalf("Values: got %v, want one entry", fields[0].Values)
				}
			},
		},
		{
			name: "directly-constructed provider returns recipients field with empty Values",
			setup: func(t *testing.T) Describer {
				t.Helper()
				p, _ := newTestProvider(t)
				return p
			},
			assert: func(t *testing.T, fields []DescribeField) {
				t.Helper()
				if len(fields) != 1 || len(fields[0].Values) != 0 {
					t.Fatalf("got %+v, want one field with no values", fields)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, tc.setup(t).Describe())
		})
	}
}

func TestLocalProvider_Put(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		setup   func(t *testing.T) *LocalProvider
		key     string
		value   string
		wantErr error
	}{
		{
			name:  "valid key writes successfully",
			setup: func(t *testing.T) *LocalProvider { t.Helper(); p, _ := newTestProvider(t); return p },
			key:   "API_KEY", value: "sk-1234",
		},
		{
			name:  "empty key returns ErrInvalidConfig",
			setup: func(t *testing.T) *LocalProvider { t.Helper(); p, _ := newTestProvider(t); return p },
			key:   "", value: "v",
			wantErr: ErrInvalidConfig,
		},
		{
			name:  "key with path separator returns ErrInvalidConfig",
			setup: func(t *testing.T) *LocalProvider { t.Helper(); p, _ := newTestProvider(t); return p },
			key:   "../escape", value: "v",
			wantErr: ErrInvalidConfig,
		},
		{
			name: "fails when vault dir cannot be written to",
			setup: func(t *testing.T) *LocalProvider {
				t.Helper()
				p, _ := newTestProvider(t)
				// Replace the vault dir with a regular file so atomic
				// write's CreateTemp fails.
				if err := os.RemoveAll(p.dir); err != nil {
					t.Fatalf("RemoveAll: %v", err)
				}
				if err := os.WriteFile(p.dir, []byte{}, 0o600); err != nil {
					t.Fatalf("plant blocking file: %v", err)
				}
				return p
			},
			key: "K", value: "v",
			wantErr: nil, // any non-nil error suffices; checked below
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := tc.setup(t)
			err := p.Put(context.Background(), tc.key, tc.value)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
			case tc.name == "fails when vault dir cannot be written to":
				if err == nil {
					t.Fatalf("Put: want error, got nil")
				}
			default:
				if err != nil {
					t.Fatalf("Put: %v", err)
				}
			}
		})
	}
}

func TestLocalProvider_Get(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		setup   func(t *testing.T) (p *LocalProvider, key string)
		want    string
		wantErr error
	}{
		{
			name: "returns the value previously written by Put",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				p, _ := newTestProvider(t)
				if err := p.Put(context.Background(), "API_KEY", "sk-1234567890"); err != nil {
					t.Fatalf("Put: %v", err)
				}
				return p, "API_KEY"
			},
			want: "sk-1234567890",
		},
		{
			name: "Put-Put-Get returns the second value (overwrite)",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				p, _ := newTestProvider(t)
				ctx := context.Background()
				if err := p.Put(ctx, "TOKEN", "v1"); err != nil {
					t.Fatalf("Put v1: %v", err)
				}
				if err := p.Put(ctx, "TOKEN", "v2"); err != nil {
					t.Fatalf("Put v2: %v", err)
				}
				return p, "TOKEN"
			},
			want: "v2",
		},
		{
			name: "missing key returns ErrKeyNotFound",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				p, _ := newTestProvider(t)
				return p, "NEVER_PUT"
			},
			wantErr: ErrKeyNotFound,
		},
		{
			name: "invalid key returns ErrInvalidConfig",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				p, _ := newTestProvider(t)
				return p, "../escape"
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "wrong identity returns ErrAuthFailed",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				p, owner := newTestProvider(t)
				if err := p.Put(context.Background(), "K", "v"); err != nil {
					t.Fatalf("Put: %v", err)
				}
				other, err := age.GenerateX25519Identity()
				if err != nil {
					t.Fatalf("identity: %v", err)
				}
				wrongResolver := func() ([]age.Identity, error) { return []age.Identity{other}, nil }
				intruder, err := NewLocalProvider(
					p.name, p.dir,
					[]age.Recipient{owner.Recipient()},
					wrongResolver,
				)
				if err != nil {
					t.Fatalf("NewLocalProvider (intruder): %v", err)
				}
				return intruder, "K"
			},
			wantErr: ErrAuthFailed,
		},
		{
			name: "nil identity resolver returns ErrAuthFailed",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				id, err := age.GenerateX25519Identity()
				if err != nil {
					t.Fatalf("identity: %v", err)
				}
				p, err := NewLocalProvider(
					"dev",
					filepath.Join(t.TempDir(), "v"),
					[]age.Recipient{id.Recipient()},
					nil,
				)
				if err != nil {
					t.Fatalf("NewLocalProvider: %v", err)
				}
				if err := p.Put(context.Background(), "K", "v"); err != nil {
					t.Fatalf("Put: %v", err)
				}
				return p, "K"
			},
			wantErr: ErrAuthFailed,
		},
		{
			name: "resolver that errors returns ErrAuthFailed",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				id, err := age.GenerateX25519Identity()
				if err != nil {
					t.Fatalf("identity: %v", err)
				}
				resolver := func() ([]age.Identity, error) { return nil, errors.New("kaboom") }
				p, err := NewLocalProvider(
					"dev", filepath.Join(t.TempDir(), "v"),
					[]age.Recipient{id.Recipient()}, resolver,
				)
				if err != nil {
					t.Fatalf("NewLocalProvider: %v", err)
				}
				if err := p.Put(context.Background(), "K", "v"); err != nil {
					t.Fatalf("Put: %v", err)
				}
				return p, "K"
			},
			wantErr: ErrAuthFailed,
		},
		{
			name: "resolver returning empty slice returns ErrAuthFailed",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				id, err := age.GenerateX25519Identity()
				if err != nil {
					t.Fatalf("identity: %v", err)
				}
				resolver := func() ([]age.Identity, error) { return nil, nil }
				p, err := NewLocalProvider(
					"dev", filepath.Join(t.TempDir(), "v"),
					[]age.Recipient{id.Recipient()}, resolver,
				)
				if err != nil {
					t.Fatalf("NewLocalProvider: %v", err)
				}
				if err := p.Put(context.Background(), "K", "v"); err != nil {
					t.Fatalf("Put: %v", err)
				}
				return p, "K"
			},
			wantErr: ErrAuthFailed,
		},
		{
			name: "directory at the secret path returns a generic read error",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				p, _ := newTestProvider(t)
				if err := os.MkdirAll(p.secretPath("BLOCK"), 0o700); err != nil {
					t.Fatalf("mkdir blocking: %v", err)
				}
				return p, "BLOCK"
			},
			// Generic error — no sentinel; assert that err is non-nil
			// in the test body.
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, key := tc.setup(t)
			got, err := p.Get(context.Background(), key)
			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
			case tc.name == "directory at the secret path returns a generic read error":
				if err == nil {
					t.Fatalf("Get: want error, got nil")
				}
			default:
				if err != nil {
					t.Fatalf("Get: %v", err)
				}
				if got != tc.want {
					t.Fatalf("Get: got %q, want %q", got, tc.want)
				}
			}
		})
	}
}

func TestLocalProvider_List(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		setup   func(t *testing.T) *LocalProvider
		want    []string
		wantErr bool
	}{
		{
			name: "returns sorted ASCII order, only .age entries",
			setup: func(t *testing.T) *LocalProvider {
				t.Helper()
				p, _ := newTestProvider(t)
				ctx := context.Background()
				for _, k := range []string{"ZULU", "alpha", "MIKE"} {
					if err := p.Put(ctx, k, "x"); err != nil {
						t.Fatalf("Put: %v", err)
					}
				}
				return p
			},
			want: []string{"MIKE", "ZULU", "alpha"},
		},
		{
			name:  "empty vault dir returns empty slice",
			setup: func(t *testing.T) *LocalProvider { t.Helper(); p, _ := newTestProvider(t); return p },
			want:  []string{},
		},
		{
			name: "missing vault dir returns empty slice",
			setup: func(t *testing.T) *LocalProvider {
				t.Helper()
				p, _ := newTestProvider(t)
				if err := os.RemoveAll(p.dir); err != nil {
					t.Fatalf("RemoveAll: %v", err)
				}
				return p
			},
			want: []string{},
		},
		{
			name: "vault dir replaced by file returns error",
			setup: func(t *testing.T) *LocalProvider {
				t.Helper()
				p, _ := newTestProvider(t)
				if err := os.RemoveAll(p.dir); err != nil {
					t.Fatalf("RemoveAll: %v", err)
				}
				if err := os.WriteFile(p.dir, []byte{}, 0o600); err != nil {
					t.Fatalf("plant blocking file: %v", err)
				}
				return p
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := tc.setup(t)
			got, err := p.List(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatalf("List: want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("List: got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLocalProvider_Delete(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		setup     func(t *testing.T) (*LocalProvider, string)
		wantErr   error
		wantNoKey bool
	}{
		{
			name: "deleting an existing key removes it from List",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				p, _ := newTestProvider(t)
				if err := p.Put(context.Background(), "API_KEY", "sk-1234"); err != nil {
					t.Fatalf("Put: %v", err)
				}
				return p, "API_KEY"
			},
			wantNoKey: true,
		},
		{
			name: "deleting a missing key returns ErrKeyNotFound",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				p, _ := newTestProvider(t)
				return p, "ABSENT"
			},
			wantErr: ErrKeyNotFound,
		},
		{
			name: "invalid key name is rejected before touching disk",
			setup: func(t *testing.T) (*LocalProvider, string) {
				t.Helper()
				p, _ := newTestProvider(t)
				return p, "../escape"
			},
			wantErr: ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p, key := tc.setup(t)
			err := p.Delete(context.Background(), key)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("Delete: want %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Delete: %v", err)
			}
			if tc.wantNoKey {
				keys, lerr := p.List(context.Background())
				if lerr != nil {
					t.Fatalf("List after Delete: %v", lerr)
				}
				if slices.Contains(keys, key) {
					t.Fatalf("Delete left key %q in vault: %v", key, keys)
				}
			}
		})
	}
}

func TestLocalProvider_MultiRecipientAnyIdentityCanDecrypt(t *testing.T) {
	// One scenario, but it's a property of the (Put, Get) pair —
	// asserts that secrets encrypted to N recipients are decryptable
	// by any one of the matching identities. Not a Put-only or Get-only
	// behavior, so it earns its own function.
	t.Parallel()
	ctx := context.Background()

	a, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("identity a: %v", err)
	}
	b, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("identity b: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "vault")
	recipients := []age.Recipient{a.Recipient(), b.Recipient()}

	pa, err := NewLocalProvider(
		"shared", dir, recipients,
		func() ([]age.Identity, error) { return []age.Identity{a}, nil },
	)
	if err != nil {
		t.Fatalf("NewLocalProvider a: %v", err)
	}
	if err := pa.Put(ctx, "SHARED_TOKEN", "the-secret"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	pb, err := NewLocalProvider(
		"shared", dir, recipients,
		func() ([]age.Identity, error) { return []age.Identity{b}, nil },
	)
	if err != nil {
		t.Fatalf("NewLocalProvider b: %v", err)
	}
	got, err := pb.Get(ctx, "SHARED_TOKEN")
	if err != nil {
		t.Fatalf("Get with identity b: %v", err)
	}
	if got != "the-secret" {
		t.Fatalf("Get: got %q, want %q", got, "the-secret")
	}
}

func TestValidateSecretKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, key string
		wantErr   error
	}{
		{"plain identifier accepted", "API_KEY", nil},
		{"hyphen and digits accepted", "key-2", nil},
		{"empty rejected", "", ErrInvalidConfig},
		{"forward slash rejected", "foo/bar", ErrInvalidConfig},
		{"backslash rejected", `foo\bar`, ErrInvalidConfig},
		{"leading dot rejected", ".key", ErrInvalidConfig},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateSecretKey(tc.key)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("validateSecretKey(%q): %v", tc.key, err)
			}
		})
	}
}

func TestWriteFileAtomic(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		setup   func(t *testing.T) (path string, data []byte)
		wantErr bool
	}{
		{
			name: "writes file with requested mode",
			setup: func(t *testing.T) (string, []byte) {
				t.Helper()
				return filepath.Join(t.TempDir(), "f"), []byte("payload")
			},
		},
		{
			name: "missing parent dir returns error",
			setup: func(t *testing.T) (string, []byte) {
				t.Helper()
				return filepath.Join(t.TempDir(), "no-such-dir", "f"), []byte("x")
			},
			wantErr: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			path, data := tc.setup(t)
			err := writeFileAtomic(path, data, 0o600)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("writeFileAtomic: want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("writeFileAtomic: %v", err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("Stat: %v", err)
			}
			if info.Mode().Perm() != 0o600 {
				t.Fatalf("mode: got %o, want 0600", info.Mode().Perm())
			}
		})
	}
}

func TestNewLocalProviderFromConfig(t *testing.T) {
	t.Parallel()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	validRec := id.Recipient().String()

	cases := []struct {
		name    string
		cfg     *Config
		wantErr error
	}{
		{
			name: "valid config constructs provider",
			cfg: &Config{
				ID: "i", Name: "v", Type: TypeLocalEncryption,
				Settings: map[string]any{"recipients": []any{validRec}},
			},
		},
		{
			name: "missing recipients returns ErrInvalidConfig",
			cfg: &Config{
				ID: "i", Name: "v", Type: TypeLocalEncryption,
				Settings: map[string]any{},
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "empty recipients slice returns ErrInvalidConfig",
			cfg: &Config{
				ID: "i", Name: "v", Type: TypeLocalEncryption,
				Settings: map[string]any{"recipients": []any{}},
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "non-string recipient entry returns ErrInvalidConfig",
			cfg: &Config{
				ID: "i", Name: "v", Type: TypeLocalEncryption,
				Settings: map[string]any{"recipients": []any{42}},
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "unparseable recipient string returns ErrInvalidConfig",
			cfg: &Config{
				ID: "i", Name: "v", Type: TypeLocalEncryption,
				Settings: map[string]any{"recipients": []any{"junk"}},
			},
			wantErr: ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := newLocalProviderFromConfig(t.TempDir(), tc.cfg, nil)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("newLocalProviderFromConfig: %v", err)
			}
		})
	}
}
