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

package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/retr0h/kvlt/internal/cli"
)

var (
	vaultCreateType       string
	vaultCreateName       string
	vaultCreatePublicKeys []string
)

// vaultCreateCmd creates a new vault. Every input is a flag — no
// positional args — so call sites are self-documenting (`--type
// local --name dev` reads unambiguously) and adding new optional
// inputs later is non-breaking. The default --type is `local`
// because that's the zero-cloud backend in the base binary.
var vaultCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new vault",
	Long: `Create a new vault under the current repository.

Supported types:
  local   age-encrypted local files (default zero-cloud backend)

Public keys default to the current user's ~/.ssh/id_ed25519.pub. Pass
--public-key repeatedly to encrypt to additional teammates / CI keys
— any one of the matching SSH private keys can decrypt.

Examples:
  kvlt vault create --name dev
  kvlt vault create --type local --name prod --public-key ~/.ssh/team.pub
  kvlt vault create --name shared -p ~/.ssh/alice.pub -p ~/.ssh/bob.pub`,
	Args: cobra.NoArgs,
	RunE: runVaultCreate,
}

func init() {
	vaultCreateCmd.Flags().StringVarP(&vaultCreateType, "type", "t", "local",
		"backend type — `local` is the only one in the base binary; cloud backends require build tags")
	vaultCreateCmd.Flags().StringVarP(&vaultCreateName, "name", "n", "",
		"vault name (required) — referenced by every later put/get/list")
	vaultCreateCmd.Flags().StringSliceVarP(&vaultCreatePublicKeys, "public-key", "p", nil,
		"SSH or age public key, or path to a .pub file (repeatable). Defaults to ~/.ssh/id_ed25519.pub")
	_ = vaultCreateCmd.MarkFlagRequired("name")
	vaultCmd.AddCommand(vaultCreateCmd)
}

func runVaultCreate(_ *cobra.Command, _ []string) error {
	recipients, err := resolvePublicKeyFlags(vaultCreatePublicKeys)
	if err != nil {
		return err
	}

	store, err := newStore()
	if err != nil {
		return err
	}
	// pkg/kvlt's CanonicalizeType handles the `local` → `local_encryption`
	// alias and the `local-encryption` typo. Anything else falls
	// through to the registry's "unknown type" error.
	cfg, err := store.Create(vaultCreateName, vaultCreateType, recipients)
	if err != nil {
		return err
	}

	// Human success line — themed; the structured slog line below is
	// for machine consumers (logging pipelines, CI). Two channels, one
	// event.
	out := os.Stdout
	_, _ = fmt.Fprintln(out, cli.Success(out, fmt.Sprintf("vault %s created (%s, %d recipient(s))",
		cli.Accent(out, cfg.Name),
		cli.Mute(out, cfg.Type),
		len(recipients))))

	logger.Info("vault created",
		"name", cfg.Name,
		"type", cfg.Type,
		"id", cfg.ID,
		"recipients", len(recipients),
	)
	return nil
}

// resolvePublicKeyFlags turns --public-key flag values (or the
// default ~/.ssh/id_ed25519.pub fallback) into the canonical
// recipient strings Store.Create expects. Each flag value is
// either an authorized_keys-style SSH line, an `age1…` recipient,
// or a path to a `.pub` file containing one or more public keys —
// we sniff for the file first. Comments and blank lines inside
// .pub files are stripped so the YAML stores only meaningful
// public-key strings.
func resolvePublicKeyFlags(flags []string) ([]string, error) {
	if len(flags) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir for default recipient: %w", err)
		}
		defaultPath := filepath.Join(home, ".ssh", "id_ed25519.pub")
		if _, err := os.Stat(defaultPath); err != nil {
			return nil, fmt.Errorf(
				"no public key passed and %s does not exist — pass --public-key or run `ssh-keygen -t ed25519`",
				defaultPath,
			)
		}
		flags = []string{defaultPath}
	}

	out := make([]string, 0, len(flags))
	for _, f := range flags {
		var raw string
		if info, err := os.Stat(f); err == nil && !info.IsDir() {
			b, err := os.ReadFile(f)
			if err != nil {
				return nil, fmt.Errorf("read recipient file %q: %w", f, err)
			}
			raw = string(b)
		} else {
			raw = f
		}
		for line := range strings.SplitSeq(raw, "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no public keys provided")
	}
	return out, nil
}
