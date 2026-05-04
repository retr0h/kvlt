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
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/retr0h/kvlt/internal/cli"
)

// themesCmd previews every registered theme so the operator can pick
// one without having to commit to KVLT_THEME first. Renders the
// banner + a representative line set per theme:
//
//	kvlt themes
//
// Set the chosen theme persistently with `export KVLT_THEME=metal`.
var themesCmd = &cobra.Command{
	Use:   "themes",
	Short: "Preview every available CLI color theme",
	Long: `Render the kvlt banner plus a representative styled line set
under each registered theme. Pick one and set KVLT_THEME=<name> in
your shell rc to make it the default for every kvlt invocation.`,
	Args: cobra.NoArgs,
	RunE: runThemes,
}

func init() { rootCmd.AddCommand(themesCmd) }

func runThemes(_ *cobra.Command, _ []string) error {
	out := os.Stdout
	current := cli.ActiveTheme().Name
	defer func() { _ = cli.SetTheme(current) }() // restore on exit

	for _, name := range cli.ThemeNames() {
		_ = cli.SetTheme(name)
		_, _ = fmt.Fprintln(out)
		_, _ = fmt.Fprintf(out, "%s %s\n",
			cli.Mute(out, "theme:"),
			cli.Accent(out, name))
		_, _ = fmt.Fprint(out, cli.Banner(out))
		_, _ = fmt.Fprintln(out, cli.Success(
			out,
			"vault "+cli.Accent(
				out,
				"dev",
			)+" created "+cli.Mute(
				out,
				"(local_encryption, 1 recipient)",
			),
		))
		_, _ = fmt.Fprintln(out, cli.Failure(out,
			cli.Err(out, "vault not found")+": "+cli.Mute(out, "\"absent\"")))
		_, _ = fmt.Fprintln(out, cli.Info(out, "→ ")+
			cli.Mute(out, "set ")+cli.Accent(out, "KVLT_THEME="+name)+
			cli.Mute(out, " to make this the default"))
	}
	_, _ = fmt.Fprintln(out)
	return nil
}
