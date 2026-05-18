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

// rootCmd prints the themed banner before falling through to cobra's
// auto-generated help. The banner is the welcoming first impression
// that matches the install script's aesthetic; everything below it —
// description, command list, flags — is whatever cobra walks from
// the tree. Hand-curating that drifted as soon as a new verb landed.
var rootCmd = &cobra.Command{
	Use:   "kvlt",
	Short: "Pluggable secrets vault. Local-first. No daemon.",
	Run: func(c *cobra.Command, _ []string) {
		_ = c.Help()
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

	// Wrap cobra's default help to print the themed banner above it.
	// SetHelpFunc fires for `kvlt --help` and for the bare-command
	// fallback alike, so the banner shows in both paths without
	// duplicating itself.
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(func(c *cobra.Command, args []string) {
		if c == rootCmd {
			out := c.OutOrStdout()
			_, _ = fmt.Fprintln(out)
			_, _ = fmt.Fprint(out, cli.Banner(out))
			_, _ = fmt.Fprintln(out)
		}
		defaultHelp(c, args)
	})
}

// initConfig wires viper — env-var overrides take effect through a
// KVLT_… prefix with dots replaced by underscores, so e.g.
// KVLT_REPO_PATH=/var/lib/kvlt overrides the repo.path default.
//
// repo.path is the repository root — the directory that contains the
// `.kvlt/` tree (vault configs under .kvlt/vaults/, encrypted
// payloads under .kvlt/secrets/). Defaults to the current working
// directory so `kvlt vault create --name dev` from inside a project
// lays state alongside the project itself.
func initConfig() {
	viper.SetEnvPrefix("kvlt")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("repo.path", ".")
}
