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
	"github.com/spf13/viper"

	"github.com/retr0h/kvlt/internal/cli"
	"github.com/retr0h/kvlt/pkg/kvlt"
)

// newStore builds the public-API Store every CLI verb operates
// against, wiring in the TTY-backed passphrase prompt as the
// IdentityResolver. Centralized here so every verb uses the same
// identity-discovery rules — and so the rules live behind the
// public IdentityResolver / DefaultIdentityResolver surface,
// proving external Go callers can use the same construction kvlt's
// own CLI does.
func newStore() (*kvlt.Store, error) {
	repo := viper.GetString("repo.path")
	resolver := kvlt.DefaultIdentityResolver(cli.PassphrasePromptTTY)
	return kvlt.NewStore(repo, resolver)
}
