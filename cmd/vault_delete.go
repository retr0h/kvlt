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

	"github.com/spf13/cobra"

	"github.com/retr0h/kvlt/internal/cli"
)

var (
	vaultDeleteName  string
	vaultDeleteForce bool
)

// vaultDeleteCmd removes a vault and every secret stored in it. The
// vault config YAML AND every encrypted payload under
// .kvlt/secrets/<type>/<name>/ are unlinked. The action is
// irreversible — there's no soft-delete or trash. Prompts on
// /dev/tty by default; --force skips the prompt for scripts.
var vaultDeleteCmd = &cobra.Command{
	Use:     "delete",
	Aliases: []string{"rm", "destroy"},
	Short:   "Delete a vault and every secret it contains",
	Long: `Remove a vault: the config YAML under .kvlt/vaults/ AND every
encrypted secret under .kvlt/secrets/<type>/<name>/. Irreversible.

Prompts for confirmation by default. Use --force in scripts.

  kvlt vault delete --name dev
  kvlt vault delete --name dev --force`,
	Args: cobra.NoArgs,
	RunE: runVaultDelete,
}

func init() {
	vaultDeleteCmd.Flags().StringVarP(&vaultDeleteName, "name", "n", "",
		"vault name to delete (required)")
	vaultDeleteCmd.Flags().BoolVar(&vaultDeleteForce, "force", false,
		"skip the confirmation prompt")
	_ = vaultDeleteCmd.MarkFlagRequired("name")
	vaultCmd.AddCommand(vaultDeleteCmd)
}

func runVaultDelete(_ *cobra.Command, _ []string) error {
	store, err := newStoreForRead("")
	if err != nil {
		return err
	}

	if !vaultDeleteForce {
		ok, cerr := cli.ConfirmDestructive(fmt.Sprintf(
			"Delete vault %q AND every secret it contains?", vaultDeleteName,
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

	if err := store.Delete(vaultDeleteName); err != nil {
		return mapGetError(err)
	}

	out := os.Stdout
	_, _ = fmt.Fprintln(out, cli.Success(out, fmt.Sprintf("deleted vault %s",
		cli.Accent(out, vaultDeleteName))))
	return nil
}
