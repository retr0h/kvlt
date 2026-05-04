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
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/lmittmann/tint"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/term"
)

// logger is the package-level slog logger, populated from initLogger
// after cobra parses persistent flags. Subcommands log through it
// directly; a child logger (with subsystem tag) is handed to the HTTP
// server when running as a daemon so daemon and CLI lines stay
// distinguishable in a unified stream.
var (
	logger     = slog.New(slog.NewTextHandler(os.Stderr, nil))
	jsonOutput bool
)

var rootCmd = &cobra.Command{
	Use:   "kvlt",
	Short: "Pluggable secrets vault — local AES-GCM by default, cloud backends optional",
	Long: `
█▄▀ █░█ █░░ ▀█▀
█░█ ▀▄▀ █▄▄ ░█░

kvlt is a small, dependency-light secrets vault. The default backend
stores secrets locally with AES-GCM. Pluggable backends (AWS Secrets
Manager, Azure Key Vault, 1Password, HashiCorp Vault) can be opted in
without touching caller code — vaults are referenced by name, not by
backend type.

  kvlt vault create local-encryption dev
  kvlt put dev API_KEY=hunter2
  kvlt get dev API_KEY
  kvlt list-keys dev`,
	RunE: func(c *cobra.Command, _ []string) error {
		return c.Help()
	},
}

// Execute runs the root command; invoked by main. SilenceUsage drops
// the help-text dump on runtime failures (config not found, bind:port
// in use, …) where it's just noise. Cobra already prints "Error: <err>"
// on its own.
func Execute() {
	rootCmd.SilenceUsage = true
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	cobra.OnInitialize(initConfig, initLogger)

	rootCmd.PersistentFlags().BoolP("debug", "d", false, "enable debug logging")
	rootCmd.PersistentFlags().BoolVarP(&jsonOutput, "json", "j", false, "emit logs as JSON")

	_ = viper.BindPFlag("debug", rootCmd.PersistentFlags().Lookup("debug"))
}

// initConfig wires viper — env-var overrides take effect through a
// KVLT_… prefix with dots replaced by underscores, so e.g.
// KVLT_REPO_PATH=/var/lib/kvlt overrides the repo.path default.
// Defaults seeded here are the source of truth; flags binding to the
// same key win over both env and default at runtime.
func initConfig() {
	viper.SetEnvPrefix("kvlt")
	viper.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	viper.AutomaticEnv()

	viper.SetDefault("repo.path", ".kvlt")
}

// initLogger swaps the package-level logger to a tint handler with
// color when stderr is a TTY, plain text otherwise. --json swaps in
// the slog JSON handler — for log aggregators that prefer structured
// input. Level follows --debug.
func initLogger() {
	level := slog.LevelInfo
	if viper.GetBool("debug") {
		level = slog.LevelDebug
	}

	var handler slog.Handler
	if jsonOutput {
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: level})
	} else {
		handler = tint.NewHandler(os.Stderr, &tint.Options{
			Level:      level,
			TimeFormat: time.Kitchen,
			NoColor:    !term.IsTerminal(int(os.Stderr.Fd())),
		})
	}

	logger = slog.New(handler)
	slog.SetDefault(logger)
}
