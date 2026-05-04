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

	"github.com/retr0h/kvlt/pkg/kvlt"
)

var (
	vaultCreateType       string
	vaultCreateName       string
	vaultCreateRecipients []string
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

Recipients default to the current user's ~/.ssh/id_ed25519.pub. Pass
--recipient repeatedly to encrypt to additional teammates / CI keys
— any one of the matching SSH private keys can decrypt.

Examples:
  kvlt vault create --name dev
  kvlt vault create --type local --name prod --recipient ~/.ssh/team.pub
  kvlt vault create --name shared -r ~/.ssh/alice.pub -r ~/.ssh/bob.pub`,
	Args: cobra.NoArgs,
	RunE: runVaultCreate,
}

func init() {
	vaultCreateCmd.Flags().StringVarP(&vaultCreateType, "type", "t", "local",
		"backend type — `local` is the only one in the base binary; cloud backends require build tags")
	vaultCreateCmd.Flags().StringVarP(&vaultCreateName, "name", "n", "",
		"vault name (required) — referenced by every later put/get/list")
	vaultCreateCmd.Flags().StringSliceVarP(&vaultCreateRecipients, "recipient", "r", nil,
		"SSH or age public key, or path to a .pub file (repeatable). Defaults to ~/.ssh/id_ed25519.pub")
	_ = vaultCreateCmd.MarkFlagRequired("name")
	vaultCmd.AddCommand(vaultCreateCmd)
}

func runVaultCreate(_ *cobra.Command, _ []string) error {
	vaultType := vaultCreateType
	// `local` is the user-facing alias; pkg/kvlt internally uses
	// local_encryption to match swamp's identifier so vaults/ paths
	// and YAML files are wire-compatible. Reject any other spelling
	// up front with a hint — `local-encryption` (hyphen) is the
	// common typo.
	switch vaultType {
	case "local":
		vaultType = kvlt.TypeLocalEncryption
	case kvlt.TypeLocalEncryption:
		// allowed for round-trip from existing on-disk configs
	case "local-encryption":
		return fmt.Errorf(
			"did you mean `--type local`? (kvlt accepts `local` as the type name; the on-disk identifier is `local_encryption` with an underscore)",
		)
	}

	recipients, err := resolveRecipientFlags(vaultCreateRecipients)
	if err != nil {
		return err
	}

	store, err := newStore()
	if err != nil {
		return err
	}
	cfg, err := store.Create(vaultCreateName, vaultType, recipients)
	if err != nil {
		return err
	}

	logger.Info("vault created",
		"name", cfg.Name,
		"type", cfg.Type,
		"id", cfg.ID,
		"recipients", len(recipients),
	)
	return nil
}

// resolveRecipientFlags turns --recipient flag values (or the
// default ~/.ssh/id_ed25519.pub fallback) into the canonical
// recipient strings Store.Create expects. Each flag value is
// either an authorized_keys-style SSH line, an `age1…` recipient,
// or a path to a `.pub` file containing one or more recipients —
// we sniff for the file first. Comments and blank lines inside
// .pub files are stripped so the YAML stores only meaningful
// recipient strings.
func resolveRecipientFlags(flags []string) ([]string, error) {
	if len(flags) == 0 {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("resolve home dir for default recipient: %w", err)
		}
		defaultPath := filepath.Join(home, ".ssh", "id_ed25519.pub")
		if _, err := os.Stat(defaultPath); err != nil {
			return nil, fmt.Errorf(
				"no recipient passed and %s does not exist — pass --recipient or run `ssh-keygen -t ed25519`",
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
		return nil, fmt.Errorf("no recipients provided")
	}
	return out, nil
}
