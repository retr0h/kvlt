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
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// listKeysCmd prints every secret key currently stored in a vault.
// Values are NEVER returned — that's a vault-wide invariant inside
// pkg/kvlt and the CLI inherits it. List works without unlocking
// the vault (no IdentityResolver is consulted), so an audit "what's
// in here?" doesn't need to pop a passphrase prompt.
//
// Default output is one key per line — pipe-friendly with `wc -l`,
// `grep`, `xargs`, etc. --json swaps in {"vault","keys":[...]}
// for scripting.
var listKeysCmd = &cobra.Command{
	Use:     "list-keys <vault>",
	Aliases: []string{"keys", "ls-keys"},
	Short:   "List the secret keys stored in a vault (no values)",
	Args:    cobra.ExactArgs(1),
	RunE:    runListKeys,
}

var listKeysJSON bool

func init() {
	listKeysCmd.Flags().BoolVar(&listKeysJSON, "json", false,
		"emit {vault, keys} JSON instead of one key per line")
	rootCmd.AddCommand(listKeysCmd)
}

func runListKeys(_ *cobra.Command, args []string) error {
	vaultName := args[0]

	store, err := newStore()
	if err != nil {
		return err
	}
	provider, err := store.Open(vaultName)
	if err != nil {
		return mapGetError(err)
	}
	keys, err := provider.List(context.Background())
	if err != nil {
		return err
	}

	if listKeysJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"vault": vaultName,
			"keys":  keys,
		})
	}
	for _, k := range keys {
		fmt.Println(k)
	}
	return nil
}
