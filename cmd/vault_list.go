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

// vaultListCmd prints every configured vault in the repository.
// One line per vault: name, type, recipient count. The id and full
// recipient list belong in `vault info <name>`, where the operator
// has explicitly asked.
var vaultListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all vaults in the repository",
	RunE:  runVaultList,
}

func init() { vaultCmd.AddCommand(vaultListCmd) }

func runVaultList(_ *cobra.Command, _ []string) error {
	store, err := newStore()
	if err != nil {
		return err
	}
	configs, err := store.List()
	if err != nil {
		return err
	}
	out := os.Stdout
	if len(configs) == 0 {
		fmt.Fprintln(os.Stderr, cli.Mute(os.Stderr,
			"no vaults configured — run `kvlt vault create --name dev`"))
		return nil
	}
	// Header row in MUTED so the data rows pop. Names accent-colored
	// because they're the user-facing handles operators reference; type
	// stays plain NC; recipient count goes MUTED (secondary metadata).
	_, _ = fmt.Fprintf(out, "%s  %s  %s\n",
		cli.Mute(out, fmt.Sprintf("%-20s", "NAME")),
		cli.Mute(out, fmt.Sprintf("%-20s", "TYPE")),
		cli.Mute(out, "RECIPIENTS"))
	for _, c := range configs {
		recCount := 0
		if rs, ok := c.Settings["recipients"].([]any); ok {
			recCount = len(rs)
		}
		_, _ = fmt.Fprintf(out, "%s  %-20s  %s\n",
			cli.Accent(out, fmt.Sprintf("%-20s", c.Name)),
			c.Type,
			cli.Mute(out, fmt.Sprintf("%d", recCount)))
	}
	return nil
}
