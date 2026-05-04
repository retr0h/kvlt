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
	"github.com/retr0h/kvlt/pkg/kvlt"
)

// vaultInfoName is the --name flag value (vault to inspect).
var vaultInfoName string

// vaultInfoCmd prints one vault's full config — id, type, every
// recipient's canonical string. Used by humans deciding whether a
// vault is the right one to put a secret in, and by audit scripts
// confirming who can decrypt before a sensitive add.
var vaultInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show one vault's id, type, and recipient list",
	Args:  cobra.NoArgs,
	RunE:  runVaultInfo,
}

func init() {
	vaultInfoCmd.Flags().StringVarP(&vaultInfoName, "name", "n", "",
		"vault name to inspect (required)")
	_ = vaultInfoCmd.MarkFlagRequired("name")
	vaultCmd.AddCommand(vaultInfoCmd)
}

func runVaultInfo(_ *cobra.Command, _ []string) error {
	store, err := newStore()
	if err != nil {
		return err
	}
	configs, err := store.List()
	if err != nil {
		return err
	}
	var match *kvlt.Config
	for _, c := range configs {
		if c.Name == vaultInfoName {
			match = c
			break
		}
	}
	if match == nil {
		return fmt.Errorf("%w: %q", kvlt.ErrVaultNotFound, vaultInfoName)
	}

	out := os.Stdout
	const width = 13 // "recipients:" + space — keeps every label aligned

	cli.Field(out, width, "name", cli.Accent(out, match.Name))
	cli.Field(out, width, "type", match.Type)
	cli.Field(out, width, "id", cli.Mute(out, match.ID))

	provider, err := store.Open(match.Name)
	if err != nil {
		return err
	}
	if d, ok := provider.(kvlt.Describer); ok {
		for _, f := range d.Describe() {
			switch len(f.Values) {
			case 0:
				continue
			case 1:
				cli.Field(out, width, f.Label, f.Values[0])
			default:
				_, _ = fmt.Fprintf(out, "%s\n", cli.Mute(out, f.Label+":"))
				for _, v := range f.Values {
					_, _ = fmt.Fprintf(out, "  %s %s\n", cli.Mute(out, "-"), v)
				}
			}
		}
	}
	return nil
}
