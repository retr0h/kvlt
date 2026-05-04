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
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/retr0h/kvlt/internal/cli"
)

var (
	secretPutVault string
	secretPutKey   string
	secretPutValue string
	// secretPutValueSet tracks whether --value was passed at all,
	// even if the empty string. Plain string flags can't distinguish
	// "not provided" from "provided as empty"; we check the cobra
	// flag's Changed bit instead. Storing the result here keeps
	// readPutInput simple.
	secretPutValueSet bool
)

// secretPutCmd encrypts and stores a secret. Three input modes,
// resolved in priority order:
//
//  1. --value provided   → fastest; lands in shell history
//  2. piped stdin        → recommended for scripts/CI
//  3. interactive prompt → echo-off /dev/tty input
var secretPutCmd = &cobra.Command{
	Use:   "put",
	Short: "Encrypt and store a secret in a vault",
	Long: `Three input modes, resolved in priority order:

  kvlt secret put --vault dev --key API_KEY --value sk-1234   # inline
  echo "$VAL" | kvlt secret put --vault dev --key API_KEY     # stdin
  kvlt secret put --vault dev --key API_KEY                   # interactive`,
	Args: cobra.NoArgs,
	RunE: runSecretPut,
}

func init() {
	secretPutCmd.Flags().StringVarP(&secretPutVault, "vault", "v", "",
		"target vault name (required)")
	secretPutCmd.Flags().StringVarP(&secretPutKey, "key", "k", "",
		"secret key to store (required)")
	secretPutCmd.Flags().StringVar(&secretPutValue, "value", "",
		"inline secret value (lands in shell history; prefer stdin or interactive)")
	_ = secretPutCmd.MarkFlagRequired("vault")
	_ = secretPutCmd.MarkFlagRequired("key")
	secretCmd.AddCommand(secretPutCmd)
}

func runSecretPut(cmd *cobra.Command, _ []string) error {
	secretPutValueSet = cmd.Flags().Changed("value")

	value, mode, err := readPutInput()
	if err != nil {
		return err
	}

	store, err := newStore()
	if err != nil {
		return err
	}
	provider, err := store.Open(secretPutVault)
	if err != nil {
		return mapGetError(err)
	}
	if err := provider.Put(context.Background(), secretPutKey, value); err != nil {
		return err
	}

	out := os.Stdout
	_, _ = fmt.Fprintln(out, cli.Success(out, fmt.Sprintf("stored %s in vault %s %s",
		cli.Accent(out, secretPutKey),
		cli.Accent(out, secretPutVault),
		cli.Mute(out, "(via "+mode+")"))))
	return nil
}

// readPutInput resolves which input mode the user invoked and
// returns (value, mode-label, error). The mode label is surfaced in
// logs so an operator can distinguish "I piped it" from "I typed it
// interactively" in audit output.
func readPutInput() (value, mode string, err error) {
	// Mode 1: --value passed. Even an explicit empty string is
	// honored — operators sometimes need to store empty values to
	// disambiguate "not set" from "intentionally blank".
	if secretPutValueSet {
		return secretPutValue, "inline", nil
	}

	// Mode 2: piped stdin. Detect by IsTerminal — if stdin is a pipe
	// or redirected file, slurp it. We strip a single trailing
	// newline (the convention for `echo "x" | …`) but otherwise
	// pass bytes through verbatim.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		raw, rerr := io.ReadAll(os.Stdin)
		if rerr != nil {
			return "", "", fmt.Errorf("read stdin: %w", rerr)
		}
		v := strings.TrimRight(string(raw), "\n")
		return v, "stdin", nil
	}

	// Mode 3: interactive. Prompt on /dev/tty with echo off.
	v, perr := cli.PromptSecretValue(secretPutKey)
	if perr != nil {
		return "", "", fmt.Errorf("prompt for secret value: %w", perr)
	}
	if v == "" {
		return "", "", fmt.Errorf("empty secret value (refusing to store)")
	}
	return v, "tty", nil
}
