// Copyright (c) 2026 John Dewey
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to
// deal in the Software without restriction, including without limitation the
// rights to use, copy, modify, merge, publish, distribute, sublicense, and/or
// sell copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
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
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"filippo.io/age"
	"filippo.io/age/agessh"
	"golang.org/x/crypto/ssh"
)

// PassphrasePrompt is invoked when a passphrase-protected SSH private
// key needs to be unlocked. Implementations should read from
// /dev/tty (not stdin/stderr) so prompts work inside command
// substitutions like $(kvlt get …). Returning a nil byte slice or an
// error aborts the unlock attempt.
//
// The provided keyPath is the path of the key file requiring the
// passphrase, suitable for inclusion in a prompt ("Enter passphrase
// for /home/user/.ssh/id_ed25519: "). Implementations are free to
// ignore it.
type PassphrasePrompt func(keyPath string) ([]byte, error)

// DefaultSSHKeyPaths returns the conventional SSH private-key
// locations under $HOME/.ssh, in priority order. Files that don't
// exist are filtered out — callers can pass the resulting slice
// directly to LoadSSHIdentities. Order matches what `ssh` itself
// tries: ed25519 first (modern default), then ecdsa, then rsa.
func DefaultSSHKeyPaths() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	candidates := []string{
		filepath.Join(home, ".ssh", "id_ed25519"),
		filepath.Join(home, ".ssh", "id_ecdsa"),
		filepath.Join(home, ".ssh", "id_rsa"),
	}
	available := make([]string, 0, len(candidates))
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			available = append(available, p)
		}
	}
	return available
}

// LoadSSHIdentity loads a single SSH private key as an age identity.
// If the key is unencrypted, it loads silently. If the key is
// passphrase-protected, prompt is called to obtain the passphrase;
// pass nil for prompt to refuse encrypted keys outright (useful in
// non-interactive contexts where popping a TTY prompt would hang).
//
// Path resolution and OS-level file errors are wrapped with the
// keyPath so callers can produce actionable error messages without
// reaching into the wrapped chain.
func LoadSSHIdentity(keyPath string, prompt PassphrasePrompt) (age.Identity, error) {
	pemBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("read SSH key %q: %w", keyPath, err)
	}

	// Fast path: unencrypted key.
	id, err := agessh.ParseIdentity(pemBytes)
	if err == nil {
		return id, nil
	}

	// agessh returns a typed error when the key is passphrase-
	// protected — we look for that, and any other error means a real
	// parse failure (truncated key, unsupported type, …).
	var passErr *ssh.PassphraseMissingError
	if !errors.As(err, &passErr) {
		return nil, fmt.Errorf("parse SSH key %q: %w", keyPath, err)
	}
	if prompt == nil {
		return nil, fmt.Errorf(
			"%w: SSH key %q is passphrase-protected and no prompt was provided",
			ErrAuthFailed,
			keyPath,
		)
	}

	// PassphraseMissingError carries the SSH public key, which agessh
	// needs to construct the encrypted identity. We hand it the same
	// PEM bytes plus a callback that turns our PassphrasePrompt into
	// the byte-slice signature agessh expects.
	if passErr.PublicKey == nil {
		return nil, fmt.Errorf(
			"parse SSH key %q: passphrase-protected but public key not recoverable",
			keyPath,
		)
	}
	encID, err := agessh.NewEncryptedSSHIdentity(
		passErr.PublicKey,
		pemBytes,
		func() ([]byte, error) { return prompt(keyPath) },
	)
	if err != nil {
		return nil, fmt.Errorf("init encrypted SSH identity %q: %w", keyPath, err)
	}
	return encID, nil
}

// LoadSSHIdentities loads every key in keyPaths as age identities,
// in order. Missing files (ENOENT) are skipped silently; other read
// or parse errors are returned. The skip-on-missing behavior matches
// the typical UX expectation that "I have id_ed25519 but not id_rsa"
// shouldn't be an error.
//
// Identities are returned in the same order the paths were given;
// age tries them sequentially during decrypt and stops at the first
// match, so put your most-common key first.
func LoadSSHIdentities(keyPaths []string, prompt PassphrasePrompt) ([]age.Identity, error) {
	out := make([]age.Identity, 0, len(keyPaths))
	for _, p := range keyPaths {
		id, err := LoadSSHIdentity(p, prompt)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// ParseSSHRecipients parses one or more SSH public keys (in the
// standard `ssh-ed25519 AAAA… [comment]` line format, separated by
// newlines) into age recipients. Lines that are blank or start with
// `#` are treated as comments. This is the format `~/.ssh/*.pub`
// files use, so callers can `os.ReadFile` an `id_ed25519.pub` and
// pass the contents straight in.
func ParseSSHRecipients(authorizedKeys []byte) ([]age.Recipient, error) {
	out := []age.Recipient{}
	lineNum := 0
	for raw := range strings.SplitSeq(string(authorizedKeys), "\n") {
		lineNum++
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		r, err := agessh.ParseRecipient(line)
		if err != nil {
			return nil, fmt.Errorf(
				"%w: parse SSH recipient on line %d: %w",
				ErrInvalidConfig,
				lineNum,
				err,
			)
		}
		out = append(out, r)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no SSH recipients found in input", ErrInvalidConfig)
	}
	return out, nil
}

// DefaultIdentityResolver returns an IdentityResolver that, on each
// invocation, loads identities from the conventional ~/.ssh/id_*
// locations using the provided passphrase prompt. The prompt may be
// nil for non-interactive contexts (CI with the key already
// unlocked, or test environments) — in that case, encrypted keys
// are skipped with an explicit error.
//
// This is a sensible default for CLI use where the caller hasn't
// configured anything more specific. Library users with stricter
// requirements (a pinned key path, a custom keyring, ssh-agent
// only) should construct their own resolver — the type alias is
// intentionally thin.
//
// ssh-agent integration is intentionally omitted from this default;
// the agent path requires extra glue (agessh's signer-based identity
// works for ed25519 but the wiring is non-trivial) and is best
// landed in a follow-up commit so the file-key path can ship first.
func DefaultIdentityResolver(prompt PassphrasePrompt) IdentityResolver {
	return func() ([]age.Identity, error) {
		ids, err := LoadSSHIdentities(DefaultSSHKeyPaths(), prompt)
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 {
			return nil, fmt.Errorf(
				"%w: no SSH identities found in ~/.ssh — generate one with `ssh-keygen -t ed25519`",
				ErrAuthFailed,
			)
		}
		return ids, nil
	}
}
