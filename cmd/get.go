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
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/retr0h/kvlt/pkg/kvlt"
)

// getCmd retrieves a secret. Output is **raw bytes on stdout, no
// trailing newline, no decoration** — so it composes with Unix
// pipes:
//
//	export AWS_KEY="$(kvlt get dev AWS_ACCESS_KEY_ID)"
//	kvlt get dev DEPLOY_KEY | ssh-add -
//	curl -H "Authorization: Bearer $(kvlt get dev API_TOKEN)" …
//
// --json swaps in the structured shape ({"vault","key","value"})
// for scripts that want metadata. Exit codes follow shell
// convention: 0 success, 2 vault/key not found, 3 auth failure
// (so `kvlt get foo BAR 2>/dev/null || fallback` works).
var getCmd = &cobra.Command{
	Use:   "get <vault> <key>",
	Short: "Decrypt and print a secret value",
	Args:  cobra.ExactArgs(2),
	RunE:  runGet,
}

var getJSON bool

func init() {
	getCmd.Flags().BoolVar(&getJSON, "json", false,
		"emit {vault, key, value} JSON instead of the raw secret")
	rootCmd.AddCommand(getCmd)
}

// exit codes the get command uses; documented in the long help so
// shell scripts can branch on them. Other commands inherit the
// cobra default (1 on any RunE error).
const (
	exitNotFound   = 2
	exitAuthFailed = 3
)

func runGet(_ *cobra.Command, args []string) error {
	vaultName, key := args[0], args[1]

	store, err := newStore()
	if err != nil {
		return err
	}
	provider, err := store.Open(vaultName)
	if err != nil {
		return mapGetError(err)
	}
	value, err := provider.Get(context.Background(), key)
	if err != nil {
		return mapGetError(err)
	}

	if getJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{
			"vault": vaultName,
			"key":   key,
			"value": value,
		})
	}
	// Raw bytes, no newline. Pipe-friendly by design — adding a
	// newline would break common patterns like:
	//   X="$(kvlt get …)"
	// where bash strips one trailing newline already; further
	// trimming is the caller's job if they want it.
	if _, err := os.Stdout.WriteString(value); err != nil {
		return fmt.Errorf("write secret to stdout: %w", err)
	}
	return nil
}

// mapGetError converts library-typed errors into shell-style exit
// codes via cobra's silent-error mechanism. We print the diagnostic
// to stderr ourselves so the user sees something actionable, then
// hand cobra a sentinel that triggers the right os.Exit code in
// Execute.
//
// Cobra has no first-class "exit with code N" support, so we set
// SilenceErrors on the command and call os.Exit explicitly here.
func mapGetError(err error) error {
	switch {
	case errors.Is(err, kvlt.ErrVaultNotFound), errors.Is(err, kvlt.ErrKeyNotFound):
		fmt.Fprintf(os.Stderr, "kvlt: %v\n", err)
		os.Exit(exitNotFound)
	case errors.Is(err, kvlt.ErrAuthFailed):
		fmt.Fprintf(os.Stderr, "kvlt: %v\n", err)
		os.Exit(exitAuthFailed)
	}
	return err
}
