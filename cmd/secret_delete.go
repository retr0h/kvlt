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
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/retr0h/kvlt/internal/cli"
)

var (
	secretDeleteVault string
	secretDeleteKey   string
	secretDeleteForce bool
)

// secretDeleteCmd removes a single secret key from a vault. Prompts
// on /dev/tty by default since the action is destructive; --force
// skips the prompt for scripts and CI.
var secretDeleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"rm"},
	Short:   "Delete a secret key from a vault",
	Long: `Remove a single secret from a vault. The encrypted .age file is
unlinked from disk; nothing about the vault config or recipients
changes.

Prompts for confirmation by default. Use --force in scripts.

  kvlt secret delete --vault dev --key API_KEY
  kvlt secret delete --vault dev --key API_KEY --force`,
	Args: cobra.NoArgs,
	RunE: runSecretDelete,
}

func init() {
	secretDeleteCmd.Flags().StringVarP(&secretDeleteVault, "vault", "v", "",
		"target vault name (required)")
	secretDeleteCmd.Flags().StringVarP(&secretDeleteKey, "key", "k", "",
		"secret key to delete (required)")
	secretDeleteCmd.Flags().BoolVar(&secretDeleteForce, "force", false,
		"skip the confirmation prompt")
	_ = secretDeleteCmd.MarkFlagRequired("vault")
	_ = secretDeleteCmd.MarkFlagRequired("key")
	secretCmd.AddCommand(secretDeleteCmd)
}

func runSecretDelete(_ *cobra.Command, _ []string) error {
	store, err := newStoreForRead("")
	if err != nil {
		return err
	}
	provider, err := store.Open(secretDeleteVault)
	if err != nil {
		return mapGetError(err)
	}

	if !secretDeleteForce {
		ok, cerr := cli.ConfirmDestructive(fmt.Sprintf(
			"Delete secret %q from vault %q?", secretDeleteKey, secretDeleteVault,
		))
		if cerr != nil {
			return cerr
		}
		if !ok {
			out := os.Stdout
			_, _ = fmt.Fprintln(out, cli.Mute(out, "aborted"))
			return nil
		}
	}

	if err := provider.Delete(context.Background(), secretDeleteKey); err != nil {
		return mapGetError(err)
	}

	out := os.Stdout
	_, _ = fmt.Fprintln(out, cli.Success(out, fmt.Sprintf("deleted %s from vault %s",
		cli.Accent(out, secretDeleteKey),
		cli.Accent(out, secretDeleteVault))))
	return nil
}
