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
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"filippo.io/age"
)

// init registers the local-encryption backend with the factory
// registry. Done in init() so the base binary always has it
// available — `local` is the default backend, never gated by build
// tags. Cloud backends in their own files do the same under
// //go:build <tag> guards so they're only present when the operator
// asked for them.
func init() {
	RegisterBackend(TypeLocalEncryption, newLocalProviderFromConfig)
}

// newLocalProviderFromConfig adapts a stored Config into a
// LocalProvider. Recipients are read from cfg.Settings["recipients"]
// (stored by Store.Create as the canonical strings the user passed
// in) and parsed back into age.Recipients via parseRecipientString.
// The on-disk vault directory under .kvlt/secrets/{type}/{name}/ is
// derived from the repo root + config — keeping path computation in
// one place so a future migrate command can compute paths the same
// way.
func newLocalProviderFromConfig(repoPath string, cfg *Config, identities IdentityResolver) (Provider, error) {
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
	dir := filepath.Join(repoPath, ".kvlt", "secrets", cfg.Type, cfg.Name)
	return NewLocalProvider(cfg.Name, dir, recipients, identities)
}

const (
	// localSecretSuffix is appended to every secret's filename. The
	// .age extension matches what `age` itself uses, so the file
	// reads as exactly what it is — an age-encrypted blob — and
	// `age` / `rage` can decrypt it directly with the right
	// identity, even if kvlt is unavailable.
	localSecretSuffix = ".age"

	// localFileMode keeps secret files owner-only. The contents are
	// already encrypted, but a tighter mode reduces accidental
	// exposure on shared filesystems and during backups.
	localFileMode fs.FileMode = 0o600

	// localDirMode keeps the vault directory owner-only.
	localDirMode fs.FileMode = 0o700
)

// IdentityResolver returns the age identities to try for decryption.
// Called lazily — only when a Get actually needs to decrypt — so a
// passphrase-protected key never prompts during operations like List
// or vault inspection. Resolvers can read ssh-agent, parse files,
// pop a TTY prompt, or hand back a static slice for tests.
//
// Returning an empty slice is treated as "no identities available";
// callers that want a clearer error should return one explicitly.
type IdentityResolver func() ([]age.Identity, error)

// LocalProvider is the local-disk backend. Encryption is delegated to
// filippo.io/age so kvlt does not own primitive crypto: encrypted
// blobs are valid age containers, listing recipients up front, and
// decrypt requires possession of one of the matching identities
// (typically an SSH private key, optionally an age-native key).
//
// The protection model is therefore the protection model of the
// identities themselves — passphrase + ssh-agent + (on macOS, via
// secretive) the Secure Enclave. The .age files at rest are useless
// without those identities.
type LocalProvider struct {
	// name is the user-defined vault name (returned by Name()).
	name string

	// dir is the per-vault directory on disk. Secrets live directly
	// inside as {key}.age — no nested layout.
	dir string

	// recipients is the public-key set every Put encrypts to. A vault
	// with multiple recipients lets any one of the matching
	// identities decrypt it (the multi-recipient case for teams).
	recipients []age.Recipient

	// identities resolves on demand to the set of identities used for
	// Get. Lazy so List/Name don't trigger passphrase prompts.
	identities IdentityResolver
}

// NewLocalProvider constructs a LocalProvider rooted at dir, encrypting
// every Put to the given recipients. The identityResolver is called
// only on Get; pass nil if the vault is write-only from this process.
//
// dir is the per-vault directory; the caller is responsible for the
// repo-level path layout (typically Store.Open handles this). Pass
// at least one recipient — a vault you can't decrypt anything with
// is allowed (write-only patterns), but a vault encrypted to nobody
// is rejected as a configuration error.
func NewLocalProvider(name, dir string, recipients []age.Recipient, identityResolver IdentityResolver) (*LocalProvider, error) {
	switch {
	case name == "":
		return nil, fmt.Errorf("%w: vault name is empty", ErrInvalidConfig)
	case dir == "":
		return nil, fmt.Errorf("%w: vault directory is empty", ErrInvalidConfig)
	case len(recipients) == 0:
		return nil, fmt.Errorf("%w: vault %q has no recipients — nothing could decrypt new secrets", ErrInvalidConfig, name)
	}
	if err := os.MkdirAll(dir, localDirMode); err != nil {
		return nil, fmt.Errorf("create vault dir %q: %w", dir, err)
	}
	return &LocalProvider{
		name:       name,
		dir:        dir,
		recipients: recipients,
		identities: identityResolver,
	}, nil
}

// Name returns the user-defined vault name.
func (p *LocalProvider) Name() string { return p.name }

// Recipients returns the configured age recipients (read-only). Used
// by `kvlt vault info` to print fingerprints; the slice is not safe
// to mutate.
func (p *LocalProvider) Recipients() []age.Recipient { return p.recipients }

// Get reads {dir}/{key}.age, asks the IdentityResolver for available
// identities, and decrypts. ErrKeyNotFound is returned when the file
// is missing — distinct from a decrypt failure (wrong identity, file
// tampered with) so callers can branch on remediation.
func (p *LocalProvider) Get(_ context.Context, key string) (string, error) {
	if err := validateSecretKey(key); err != nil {
		return "", err
	}
	if p.identities == nil {
		return "", fmt.Errorf("%w: vault %q has no identity resolver configured", ErrAuthFailed, p.name)
	}

	blob, err := os.ReadFile(p.secretPath(key))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("%w: vault=%q key=%q", ErrKeyNotFound, p.name, key)
		}
		return "", fmt.Errorf("read secret %q in vault %q: %w", key, p.name, err)
	}

	identities, err := p.identities()
	if err != nil {
		return "", fmt.Errorf("%w: vault %q: %w", ErrAuthFailed, p.name, err)
	}
	if len(identities) == 0 {
		return "", fmt.Errorf("%w: vault %q: no identities available for decrypt", ErrAuthFailed, p.name)
	}

	r, err := age.Decrypt(bytes.NewReader(blob), identities...)
	if err != nil {
		// age returns "no identity matched any of the recipients"
		// when none of our identities own a stanza in the header.
		// Wrap as ErrAuthFailed so callers can branch the same way
		// they do for backend auth errors.
		return "", fmt.Errorf("%w: vault %q key %q: %w", ErrAuthFailed, p.name, key, err)
	}

	plaintext, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("decrypt secret %q in vault %q: %w", key, p.name, err)
	}
	return string(plaintext), nil
}

// Put encrypts value to all configured recipients and writes the
// result atomically. The on-disk file is a valid age container and
// can be decrypted by `age -d` / `rage -d` directly with one of the
// recipient identities, even if kvlt is unavailable. That's a
// deliberate property: kvlt is a convenience layer, not a
// crypto-format lock-in.
func (p *LocalProvider) Put(_ context.Context, key, value string) error {
	if err := validateSecretKey(key); err != nil {
		return err
	}

	var buf bytes.Buffer
	w, err := age.Encrypt(&buf, p.recipients...)
	if err != nil {
		return fmt.Errorf("init age encrypt for vault %q: %w", p.name, err)
	}
	if _, err := io.WriteString(w, value); err != nil {
		_ = w.Close()
		return fmt.Errorf("encrypt secret %q in vault %q: %w", key, p.name, err)
	}
	if err := w.Close(); err != nil {
		return fmt.Errorf("finalize encrypted secret %q in vault %q: %w", key, p.name, err)
	}

	return writeFileAtomic(p.secretPath(key), buf.Bytes(), localFileMode)
}

// List returns the secret keys stored in this vault, sorted
// alphabetically. Files without the .age suffix are skipped — kvlt
// never wrote them, so they're stray and should not be advertised
// as secrets. The vault's own metadata files (recipients list, etc.)
// live in the parent vaults/ tree, not here.
func (p *LocalProvider) List(_ context.Context) ([]string, error) {
	entries, err := os.ReadDir(p.dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("list vault %q: %w", p.name, err)
	}

	keys := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		nm := e.Name()
		if !strings.HasSuffix(nm, localSecretSuffix) {
			continue
		}
		keys = append(keys, strings.TrimSuffix(nm, localSecretSuffix))
	}
	sort.Strings(keys)
	return keys, nil
}

// secretPath joins the vault directory and key with the .age suffix.
// validateSecretKey has already rejected anything that would let the
// joined path escape the vault dir.
func (p *LocalProvider) secretPath(key string) string {
	return filepath.Join(p.dir, key+localSecretSuffix)
}

// validateSecretKey rejects keys that would create an unsafe on-disk
// path. Backends are free to support richer key namespaces;
// LocalProvider keeps it boring on purpose so the filesystem can't
// bite us — no path separators, no leading dots, no empty.
func validateSecretKey(key string) error {
	switch {
	case key == "":
		return fmt.Errorf("%w: secret key is empty", ErrInvalidConfig)
	case strings.ContainsAny(key, "/\\"):
		return fmt.Errorf("%w: secret key %q contains a path separator", ErrInvalidConfig, key)
	case strings.HasPrefix(key, "."):
		return fmt.Errorf("%w: secret key %q must not start with a dot", ErrInvalidConfig, key)
	}
	return nil
}

// writeFileAtomic creates a sibling tempfile, fsyncs and renames it
// over path. Mode is applied to the tempfile before rename so the
// final file never exists with looser permissions, even briefly.
// On rename failure the tempfile is removed so the directory does
// not accumulate orphans across retries.
func writeFileAtomic(path string, data []byte, mode fs.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create tempfile in %q: %w", dir, err)
	}
	tmpName := tmp.Name()

	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
	}

	if err := tmp.Chmod(mode); err != nil {
		cleanup()
		return fmt.Errorf("chmod tempfile: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		cleanup()
		return fmt.Errorf("write tempfile: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		cleanup()
		return fmt.Errorf("fsync tempfile: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close tempfile: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("rename %q → %q: %w", tmpName, path, err)
	}
	return nil
}
