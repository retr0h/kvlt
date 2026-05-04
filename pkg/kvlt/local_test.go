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

func TestLocalProvider_PutGetRoundTrip(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, _ := newTestProvider(t)

	const key, want = "API_KEY", "sk-1234567890"
	if err := p.Put(ctx, key, want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := p.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != want {
		t.Fatalf("Get returned %q, want %q", got, want)
	}
}

func TestLocalProvider_GetMissingKeyReturnsErrKeyNotFound(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, _ := newTestProvider(t)

	_, err := p.Get(ctx, "NEVER_PUT")
	if !errors.Is(err, ErrKeyNotFound) {
		t.Fatalf("Get of missing key: got %v, want errors.Is(err, ErrKeyNotFound)", err)
	}
}

func TestLocalProvider_GetWithWrongIdentityFails(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, _ := newTestProvider(t)

	if err := p.Put(ctx, "API_KEY", "sk-1234"); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// Swap the resolver to return a different identity that was never
	// a recipient — Get must fail with ErrAuthFailed, not silently
	// return garbage or panic. This is the "someone else on the box"
	// case validated in code.
	other, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate other identity: %v", err)
	}
	p.identities = func() ([]age.Identity, error) { return []age.Identity{other}, nil }

	if _, err := p.Get(ctx, "API_KEY"); !errors.Is(err, ErrAuthFailed) {
		t.Fatalf("Get with wrong identity: got %v, want errors.Is(err, ErrAuthFailed)", err)
	}
}

func TestLocalProvider_PutOverwrites(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, _ := newTestProvider(t)

	if err := p.Put(ctx, "TOKEN", "v1"); err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	if err := p.Put(ctx, "TOKEN", "v2"); err != nil {
		t.Fatalf("Put v2: %v", err)
	}
	got, err := p.Get(ctx, "TOKEN")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got != "v2" {
		t.Fatalf("after overwrite: got %q, want %q", got, "v2")
	}
}

func TestLocalProvider_ListReturnsSortedKeysOnly(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, _ := newTestProvider(t)

	for _, k := range []string{"ZULU", "alpha", "MIKE"} {
		if err := p.Put(ctx, k, "x"); err != nil {
			t.Fatalf("Put %q: %v", k, err)
		}
	}

	keys, err := p.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"MIKE", "ZULU", "alpha"} // ASCII sort
	if len(keys) != len(want) {
		t.Fatalf("List: got %v, want %v", keys, want)
	}
	for i := range keys {
		if keys[i] != want[i] {
			t.Fatalf("List[%d]: got %q, want %q", i, keys[i], want[i])
		}
	}
}

func TestLocalProvider_ListOnEmptyOrMissingDirReturnsEmpty(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	p, _ := newTestProvider(t)

	keys, err := p.List(ctx)
	if err != nil {
		t.Fatalf("List on empty vault: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("List on empty vault: got %v, want empty", keys)
	}
}

func TestLocalProvider_MultiRecipientAnyIdentityCanDecrypt(t *testing.T) {
	t.Parallel()
	ctx := context.Background()

	a, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity a: %v", err)
	}
	b, err := age.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("generate identity b: %v", err)
	}

	dir := filepath.Join(t.TempDir(), "vault")
	// Encrypt to BOTH a and b — the team-sharing case.
	recipients := []age.Recipient{a.Recipient(), b.Recipient()}

	// Put with identity a as the resolver.
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

	// New provider over the same directory but with only identity b.
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
		t.Fatalf("Get returned %q, want %q", got, "the-secret")
	}
}

func TestLocalProvider_ConstructorRejectsBadInputs(t *testing.T) {
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
	}{
		{"empty name", "", dir, rec},
		{"empty dir", "dev", "", rec},
		{"no recipients", "dev", dir, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewLocalProvider(tc.vaultName, tc.dir, tc.recipients, resolver)
			if !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("NewLocalProvider(%q): got %v, want errors.Is(err, ErrInvalidConfig)", tc.name, err)
			}
		})
	}
}

func TestValidateSecretKey(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		key    string
		wantOK bool
	}{
		{"normal", "API_KEY", true},
		{"hyphen-and-digits", "key-2", true},
		{"empty", "", false},
		{"slash", "foo/bar", false},
		{"backslash", `foo\bar`, false},
		{"leading dot", ".key", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateSecretKey(tc.key)
			if (err == nil) != tc.wantOK {
				t.Fatalf("validateSecretKey(%q) = %v, wantOK=%v", tc.key, err, tc.wantOK)
			}
		})
	}
}
