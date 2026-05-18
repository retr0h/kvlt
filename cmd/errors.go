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
	"errors"

	"github.com/retr0h/kvlt/pkg/kvlt"
)

// Exit codes used by verbs that decrypt (secret get, env, run).
// Documented here — and in each verb's --help text — so shell
// scripts can branch on them without parsing strings:
//
//	kvlt secret get --vault foo --key BAR 2>/dev/null
//	case $? in
//	  0) ;; # success
//	  2) echo "missing" ;;
//	  3) echo "auth failed" ;;
//	esac
const (
	exitGeneric    = 1
	exitNotFound   = 2
	exitAuthFailed = 3
)

// mapGetError annotates a library error so the top-level Execute can
// translate it to the right shell exit code. Verbs return the result
// directly from RunE — no os.Exit here, which would bypass any
// cleanup deferred by the verb and make every command unit-testable
// only via subprocess. Errors that don't match a known sentinel are
// returned unchanged so cobra falls back to its generic error path.
func mapGetError(err error) error { return err }

// exitCodeFor returns the shell exit code Execute should use for err.
// Centralizes the sentinel-to-code mapping so verbs only have to
// return their library errors verbatim.
func exitCodeFor(err error) int {
	switch {
	case err == nil:
		return 0
	case errors.Is(err, kvlt.ErrVaultNotFound), errors.Is(err, kvlt.ErrKeyNotFound):
		return exitNotFound
	case errors.Is(err, kvlt.ErrAuthFailed):
		return exitAuthFailed
	default:
		return exitGeneric
	}
}
