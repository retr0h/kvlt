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
	"fmt"
	"slices"
	"sync"
)

// ProviderFactory builds a Provider from a stored vault Config plus
// the runtime context (the repo root, the IdentityResolver). Every
// backend registers a factory under its type identifier; Store
// dispatches by looking up cfg.Type in the registry. Adding a new
// backend (sops, aws-sm, 1password, hashicorp-vault) is a single
// new file with one RegisterBackend call in its init() — the base
// binary doesn't change, and build tags gate the cloud SDKs.
type ProviderFactory func(repoPath string, cfg *Config, identities IdentityResolver) (Provider, error)

var (
	registryMu sync.RWMutex
	registry   = map[string]ProviderFactory{}
)

// RegisterBackend registers a ProviderFactory under typeID. Called
// from each backend's init(); panics on duplicate registration so
// build-tag misconfigurations (two files registering the same type)
// surface at startup rather than at first vault open. Pure
// programming errors deserve panics, not silent shadowing.
func RegisterBackend(typeID string, factory ProviderFactory) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if _, exists := registry[typeID]; exists {
		panic(fmt.Sprintf("kvlt: backend %q registered twice", typeID))
	}
	registry[typeID] = factory
}

// RegisteredBackends returns the type identifiers of every backend
// available in this build, sorted. Used by the CLI's `vault create`
// help text and by error messages — "unknown type X, available
// types are [local_encryption]" — so a user with the wrong build
// tags can see what's missing.
func RegisteredBackends() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	slices.Sort(out)
	return out
}

// IsBackendRegistered reports whether typeID has a registered
// factory. Used by Store.Create's preflight before the YAML is
// written — better to refuse a create with a clear "unknown type"
// than to write a config no Open can use.
func IsBackendRegistered(typeID string) bool {
	registryMu.RLock()
	defer registryMu.RUnlock()
	_, ok := registry[typeID]
	return ok
}

// newProviderFromConfig dispatches to the registered factory for
// cfg.Type. Returns ErrInvalidConfig if no backend matches —
// usually a build-tag mismatch (a vault config saved on a build
// with `-tags aws` opened on a base binary without that tag).
func newProviderFromConfig(
	repoPath string,
	cfg *Config,
	identities IdentityResolver,
) (Provider, error) {
	registryMu.RLock()
	factory, ok := registry[cfg.Type]
	registryMu.RUnlock()
	if !ok {
		return nil, fmt.Errorf(
			"%w: unknown vault type %q in config — registered types: %v (rebuild with appropriate -tags?)",
			ErrInvalidConfig,
			cfg.Type,
			RegisteredBackends(),
		)
	}
	return factory(repoPath, cfg, identities)
}
