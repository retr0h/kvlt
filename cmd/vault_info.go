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

	"github.com/spf13/cobra"

	"github.com/retr0h/kvlt/pkg/kvlt"
)

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
	for _, c := range configs {
		if c.Name != vaultInfoName {
			continue
		}
		fmt.Printf("name:        %s\n", c.Name)
		fmt.Printf("type:        %s\n", c.Type)
		fmt.Printf("id:          %s\n", c.ID)
		fmt.Println("recipients:")
		if rs, ok := c.Settings["recipients"].([]any); ok {
			for _, r := range rs {
				fmt.Printf("  - %v\n", r)
			}
		}
		return nil
	}
	return fmt.Errorf("%w: %q", kvlt.ErrVaultNotFound, vaultInfoName)
}
