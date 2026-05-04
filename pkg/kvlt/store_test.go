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

func TestNewStore_RejectsEmptyPath(t *testing.T) {
	t.Parallel()
	if _, err := NewStore("", nil); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("NewStore with empty path: got %v, want ErrInvalidConfig", err)
	}
}

func TestStore_CreateAndOpenRoundTrip(t *testing.T) {
	t.Parallel()
	store, id := newTestStore(t)
	ctx := context.Background()

	cfg, err := store.Create("dev", TypeLocalEncryption, []age.Recipient{id.Recipient()})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if cfg.ID == "" {
		t.Fatalf("Create returned config with empty ID")
	}
	if cfg.Name != "dev" || cfg.Type != TypeLocalEncryption {
		t.Fatalf("Create returned wrong identity: %+v", cfg)
	}

	provider, err := store.Open("dev")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := provider.Put(ctx, "API_KEY", "sk-1234"); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := provider.Get(ctx, "API_KEY")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "sk-1234" {
		t.Fatalf("Get returned %q, want %q", got, "sk-1234")
	}
}

func TestStore_OpenMissingVaultReturnsErrVaultNotFound(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	if _, err := store.Open("absent"); !errors.Is(err, ErrVaultNotFound) {
		t.Fatalf("Open of missing vault: got %v, want ErrVaultNotFound", err)
	}
}

func TestStore_CreateDuplicateNameReturnsErrVaultAlreadyExists(t *testing.T) {
	t.Parallel()
	store, id := newTestStore(t)
	rec := []age.Recipient{id.Recipient()}

	if _, err := store.Create("dev", TypeLocalEncryption, rec); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := store.Create("dev", TypeLocalEncryption, rec)
	if !errors.Is(err, ErrVaultAlreadyExists) {
		t.Fatalf("second Create with same name: got %v, want ErrVaultAlreadyExists", err)
	}
}

func TestStore_CreateRejectsUnknownType(t *testing.T) {
	t.Parallel()
	store, id := newTestStore(t)
	_, err := store.Create("dev", "unknown-type", []age.Recipient{id.Recipient()})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Create with unknown type: got %v, want ErrInvalidConfig", err)
	}
}

func TestStore_CreateRejectsEmptyRecipients(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	_, err := store.Create("dev", TypeLocalEncryption, nil)
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("Create with no recipients: got %v, want ErrInvalidConfig", err)
	}
}

func TestStore_CreateRejectsBadName(t *testing.T) {
	t.Parallel()
	store, id := newTestStore(t)
	rec := []age.Recipient{id.Recipient()}

	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"slash", "foo/bar"},
		{"backslash", `foo\bar`},
		{"leading dot", ".hidden"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := store.Create(tc.input, TypeLocalEncryption, rec)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("Create(%q): got %v, want ErrInvalidConfig", tc.input, err)
			}
		})
	}
}

func TestStore_ListReturnsCreatedVaultsSorted(t *testing.T) {
	t.Parallel()
	store, id := newTestStore(t)
	rec := []age.Recipient{id.Recipient()}

	for _, name := range []string{"prod", "dev", "staging"} {
		if _, err := store.Create(name, TypeLocalEncryption, rec); err != nil {
			t.Fatalf("Create %q: %v", name, err)
		}
	}

	configs, err := store.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"dev", "prod", "staging"}
	if len(configs) != len(want) {
		t.Fatalf("List: got %d entries, want %d", len(configs), len(want))
	}
	for i, c := range configs {
		if c.Name != want[i] {
			t.Fatalf("List[%d]: got %q, want %q", i, c.Name, want[i])
		}
	}
}

func TestStore_ListOnEmptyRepoReturnsEmptySlice(t *testing.T) {
	t.Parallel()
	store, _ := newTestStore(t)
	configs, err := store.List()
	if err != nil {
		t.Fatalf("List on empty repo: %v", err)
	}
	if len(configs) != 0 {
		t.Fatalf("List on empty repo: got %v, want empty", configs)
	}
}

func TestStore_OpenReadsRecipientsFromYAML(t *testing.T) {
	t.Parallel()
	store, id := newTestStore(t)
	ctx := context.Background()

	if _, err := store.Create("dev", TypeLocalEncryption, []age.Recipient{id.Recipient()}); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// New Store over the same repoPath — the original instance is
	// not consulted; everything goes through the on-disk YAML. This
	// guards against an accidental reliance on in-memory state in
	// the Create → Open path.
	store2, err := NewStore(store.RepoPath(), func() ([]age.Identity, error) {
		return []age.Identity{id}, nil
	})
	if err != nil {
		t.Fatalf("NewStore (reopen): %v", err)
	}
	provider, err := store2.Open("dev")
	if err != nil {
		t.Fatalf("Open via reopened Store: %v", err)
	}
	if err := provider.Put(ctx, "TOKEN", "v"); err != nil {
		t.Fatalf("Put via reopened Store: %v", err)
	}
	got, err := provider.Get(ctx, "TOKEN")
	if err != nil {
		t.Fatalf("Get via reopened Store: %v", err)
	}
	if got != "v" {
		t.Fatalf("Get via reopened Store: got %q, want %q", got, "v")
	}
}
