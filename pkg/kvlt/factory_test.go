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
	"errors"
	"slices"
	"testing"
)

// dummyFactory is a no-op ProviderFactory used by registry tests that
// only care about presence/absence of a registration, not behavior.
func dummyFactory(_ string, _ *Config, _ IdentityResolver) (Provider, error) {
	return nil, nil
}

// withTempBackend registers a backend under typeID for the duration of
// the test, restoring the registry to its prior state on cleanup.
// Required for tests that mutate process-wide state (the registry is
// guarded by a sync.RWMutex and is shared across the test binary).
func withTempBackend(t *testing.T, typeID string) {
	t.Helper()
	RegisterBackend(typeID, dummyFactory)
	t.Cleanup(func() {
		registryMu.Lock()
		delete(registry, typeID)
		registryMu.Unlock()
	})
}

func TestRegisterBackend(t *testing.T) {
	// Cannot t.Parallel here: subtests mutate the process-wide registry.
	cases := []struct {
		name      string
		setup     func(t *testing.T) (typeID string)
		register  bool // call RegisterBackend under test
		wantPanic bool
	}{
		{
			name:      "first registration succeeds silently",
			setup:     func(_ *testing.T) string { return "factory-test-fresh" },
			register:  true,
			wantPanic: false,
		},
		{
			name: "duplicate registration panics",
			setup: func(t *testing.T) string {
				t.Helper()
				const id = "factory-test-dup"
				withTempBackend(t, id)
				return id
			},
			register:  true,
			wantPanic: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			typeID := tc.setup(t)
			t.Cleanup(func() {
				registryMu.Lock()
				delete(registry, typeID)
				registryMu.Unlock()
			})

			defer func() {
				r := recover()
				if (r != nil) != tc.wantPanic {
					t.Fatalf("RegisterBackend panic=%v, want panic=%v", r != nil, tc.wantPanic)
				}
			}()
			if tc.register {
				RegisterBackend(typeID, dummyFactory)
			}
		})
	}
}

func TestIsBackendRegistered(t *testing.T) {
	withTempBackend(t, "factory-test-isreg")
	cases := []struct {
		name, typeID string
		want         bool
	}{
		{"builtin local backend is always registered", TypeLocalEncryption, true},
		{"test-only registration is visible", "factory-test-isreg", true},
		{"unregistered type returns false", "definitely-unregistered", false},
		{"empty string returns false", "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := IsBackendRegistered(tc.typeID); got != tc.want {
				t.Fatalf("IsBackendRegistered(%q) = %v, want %v", tc.typeID, got, tc.want)
			}
		})
	}
}

func TestRegisteredBackends(t *testing.T) {
	t.Parallel()
	got := RegisteredBackends()
	cases := []struct {
		name   string
		assert func(t *testing.T, got []string)
	}{
		{
			name: "includes builtin local backend",
			assert: func(t *testing.T, got []string) {
				t.Helper()
				if !slices.Contains(got, TypeLocalEncryption) {
					t.Fatalf("got %v, want it to include %q", got, TypeLocalEncryption)
				}
			},
		},
		{
			name: "result is sorted alphabetically",
			assert: func(t *testing.T, got []string) {
				t.Helper()
				if !slices.IsSorted(got) {
					t.Fatalf("got %v, want sorted", got)
				}
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			tc.assert(t, got)
		})
	}
}

func TestNewProviderFromConfig(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cfg     *Config
		wantErr error
	}{
		{
			name:    "unregistered type returns ErrInvalidConfig",
			cfg:     &Config{ID: "i", Name: "n", Type: "definitely-unregistered"},
			wantErr: ErrInvalidConfig,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := newProviderFromConfig(t.TempDir(), tc.cfg, nil)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("newProviderFromConfig: got %v, want errors.Is(err, %v)", err, tc.wantErr)
			}
		})
	}
}
