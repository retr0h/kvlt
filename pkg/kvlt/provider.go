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

// Package kvlt is the public vault library — importable by other Go
// projects (`import "github.com/retr0h/kvlt/pkg/kvlt"`) and used by
// kvlt's own CLI through the same surface. The cmd/ tree intentionally
// dogfoods this package: anything the CLI can do, an external Go
// program can do via New* constructors.
//
// Provider is the small interface every backend implements; the local
// AES-GCM backend lives next to this file, cloud backends behind build
// tags so the base binary stays dependency-light. Construction is
// always via a `New*` function — there are no exported zero-value
// types meant to be used directly.
package kvlt

import "context"

// Provider is the surface every vault backend must satisfy. Mirrors
// swamp's VaultProvider (TypeScript) so call patterns carry between
// the two systems — Get/Put/List/Name only, on purpose. Anything
// fancier (rotation, lease, audit) is layered on top by callers, not
// pushed into the backend contract.
type Provider interface {
	// Get retrieves a secret by key. Returns an error if the key is
	// missing — backends should not silently return "" for unknown
	// keys, since callers can't distinguish that from an empty value.
	Get(ctx context.Context, key string) (string, error)

	// Put stores a secret. Overwrites without warning — versioning,
	// if any, is the backend's concern.
	Put(ctx context.Context, key, value string) error

	// List returns every key currently stored in this vault. Values
	// are never returned, even in debug logs.
	List(ctx context.Context) ([]string, error)

	// Name returns the vault's user-defined name (not the backend
	// type). Used in error messages and audit lines.
	Name() string
}

// Describer is an optional capability backends may implement to
// expose human-readable detail to `kvlt vault info`. Modeled the
// same way as io.WriterTo / io.ReaderFrom — narrow, single-purpose,
// callers type-assert.
//
// Each DescribeField is one row in the rendered output. Backends
// decide what to surface (recipient list for local, region + secret
// prefix for AWS, kv path for HCV) — the CLI never reaches into
// Config.Settings, so adding a new backend doesn't require teaching
// `vault info` about its settings shape.
type Describer interface {
	Describe() []DescribeField
}

// DescribeField is one (label, values) row in `vault info` output.
// Multi-value fields (e.g. a recipient list) carry every entry in
// Values so the CLI renderer can format them consistently. Single-
// value fields use a one-element Values slice.
type DescribeField struct {
	Label  string
	Values []string
}

// Backend type identifiers. Stored as the `type` field in vault
// configuration files. Constants — not loose strings — so a typo in
// CLI or library code is a compile error, not a runtime "unknown
// backend" error.
const (
	// TypeLocalEncryption is the wire identifier (matches swamp's
	// vault subsystem) and is what gets persisted to YAML and used as
	// a path component under vaults/.
	TypeLocalEncryption = "local_encryption"

	// TypeLocal is the friendly alias accepted on the way in. Library
	// callers and the CLI both pass "local"; CanonicalizeType maps it
	// to TypeLocalEncryption before lookup so external Go programs do
	// not need to know the wire identifier.
	TypeLocal = "local"
)

// CanonicalizeType normalizes user-facing aliases to the wire
// identifier persisted in vault configs. Unknown values pass through
// unchanged so the registry's "unknown vault type" error path still
// fires for genuine typos. The `local-encryption` hyphen typo is
// translated to TypeLocalEncryption explicitly because it is the most
// common mistake — coming from the path component spelling.
func CanonicalizeType(t string) string {
	switch t {
	case TypeLocal, "local-encryption":
		return TypeLocalEncryption
	default:
		return t
	}
}

// Config is the on-disk vault configuration. Each named vault writes
// one of these to vaults/{type}/{id}.yaml, mirroring swamp's layout
// so an operator's mental model carries between projects.
type Config struct {
	// ID is the auto-assigned UUID; never edited by humans.
	ID string `yaml:"id"`
	// Name is the user-defined vault name used in put/get expressions.
	Name string `yaml:"name"`
	// Type identifies the backend (local_encryption, aws-sm, …). Use
	// one of the Type* constants when constructing Configs in code.
	Type string `yaml:"type"`
	// Settings is backend-specific configuration. Schema validation
	// lives in each backend's loader.
	Settings map[string]any `yaml:"settings,omitempty"`
}
