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
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// envCmd prints every secret in a vault as `export KEY=VALUE`
// shell statements. The natural .envrc integration:
//
//	# .envrc — committed to git, ZERO plaintext secrets
//	eval "$(kvlt env dev)"
//
// Values are POSIX-shell-quoted (single-quotes with embedded
// single-quotes escaped) so secrets containing whitespace, $, `, or
// ! survive `eval` intact. We do NOT use double-quotes — they'd
// interpret $-substitutions inside the secret, which would be a
// silent corruption surface for any value containing dollar signs.
var envCmd = &cobra.Command{
	Use:   "env",
	Short: "Print all secrets as `export KEY=VALUE` lines for shell `eval`",
	Long: `Decrypt every secret in --vault and print export statements
suitable for ` + "`eval`" + `. Quoting is POSIX single-quote so values
containing $, backticks, spaces, or special chars survive intact.

Use cases:
  # .envrc — committed to git, no plaintext secrets
  eval "$(kvlt env --vault dev)"

  # Restrict to specific keys
  eval "$(kvlt env --vault dev --only API_KEY,DB_PASSWORD)"

  # Add a prefix to every exported var
  eval "$(kvlt env --vault dev --prefix MY_APP_)"`,
	Args: cobra.NoArgs,
	RunE: runEnv,
}

var (
	envVault      string
	envOnlyKeys   []string
	envPrefix     string
	envPrivateKey string
)

func init() {
	envCmd.Flags().StringVarP(&envVault, "vault", "v", "",
		"vault to source secrets from (required)")
	envCmd.Flags().StringSliceVar(&envOnlyKeys, "only", nil,
		"comma-separated list of keys to export (default: all)")
	envCmd.Flags().StringVar(&envPrefix, "prefix", "",
		"prefix prepended to every exported variable name")
	envCmd.Flags().StringVarP(&envPrivateKey, "private-key", "i", "",
		"SSH private key to decrypt with (overrides ~/.ssh/id_* auto-discovery; env KVLT_PRIVATE_KEY)")
	_ = envCmd.MarkFlagRequired("vault")
	rootCmd.AddCommand(envCmd)
}

func runEnv(_ *cobra.Command, _ []string) error {
	ctx := context.Background()

	store, err := newStoreForRead(envPrivateKey)
	if err != nil {
		return err
	}
	provider, err := store.Open(envVault)
	if err != nil {
		return mapGetError(err)
	}

	keys, err := provider.List(ctx)
	if err != nil {
		return err
	}
	if len(envOnlyKeys) > 0 {
		keys = filterKeys(keys, envOnlyKeys)
	}

	for _, k := range keys {
		val, err := provider.Get(ctx, k)
		if err != nil {
			return mapGetError(err)
		}
		fmt.Printf("export %s%s=%s\n", envPrefix, k, shellQuote(val))
	}
	return nil
}

// filterKeys returns the keys present in `available` that match an
// entry in `wanted`. Order follows `available` so the output is
// stable across runs (callers writing `eval "$(kvlt env …)"` into
// .envrc benefit from a deterministic line ordering for diff-friendly
// version control).
func filterKeys(available, wanted []string) []string {
	want := make(map[string]struct{}, len(wanted))
	for _, w := range wanted {
		want[w] = struct{}{}
	}
	out := make([]string, 0, len(wanted))
	for _, k := range available {
		if _, ok := want[k]; ok {
			out = append(out, k)
		}
	}
	return out
}

// shellQuote wraps s in POSIX single-quotes, doubling any embedded
// single-quote so the eval'd form reproduces the original byte
// sequence exactly. The escape pattern `'\”` is the standard idiom:
// close the single-quoted span, emit a literal single-quote with
// backslash, reopen the single-quoted span.
//
// Single-quotes (not double) are required: inside double-quotes,
// the shell still interprets $, backticks, and \. A secret value
// containing $foo would be silently substituted with whatever $foo
// is in the calling shell's environment.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
