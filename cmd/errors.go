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
	"errors"
	"fmt"
	"os"

	"github.com/retr0h/kvlt/pkg/kvlt"
)

// Exit codes used by verbs that decrypt (secret get, env, run).
// Documented here — and in each verb's --help text — so shell
// scripts can branch on them without parsing strings:
//
//	kvlt secret get foo BAR 2>/dev/null
//	case $? in
//	  0) ;; # success
//	  2) echo "missing" ;;
//	  3) echo "auth failed" ;;
//	esac
const (
	exitNotFound   = 2
	exitAuthFailed = 3
)

// mapGetError converts library-typed errors into shell-style exit
// codes via os.Exit. We print the diagnostic to stderr ourselves so
// the operator sees something actionable, then call os.Exit
// explicitly: cobra has no first-class "exit with code N" hook —
// its RunE-error path always exits 1.
//
// Errors that don't match a known sentinel pass through unchanged
// for cobra's default handling.
func mapGetError(err error) error {
	switch {
	case errors.Is(err, kvlt.ErrVaultNotFound), errors.Is(err, kvlt.ErrKeyNotFound):
		fmt.Fprintf(os.Stderr, "kvlt: %v\n", err)
		os.Exit(exitNotFound)
	case errors.Is(err, kvlt.ErrAuthFailed):
		fmt.Fprintf(os.Stderr, "kvlt: %v\n", err)
		os.Exit(exitAuthFailed)
	}
	return err
}
