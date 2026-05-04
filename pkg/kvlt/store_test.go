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
	"testing"

	"filippo.io/age"
)

// newTestStore builds a Store rooted at a fresh tempdir, returning
// it alongside a single age identity convenient for both creating
// vaults (its Recipient) and decrypting (its Identity).
func newTestStore(t *testing.T) (*Store, *age.X25519Identity) {
	t.Helper()
	id, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity: %v", err)
	}
	repo := filepath.Join(t.TempDir(), "repo")
	resolver := func() ([]age.Identity, error) { return []age.Identity{id}, nil }
	store, err := NewStore(repo, resolver)
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	return store, id
}

func TestNewStore(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		repoPath string
		wantErr  error
	}{
		{
			name:     "valid path constructs successfully",
			repoPath: t.TempDir(),
		},
		{
			name:     "empty path returns ErrInvalidConfig",
			repoPath: "",
			wantErr:  ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewStore(tc.repoPath, nil)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("NewStore: %v", err)
			}
		})
	}
}

func TestStore_RepoPath(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
	}{
		{"absolute path passes through", t.TempDir()},
		{"relative path is resolved to absolute", "."},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			s, err := NewStore(tc.in, nil)
			if err != nil {
				t.Fatalf("NewStore: %v", err)
			}
			if !filepath.IsAbs(s.RepoPath()) {
				t.Fatalf("RepoPath() = %q, want absolute", s.RepoPath())
			}
		})
	}
}

func TestStore_Create(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		setup     func(t *testing.T, s *Store, rec []string)
		vaultName string
		vaultType string
		recipFn   func(id *age.X25519Identity) []string // recipient strings; nil means default
		wantErr   error
		assert    func(t *testing.T, cfg *Config)
	}{
		{
			name:      "valid inputs round-trip",
			vaultName: "dev", vaultType: TypeLocalEncryption,
		},
		{
			name:      "TypeLocal alias is canonicalized to wire identifier",
			vaultName: "dev", vaultType: TypeLocal,
			assert: func(t *testing.T, cfg *Config) {
				t.Helper()
				if cfg.Type != TypeLocalEncryption {
					t.Fatalf(
						"Type after canonicalization: got %q, want %q",
						cfg.Type,
						TypeLocalEncryption,
					)
				}
			},
		},
		{
			name:      "duplicate name returns ErrVaultAlreadyExists",
			vaultName: "dev", vaultType: TypeLocalEncryption,
			setup: func(t *testing.T, s *Store, rec []string) {
				t.Helper()
				if _, err := s.Create("dev", TypeLocalEncryption, rec); err != nil {
					t.Fatalf("first Create: %v", err)
				}
			},
			wantErr: ErrVaultAlreadyExists,
		},
		{
			name:      "unknown type returns ErrInvalidConfig",
			vaultName: "dev", vaultType: "unknown-type",
			wantErr: ErrInvalidConfig,
		},
		{
			name:      "no recipients returns ErrInvalidConfig",
			vaultName: "dev", vaultType: TypeLocalEncryption,
			recipFn: func(_ *age.X25519Identity) []string { return nil },
			wantErr: ErrInvalidConfig,
		},
		{
			name:      "malformed recipient returns ErrInvalidConfig",
			vaultName: "dev", vaultType: TypeLocalEncryption,
			recipFn: func(_ *age.X25519Identity) []string { return []string{"not-a-key"} },
			wantErr: ErrInvalidConfig,
		},
		{
			name:      "empty name returns ErrInvalidConfig",
			vaultName: "", vaultType: TypeLocalEncryption,
			wantErr: ErrInvalidConfig,
		},
		{
			name:      "name with forward slash returns ErrInvalidConfig",
			vaultName: "foo/bar", vaultType: TypeLocalEncryption,
			wantErr: ErrInvalidConfig,
		},
		{
			name:      "name with backslash returns ErrInvalidConfig",
			vaultName: `foo\bar`, vaultType: TypeLocalEncryption,
			wantErr: ErrInvalidConfig,
		},
		{
			name:      "name with leading dot returns ErrInvalidConfig",
			vaultName: ".hidden", vaultType: TypeLocalEncryption,
			wantErr: ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, id := newTestStore(t)
			rec := []string{id.Recipient().String()}
			if tc.recipFn != nil {
				rec = tc.recipFn(id)
			}
			if tc.setup != nil {
				tc.setup(t, store, rec)
			}

			cfg, err := store.Create(tc.vaultName, tc.vaultType, rec)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Create: %v", err)
			}
			if cfg.ID == "" || cfg.Name != tc.vaultName {
				t.Fatalf("Create returned %+v", cfg)
			}
			if tc.assert != nil {
				tc.assert(t, cfg)
			}
		})
	}
}

func TestStore_Open(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		setup   func(t *testing.T) (s *Store, vaultName string)
		wantErr error
	}{
		{
			name: "round-trips a vault that was just created",
			setup: func(t *testing.T) (*Store, string) {
				t.Helper()
				store, id := newTestStore(t)
				if _, err := store.Create("dev", TypeLocalEncryption, []string{id.Recipient().String()}); err != nil {
					t.Fatalf("Create: %v", err)
				}
				return store, "dev"
			},
		},
		{
			name: "round-trips through a freshly-constructed Store over the same repo",
			setup: func(t *testing.T) (*Store, string) {
				t.Helper()
				store, id := newTestStore(t)
				if _, err := store.Create("dev", TypeLocalEncryption, []string{id.Recipient().String()}); err != nil {
					t.Fatalf("Create: %v", err)
				}
				store2, err := NewStore(store.RepoPath(), func() ([]age.Identity, error) {
					return []age.Identity{id}, nil
				})
				if err != nil {
					t.Fatalf("NewStore (reopen): %v", err)
				}
				return store2, "dev"
			},
		},
		{
			name:    "missing vault returns ErrVaultNotFound",
			setup:   func(t *testing.T) (*Store, string) { t.Helper(); s, _ := newTestStore(t); return s, "absent" },
			wantErr: ErrVaultNotFound,
		},
		{
			name: "unregistered backend type returns ErrInvalidConfig",
			setup: func(t *testing.T) (*Store, string) {
				t.Helper()
				store, _ := newTestStore(t)
				dir := filepath.Join(store.RepoPath(), "vaults", "imaginary")
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				body := "id: abc\nname: ghost\ntype: imaginary\n"
				if err := os.WriteFile(filepath.Join(dir, "abc.yaml"), []byte(body), 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
				return store, "ghost"
			},
			wantErr: ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store, name := tc.setup(t)
			provider, err := store.Open(name)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			// Smoke round-trip — Open's contract says the returned
			// provider works.
			ctx := context.Background()
			if err := provider.Put(ctx, "K", "v"); err != nil {
				t.Fatalf("Put after Open: %v", err)
			}
			got, err := provider.Get(ctx, "K")
			if err != nil {
				t.Fatalf("Get after Open: %v", err)
			}
			if got != "v" {
				t.Fatalf("round-trip: got %q, want %q", got, "v")
			}
		})
	}
}

func TestStore_List(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		setup     func(t *testing.T) *Store
		wantNames []string
		wantErr   error
	}{
		{
			name:      "empty repo returns empty slice",
			setup:     func(t *testing.T) *Store { t.Helper(); s, _ := newTestStore(t); return s },
			wantNames: []string{},
		},
		{
			name: "returns created vaults sorted alphabetically by name",
			setup: func(t *testing.T) *Store {
				t.Helper()
				store, id := newTestStore(t)
				rec := []string{id.Recipient().String()}
				for _, name := range []string{"prod", "dev", "staging"} {
					if _, err := store.Create(name, TypeLocalEncryption, rec); err != nil {
						t.Fatalf("Create %q: %v", name, err)
					}
				}
				return store
			},
			wantNames: []string{"dev", "prod", "staging"},
		},
		{
			name: "malformed YAML on disk returns ErrInvalidConfig",
			setup: func(t *testing.T) *Store {
				t.Helper()
				store, _ := newTestStore(t)
				dir := filepath.Join(store.RepoPath(), "vaults", TypeLocalEncryption)
				if err := os.MkdirAll(dir, 0o700); err != nil {
					t.Fatalf("mkdir: %v", err)
				}
				if err := os.WriteFile(filepath.Join(dir, "bad.yaml"), []byte("not: valid: yaml: ::"), 0o600); err != nil {
					t.Fatalf("write: %v", err)
				}
				return store
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "config with empty id returns ErrInvalidConfig",
			setup: func(t *testing.T) *Store {
				t.Helper()
				return storeWithRawConfig(t, "id: \"\"\nname: dev\ntype: local_encryption\n")
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "config with empty name returns ErrInvalidConfig",
			setup: func(t *testing.T) *Store {
				t.Helper()
				return storeWithRawConfig(t, "id: abc\nname: \"\"\ntype: local_encryption\n")
			},
			wantErr: ErrInvalidConfig,
		},
		{
			name: "config with empty type returns ErrInvalidConfig",
			setup: func(t *testing.T) *Store {
				t.Helper()
				return storeWithRawConfig(t, "id: abc\nname: dev\ntype: \"\"\n")
			},
			wantErr: ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			store := tc.setup(t)
			got, err := store.List()
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want errors.Is(err, %v)", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("List: %v", err)
			}
			if len(got) != len(tc.wantNames) {
				t.Fatalf("len: got %d entries, want %d", len(got), len(tc.wantNames))
			}
			for i, want := range tc.wantNames {
				if got[i].Name != want {
					t.Fatalf("List[%d]: got %q, want %q", i, got[i].Name, want)
				}
			}
		})
	}
}

// storeWithRawConfig writes a hand-crafted YAML body into a Store's
// vaults/local_encryption/x.yaml so List has something to parse.
// Used by the empty-fields cases for TestStore_List — typed setup
// would route through writeConfig and never produce an empty-field
// document.
func storeWithRawConfig(t *testing.T, body string) *Store {
	t.Helper()
	store, _ := newTestStore(t)
	dir := filepath.Join(store.RepoPath(), "vaults", "local_encryption")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "x.yaml"), []byte(body), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	return store
}
