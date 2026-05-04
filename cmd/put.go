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

// putCmd encrypts a secret and stores it in the named vault. Three
// input modes, in priority order, mirror the standard secret-CLI
// idiom (1Password CLI, AWS CLI, vault, etc.):
//
//  1. KEY=VALUE in the args         → fastest, but lands in shell history
//  2. piped stdin                    → recommended for scripts/CI
//  3. interactive prompt (no TTY)    → recommended for humans, echo off
var putCmd = &cobra.Command{
	Use:   "put <vault> <key>[=<value>]",
	Short: "Encrypt and store a secret in a vault",
	Long: `Three input modes:

  kvlt put dev API_KEY=sk-1234           # inline (lands in shell history)
  echo "$VAL" | kvlt put dev API_KEY     # stdin (recommended for scripts)
  kvlt put dev API_KEY                   # interactive prompt, echo off`,
	Args: cobra.ExactArgs(2),
	RunE: runPut,
}

func init() { rootCmd.AddCommand(putCmd) }

func runPut(_ *cobra.Command, args []string) error {
	vaultName, keyArg := args[0], args[1]

	key, value, mode, err := readPutInput(keyArg)
	if err != nil {
		return err
	}

	store, err := newStore()
	if err != nil {
		return err
	}
	provider, err := store.Open(vaultName)
	if err != nil {
		return err
	}
	if err := provider.Put(context.Background(), key, value); err != nil {
		return err
	}

	logger.Info("secret stored", "vault", vaultName, "key", key, "input", mode)
	return nil
}

// readPutInput resolves which input mode the user invoked and
// returns (key, value, mode-label, error). The mode label is
// surfaced in logs so an operator can distinguish "I piped it" from
// "I typed it interactively" in audit output.
func readPutInput(keyArg string) (key, value, mode string, err error) {
	// Mode 1: KEY=VALUE — the equals sign disambiguates without a flag.
	if k, v, ok := strings.Cut(keyArg, "="); ok {
		return k, v, "inline", nil
	}

	// Mode 2: piped stdin. Detect by IsTerminal — if stdin is a pipe
	// or a redirected file, slurp it. We strip a single trailing
	// newline (the convention for `echo "x" | …`) but otherwise pass
	// bytes through verbatim.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		raw, rerr := io.ReadAll(os.Stdin)
		if rerr != nil {
			return "", "", "", fmt.Errorf("read stdin: %w", rerr)
		}
		v := strings.TrimRight(string(raw), "\n")
		return keyArg, v, "stdin", nil
	}

	// Mode 3: interactive. Prompt on /dev/tty with echo off.
	v, perr := cli.PromptSecretValue(keyArg)
	if perr != nil {
		return "", "", "", fmt.Errorf("prompt for secret value: %w", perr)
	}
	if v == "" {
		return "", "", "", fmt.Errorf("empty secret value (refusing to store)")
	}
	return keyArg, v, "tty", nil
}
