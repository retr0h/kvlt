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

// Package cmd contains the kvlt cobra command tree.
package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/retr0h/kvlt/internal/cli"
)

var rootCmd = &cobra.Command{
	Use:   "kvlt",
	Short: "Pluggable secrets vault — age + SSH keys by default, cloud backends optional",
	Long: `kvlt is a small, dependency-light secrets vault. The default backend
encrypts with age using your existing SSH keys; pluggable backends
(SOPS, AWS Secrets Manager, Azure Key Vault, 1Password, HashiCorp
Vault) can be opted in via build tags without touching caller code —
vaults are referenced by name, not by backend type.`,
	RunE: func(c *cobra.Command, _ []string) error {
		// Bare `kvlt` prints the styled banner + a terse subcommand
		// list. We deliberately avoid c.Help()'s wall-of-text default —
		// the goal is a welcoming first impression that matches the
		// install script's aesthetic.
		out := c.OutOrStdout()
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprint(out, cli.Banner(out))
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, cli.Mute(out, "Pluggable secrets vault. Local-first. No daemon."))
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintln(out, cli.Mute(out, "Commands:"))
		for _, line := range []struct{ name, desc string }{
			{"vault create", "create a new vault"},
			{"vault list", "list configured vaults"},
			{"vault info", "show one vault's id, type, recipients"},
			{"secret put", "encrypt and store a secret"},
			{"secret get", "decrypt and print a secret"},
			{"secret list", "list secret keys (never values)"},
			{"env", "emit `export KEY=VALUE` for `eval`"},
			{"run", "exec a child with vault secrets in env"},
			{"themes", "preview the CLI color themes"},
			{"version", "print build identity"},
		} {
			_, _ = fmt.Fprintf(out, "  %-14s  %s\n",
				cli.Accent(out, line.name), cli.Mute(out, line.desc))
		}
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(
			out,
			"%s %s\n",
			cli.Mute(out, "Run"),
			cli.Accent(
				out,
				"kvlt <command> --help",
			)+cli.Mute(
				out,
				" for command-specific options.",
			),
		)
		_, _ = fmt.Fprintln(out)
		return nil
	},
}

// Execute runs the root command; invoked by main. SilenceUsage drops
// the help-text dump on runtime failures (config not found, bind:port
// in use, …) where it's just noise.
//
// Errors flow up from pkg/kvlt as wrapped sentinels, return through
// each verb's RunE verbatim, and land here. We render one themed
// line via cli.Failure and exit with the code exitCodeFor maps from
// the sentinel — the only place in the binary that translates a
// library error into shell-shaped output. The library never logs.
func Execute() {
	rootCmd.SilenceUsage = true
	rootCmd.SilenceErrors = true
	err := rootCmd.Execute()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, cli.Failure(os.Stderr, err.Error()))
	}
	if code := exitCodeFor(err); code != 0 {
		os.Exit(code)
	}
}

func init() {
	cobra.OnInitialize(initConfig)
}

// initConfig wires viper — env-var overrides take effect through a
// KVLT_… prefix with dots replaced by underscores, so e.g.
// KVLT_REPO_PATH=/var/lib/kvlt overrides the repo.path default.
//
// repo.path is the repository root — the directory that contains the
// `vaults/` and `.kvlt/secrets/` trees. Defaults to the current
// working directory so `kvlt vault create --name dev` from inside a
// project lays state alongside the project itself.
func initConfig() {
	viper.SetEnvPrefix("kvlt")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("repo.path", ".")
}
