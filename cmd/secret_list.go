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

package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	secretListVault string
	secretListJSON  bool
)

// secretListCmd prints every secret key currently stored in a
// vault. Values are NEVER returned — that's a vault-wide invariant
// inside pkg/kvlt and the CLI inherits it. List works without
// unlocking the vault (no IdentityResolver consulted) so an audit
// "what's in here?" doesn't need to pop a passphrase prompt.
//
// Default output is one key per line — pipe-friendly with `wc -l`,
// `grep`, `xargs`. --json swaps in {"vault","keys":[…]} for scripts.
var secretListCmd = &cobra.Command{
	Use:   "list",
	Short: "List the secret keys stored in a vault (names only, never values)",
	Args:  cobra.NoArgs,
	RunE:  runSecretList,
}

func init() {
	secretListCmd.Flags().StringVarP(&secretListVault, "vault", "v", "",
		"vault to list (required)")
	secretListCmd.Flags().BoolVar(&secretListJSON, "json", false,
		"emit {vault, keys} JSON instead of one key per line")
	_ = secretListCmd.MarkFlagRequired("vault")
	secretCmd.AddCommand(secretListCmd)
}

func runSecretList(_ *cobra.Command, _ []string) error {
	store, err := newStore()
	if err != nil {
		return err
	}
	provider, err := store.Open(secretListVault)
	if err != nil {
		return mapGetError(err)
	}
	keys, err := provider.List(context.Background())
	if err != nil {
		return err
	}

	if secretListJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"vault": secretListVault,
			"keys":  keys,
		})
	}
	for _, k := range keys {
		fmt.Println(k)
	}
	return nil
}
