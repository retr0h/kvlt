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
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"gopkg.in/yaml.v3"
)

// Store is a named-vault repository — the directory layer that maps
// human-friendly vault names ("dev", "prod") to backend providers.
// Every kvlt operation that's not "encrypt this raw file" goes
// through a Store: the CLI verbs, external Go callers, the migrate
// command. The path layout matches swamp's so muscle memory carries:
//
//	{repoPath}/
//	  vaults/
//	    {type}/
//	      {id}.yaml         # vault config — name, type, recipients
//	  .kvlt/
//	    secrets/
//	      {type}/
//	        {name}/         # backend-managed; for `local`, the .age blobs
//
// A Store does not load identities; that's the caller's call so a
// non-interactive CI run can supply a static identity list while an
// interactive shell can pop a passphrase prompt. Pass an
// IdentityResolver to NewStore to make Get work; pass nil for a
// write-only Store (creating new vaults, putting secrets, listing
// keys — none of which need to decrypt).
type Store struct {
	repoPath   string
	identities IdentityResolver
}

// NewStore returns a Store rooted at repoPath. The directory is not
// required to exist — Create will lay it down on first vault
// creation. identityResolver may be nil for a write-only Store.
func NewStore(repoPath string, identityResolver IdentityResolver) (*Store, error) {
	if repoPath == "" {
		return nil, fmt.Errorf("%w: repository path is empty", ErrInvalidConfig)
	}
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		return nil, fmt.Errorf("resolve repo path %q: %w", repoPath, err)
	}
	return &Store{repoPath: abs, identities: identityResolver}, nil
}

// RepoPath returns the absolute path of the repository root. Helpful
// for diagnostic output and for callers that want to derive sibling
// paths (a `.envrc`, an editor temp file, …) relative to it.
func (s *Store) RepoPath() string { return s.repoPath }

// Create initializes a new vault with the given name and type. The
// vault's ID is freshly generated; the caller never assigns one.
// Returns ErrVaultAlreadyExists if a vault with this name is already
// configured (regardless of type).
//
// recipientStrings is a list of authorized_keys-style SSH lines or
// age-native recipient strings (`age1…`); each is validated before
// the vault is written. At least one valid recipient is required —
// a vault encrypted to nobody is treated as a misconfiguration,
// since nothing put into it could ever be read back. The strings
// are stored as-passed in the config, so an operator auditing the
// YAML sees exactly what was given (and can paste it directly into
// `ssh-keygen -lf -` to confirm a fingerprint).
func (s *Store) Create(name, vaultType string, recipientStrings []string) (*Config, error) {
	if err := validateVaultName(name); err != nil {
		return nil, err
	}
	if !IsBackendRegistered(vaultType) {
		return nil, fmt.Errorf(
			"%w: unknown vault type %q — registered types: %v",
			ErrInvalidConfig, vaultType, RegisteredBackends(),
		)
	}
	if len(recipientStrings) == 0 {
		return nil, fmt.Errorf("%w: at least one recipient is required", ErrInvalidConfig)
	}
	// Validate every recipient string up front — the worst time to
	// discover a bad recipient is on first Get of a secret already
	// encrypted to it.
	for _, rs := range recipientStrings {
		if _, err := parseRecipientString(rs); err != nil {
			return nil, fmt.Errorf("%w: recipient %q: %w", ErrInvalidConfig, rs, err)
		}
	}

	// Check for existing vault by name across every type — names are
	// unique repo-wide, since callers reference them without a type
	// prefix.
	if existing, _ := s.findConfigByName(name); existing != nil {
		return nil, fmt.Errorf("%w: %q (type %s)", ErrVaultAlreadyExists, name, existing.Type)
	}

	id, err := newVaultID()
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		ID:       id,
		Name:     name,
		Type:     vaultType,
		Settings: map[string]any{
			"recipients": stringsToAny(recipientStrings),
		},
	}

	if err := s.writeConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// stringsToAny widens []string to []any for storage in
// map[string]any — yaml.v3 round-trips slices as []any and a
// hand-written []string would deserialize back as []any anyway,
// causing surprising type mismatches. Keep both ends []any.
func stringsToAny(s []string) []any {
	out := make([]any, len(s))
	for i, v := range s {
		out[i] = v
	}
	return out
}

// Open returns the Provider for the named vault. Returns
// ErrVaultNotFound if no vault by that name exists. The provider's
// IdentityResolver is the one the Store was constructed with — pass
// a non-nil resolver to NewStore if Get is meant to work.
//
// Dispatch goes through the backend registry, so a vault config
// pointing at a type registered behind a build tag (e.g. `sops`,
// `aws-sm`) opens fine if the binary includes that backend, and
// returns a clear ErrInvalidConfig if it doesn't.
func (s *Store) Open(name string) (Provider, error) {
	cfg, err := s.findConfigByName(name)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("%w: %q", ErrVaultNotFound, name)
	}
	return newProviderFromConfig(s.repoPath, cfg, s.identities)
}

// List returns every vault config in the repository, sorted by name.
// Used by `kvlt vault list` and by the migrate command's preflight.
func (s *Store) List() ([]*Config, error) {
	vaultsRoot := filepath.Join(s.repoPath, "vaults")
	typeDirs, err := os.ReadDir(vaultsRoot)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []*Config{}, nil
		}
		return nil, fmt.Errorf("list vault types in %q: %w", vaultsRoot, err)
	}

	out := []*Config{}
	for _, td := range typeDirs {
		if !td.IsDir() {
			continue
		}
		typeDir := filepath.Join(vaultsRoot, td.Name())
		entries, err := os.ReadDir(typeDir)
		if err != nil {
			return nil, fmt.Errorf("list vaults in %q: %w", typeDir, err)
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			cfg, err := s.readConfig(filepath.Join(typeDir, e.Name()))
			if err != nil {
				return nil, err
			}
			out = append(out, cfg)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// findConfigByName scans the vaults/ tree for a vault with the given
// name. Returns (nil, nil) when no match exists — distinct from an
// I/O error. Names are unique repo-wide, so the first match is the
// only match.
func (s *Store) findConfigByName(name string) (*Config, error) {
	configs, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, c := range configs {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, nil
}

// writeConfig serializes cfg to {repoPath}/vaults/{type}/{id}.yaml
// using the same atomic-write helper LocalProvider uses, so a half-
// written config can never confuse a future Open. Parent
// directories are created with mode 0700 since the config holds
// recipient pubkeys (not secret, but worth keeping owner-only by
// default for repos that aren't checked into git).
func (s *Store) writeConfig(cfg *Config) error {
	dir := filepath.Join(s.repoPath, "vaults", cfg.Type)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create vaults dir %q: %w", dir, err)
	}
	path := filepath.Join(dir, cfg.ID+".yaml")

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal vault config: %w", err)
	}
	return writeFileAtomic(path, data, 0o600)
}

// readConfig loads and validates a single vault config file. Lightly
// strict: empty name/type/id are treated as ErrInvalidConfig so
// hand-edited bad files surface immediately rather than blowing up
// downstream.
func (s *Store) readConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read vault config %q: %w", path, err)
	}
	cfg := &Config{}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("%w: parse vault config %q: %w", ErrInvalidConfig, path, err)
	}
	switch {
	case cfg.ID == "":
		return nil, fmt.Errorf("%w: vault config %q has empty id", ErrInvalidConfig, path)
	case cfg.Name == "":
		return nil, fmt.Errorf("%w: vault config %q has empty name", ErrInvalidConfig, path)
	case cfg.Type == "":
		return nil, fmt.Errorf("%w: vault config %q has empty type", ErrInvalidConfig, path)
	}
	return cfg, nil
}

// validateVaultName rejects names that would be unsafe as path
// components or confusing in CLI output — same family as
// validateSecretKey for secrets, but allowing dashes and underscores
// since vault names are typically dev/staging/prod or app-prod.
func validateVaultName(name string) error {
	switch {
	case name == "":
		return fmt.Errorf("%w: vault name is empty", ErrInvalidConfig)
	case strings.ContainsAny(name, "/\\"):
		return fmt.Errorf("%w: vault name %q contains a path separator", ErrInvalidConfig, name)
	case strings.HasPrefix(name, "."):
		return fmt.Errorf("%w: vault name %q must not start with a dot", ErrInvalidConfig, name)
	}
	return nil
}

// newVaultID returns a 16-byte random identifier rendered as 32 hex
// chars. We avoid pulling in a UUID library for what's effectively a
// random string with no parsing requirements — the only consumer is
// the filename `{id}.yaml`, and humans never type these.
func newVaultID() (string, error) {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return "", fmt.Errorf("generate vault ID: %w", err)
	}
	return hex.EncodeToString(b[:]), nil
}

// parseRecipientString parses one stored recipient back into an
// age.Recipient. Tries SSH first (since SSH is the dominant case
// for kvlt), falls back to age-native parsing. Returns
// ErrInvalidConfig wrapped if neither shape matches.
func parseRecipientString(s string) (age.Recipient, error) {
	s = strings.TrimSpace(s)
	if r, err := agessh.ParseRecipient(s); err == nil {
		return r, nil
	}
	if r, err := age.ParseX25519Recipient(s); err == nil {
		return r, nil
	}
	return nil, fmt.Errorf("not a valid SSH or age recipient")
}
