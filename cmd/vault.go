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
	"github.com/spf13/viper"

	"github.com/retr0h/kvlt/internal/cli"
	"github.com/retr0h/kvlt/pkg/kvlt"
)

// vaultCmd is the parent for vault lifecycle subcommands (create,
// list, info). Secret-level verbs (put/get/list-keys) sit at the
// top level for ergonomic muscle memory — `kvlt get dev API_KEY`
// instead of `kvlt secret get dev API_KEY`.
var vaultCmd = &cobra.Command{
	Use:   "vault",
	Short: "Create, inspect, and list vaults",
}

// newStore builds the public-API Store the CLI verbs operate
// against, wiring in the TTY-backed passphrase prompt as the
// IdentityResolver. Keeping this in one place ensures every verb
// uses the same identity-discovery rules — and that the rules live
// behind the public IdentityResolver / DefaultIdentityResolver
// surface so external Go callers can use the same construction.
func newStore() (*kvlt.Store, error) {
	repo := viper.GetString("repo.path")
	resolver := kvlt.DefaultIdentityResolver(cli.PassphrasePromptTTY)
	return kvlt.NewStore(repo, resolver)
}

func init() {
	rootCmd.AddCommand(vaultCmd)
}

// vaultCreateCmd creates a new local vault encrypted to one or more
// recipient SSH public keys. The default recipient is the current
// user's ed25519 key (~/.ssh/id_ed25519.pub) since that's what
// every developer has by default; --recipient can be passed
// repeatedly to encrypt to additional teammates / CI keys.
var vaultCreateCmd = &cobra.Command{
	Use:   "create <type> <name>",
	Short: "Create a new vault (type must be `local`)",
	Long: `Create a new vault under the current repository.

Supported types:
  local   age-encrypted local files (default zero-cloud backend)

Recipients default to the current user's ~/.ssh/id_ed25519.pub. Pass
--recipient repeatedly to encrypt to additional teammates / CI keys
— any one of the matching SSH private keys can decrypt.`,
	Args: cobra.ExactArgs(2),
	RunE: runVaultCreate,
}

var vaultCreateRecipients []string

func init() {
	vaultCreateCmd.Flags().StringSliceVarP(&vaultCreateRecipients, "recipient", "r", nil,
		"SSH or age public key, or path to a .pub file (repeatable). Defaults to ~/.ssh/id_ed25519.pub.")
	vaultCmd.AddCommand(vaultCreateCmd)
}

func runVaultCreate(_ *cobra.Command, args []string) error {
	vaultType, name := args[0], args[1]
	// `local` is the user-facing alias; pkg/kvlt internally calls it
	// local_encryption to match swamp's identifier.
	if vaultType == "local" {
		vaultType = kvlt.TypeLocalEncryption
	}

	recipients, err := resolveRecipientFlags(vaultCreateRecipients)
	if err != nil {
		return err
	}

	store, err := newStore()
	if err != nil {
		return err
	}
	cfg, err := store.Create(name, vaultType, recipients)
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
			return nil, fmt.Errorf("no recipient passed and %s does not exist — pass --recipient or run `ssh-keygen -t ed25519`", defaultPath)
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

// vaultListCmd prints every configured vault in the repository.
// One line per vault: name, type, recipient count. The id and full
// recipient list belong in `vault info <name>`, where the operator
// has explicitly asked.
var vaultListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all vaults in the repository",
	RunE:  runVaultList,
}

func init() { vaultCmd.AddCommand(vaultListCmd) }

func runVaultList(_ *cobra.Command, _ []string) error {
	store, err := newStore()
	if err != nil {
		return err
	}
	configs, err := store.List()
	if err != nil {
		return err
	}
	if len(configs) == 0 {
		fmt.Fprintln(os.Stderr, "no vaults configured — run `kvlt vault create local <name>`")
		return nil
	}
	for _, c := range configs {
		recCount := 0
		if rs, ok := c.Settings["recipients"].([]any); ok {
			recCount = len(rs)
		}
		fmt.Printf("%-20s  %-20s  %d recipient(s)\n", c.Name, c.Type, recCount)
	}
	return nil
}

// vaultInfoCmd prints one vault's full config — id, type, every
// recipient's canonical string. Used by humans deciding whether a
// vault is the right one to put a secret in, and by audit scripts
// confirming who can decrypt before a sensitive add.
var vaultInfoCmd = &cobra.Command{
	Use:   "info <name>",
	Short: "Show one vault's id, type, and recipient list",
	Args:  cobra.ExactArgs(1),
	RunE:  runVaultInfo,
}

func init() { vaultCmd.AddCommand(vaultInfoCmd) }

func runVaultInfo(_ *cobra.Command, args []string) error {
	store, err := newStore()
	if err != nil {
		return err
	}
	configs, err := store.List()
	if err != nil {
		return err
	}
	for _, c := range configs {
		if c.Name != args[0] {
			continue
		}
		fmt.Printf("name:        %s\n", c.Name)
		fmt.Printf("type:        %s\n", c.Type)
		fmt.Printf("id:          %s\n", c.ID)
		fmt.Println("recipients:")
		if rs, ok := c.Settings["recipients"].([]any); ok {
			for _, r := range rs {
				fmt.Printf("  - %v\n", r)
			}
		}
		return nil
	}
	return fmt.Errorf("%w: %q", kvlt.ErrVaultNotFound, args[0])
}
