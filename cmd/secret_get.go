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
	"golang.org/x/term"
)

var (
	secretGetVault      string
	secretGetKey        string
	secretGetJSON       bool
	secretGetPrivateKey string
)

// secretGetCmd retrieves a secret. Output is **raw bytes on stdout,
// no trailing newline, no decoration** so it composes with Unix
// pipes:
//
//	export AWS_KEY="$(kvlt secret get --vault dev --key AWS_KEY)"
//	kvlt secret get --vault dev --key DEPLOY_KEY | ssh-add -
//
// --json swaps in {vault, key, value} for scripts that want
// metadata.
var secretGetCmd = &cobra.Command{
	Use:   "get",
	Short: "Decrypt and print a secret value",
	Args:  cobra.NoArgs,
	RunE:  runSecretGet,
}

func init() {
	secretGetCmd.Flags().StringVarP(&secretGetVault, "vault", "v", "",
		"vault to read from (required)")
	secretGetCmd.Flags().StringVarP(&secretGetKey, "key", "k", "",
		"secret key to retrieve (required)")
	secretGetCmd.Flags().BoolVar(&secretGetJSON, "json", false,
		"emit {vault, key, value} JSON instead of the raw secret")
	secretGetCmd.Flags().StringVarP(&secretGetPrivateKey, "private-key", "i", "",
		"SSH private key to decrypt with (overrides ~/.ssh/id_* auto-discovery; env KVLT_PRIVATE_KEY)")
	_ = secretGetCmd.MarkFlagRequired("vault")
	_ = secretGetCmd.MarkFlagRequired("key")
	secretCmd.AddCommand(secretGetCmd)
}

func runSecretGet(_ *cobra.Command, _ []string) error {
	store, err := newStoreForRead(secretGetPrivateKey)
	if err != nil {
		return err
	}
	provider, err := store.Open(secretGetVault)
	if err != nil {
		return mapGetError(err)
	}
	value, err := provider.Get(context.Background(), secretGetKey)
	if err != nil {
		return mapGetError(err)
	}

	if secretGetJSON {
		return json.NewEncoder(os.Stdout).Encode(map[string]string{
			"vault": secretGetVault,
			"key":   secretGetKey,
			"value": value,
		})
	}
	// Bytes verbatim, with a single trailing newline added only when
	// stdout is a TTY. The interactive case wants the newline so the
	// shell prompt doesn't sit next to the value (`safe%`); pipes
	// and redirects want bytes-as-stored so file round-trip stays
	// byte-perfect (`kvlt secret get --key kubeconfig > ~/.kube/config`
	// reproduces the original file exactly).
	if _, err := os.Stdout.WriteString(value); err != nil {
		return fmt.Errorf("write secret to stdout: %w", err)
	}
	if term.IsTerminal(int(os.Stdout.Fd())) {
		_, _ = os.Stdout.WriteString("\n")
	}
	return nil
}
