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
// recipients defines who can decrypt secrets put into this vault. At
// least one is required — a vault encrypted to nobody is treated as
// a misconfiguration, since nothing put into it could ever be read
// back. Recipients are stored as their canonical string form
// (`ssh-ed25519 AAA…`, `age1…`) in the YAML so a reader can audit
// the recipient list without parsing key blobs.
func (s *Store) Create(name, vaultType string, recipients []age.Recipient) (*Config, error) {
	if err := validateVaultName(name); err != nil {
		return nil, err
	}
	if vaultType != TypeLocalEncryption {
		return nil, fmt.Errorf("%w: unknown vault type %q", ErrInvalidConfig, vaultType)
	}
	if len(recipients) == 0 {
		return nil, fmt.Errorf("%w: at least one recipient is required", ErrInvalidConfig)
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
		Settings: make(map[string]any),
	}
	cfg.Settings["recipients"] = serializeRecipients(recipients)

	if err := s.writeConfig(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Open returns the Provider for the named vault. Returns
// ErrVaultNotFound if no vault by that name exists. The provider's
// IdentityResolver is the one the Store was constructed with — pass
// a non-nil resolver to NewStore if Get is meant to work.
func (s *Store) Open(name string) (Provider, error) {
	cfg, err := s.findConfigByName(name)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("%w: %q", ErrVaultNotFound, name)
	}

	switch cfg.Type {
	case TypeLocalEncryption:
		return s.openLocal(cfg)
	default:
		return nil, fmt.Errorf("%w: unknown vault type %q in config", ErrInvalidConfig, cfg.Type)
	}
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

// openLocal materializes a LocalProvider from a config plus the
// Store's IdentityResolver. The vault directory under
// .kvlt/secrets/{type}/{name}/ is created lazily by NewLocalProvider.
func (s *Store) openLocal(cfg *Config) (*LocalProvider, error) {
	recList, _ := cfg.Settings["recipients"].([]any)
	if len(recList) == 0 {
		return nil, fmt.Errorf("%w: vault %q has no recipients in config", ErrInvalidConfig, cfg.Name)
	}
	recipients := make([]age.Recipient, 0, len(recList))
	for _, raw := range recList {
		s, ok := raw.(string)
		if !ok {
			return nil, fmt.Errorf("%w: vault %q has a non-string recipient entry", ErrInvalidConfig, cfg.Name)
		}
		r, err := parseRecipientString(s)
		if err != nil {
			return nil, fmt.Errorf("%w: vault %q recipient %q: %w", ErrInvalidConfig, cfg.Name, s, err)
		}
		recipients = append(recipients, r)
	}

	dir := filepath.Join(s.repoPath, ".kvlt", "secrets", cfg.Type, cfg.Name)
	return NewLocalProvider(cfg.Name, dir, recipients, s.identities)
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

// serializeRecipients renders a slice of age.Recipients to their
// canonical string form for storage in YAML. SSH recipients
// stringify as `ssh-ed25519 AAA…`; age-native recipients as
// `age1…`. Every concrete type that age exposes implements
// fmt.Stringer.
func serializeRecipients(recipients []age.Recipient) []string {
	out := make([]string, 0, len(recipients))
	for _, r := range recipients {
		out = append(out, fmt.Sprint(r))
	}
	return out
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
