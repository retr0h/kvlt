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

// Package cli holds CLI-only helpers — TTY prompts, output
// formatting, anything that only makes sense in the context of
// running kvlt on a terminal. Never imported by pkg/kvlt: the public
// library should not pull in `golang.org/x/term` for its callers.
package cli

import (
	"errors"
	"fmt"
	"os"

	"golang.org/x/term"
)

// PassphrasePromptTTY reads a passphrase from /dev/tty with echo
// suppressed. Going through /dev/tty (not stdin/stderr) is
// deliberate so prompts work inside command substitutions like
// $(kvlt get …) — stdout might be captured, stderr might be
// redirected, but /dev/tty always points at the user's terminal.
//
// Returns an error if the process has no controlling terminal (CI,
// detached daemon, …) so callers can surface a clear "no TTY
// available" message rather than silently hanging on a Read.
func PassphrasePromptTTY(keyPath string) ([]byte, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, fmt.Errorf("open /dev/tty for passphrase prompt: %w", err)
	}
	defer func() { _ = tty.Close() }()

	if !term.IsTerminal(int(tty.Fd())) {
		return nil, errors.New("/dev/tty is not a terminal")
	}

	if _, err := fmt.Fprintf(tty, "Enter passphrase for %s: ", keyPath); err != nil {
		return nil, fmt.Errorf("write passphrase prompt: %w", err)
	}
	pass, err := term.ReadPassword(int(tty.Fd()))
	// term.ReadPassword leaves the cursor on the same line — print a
	// newline so subsequent output (or the next prompt) starts fresh.
	_, _ = fmt.Fprintln(tty)
	if err != nil {
		return nil, fmt.Errorf("read passphrase: %w", err)
	}
	if len(pass) == 0 {
		return nil, errors.New("empty passphrase")
	}
	return pass, nil
}

// ConfirmDestructive shows a yes/no prompt on /dev/tty for the
// "delete this thing" path. Returns true only on an exact "y" or
// "yes" (case-insensitive). Anything else — empty enter, "n",
// gibberish — returns false. We deliberately do NOT default to yes
// on enter: destructive actions should require an affirmative key.
//
// Returns an error if no /dev/tty is available, so callers in CI
// can rely on `--force` rather than silently waiting on Read.
func ConfirmDestructive(question string) (bool, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return false, fmt.Errorf("open /dev/tty for confirm prompt: %w", err)
	}
	defer func() { _ = tty.Close() }()
	if !term.IsTerminal(int(tty.Fd())) {
		return false, errors.New("/dev/tty is not a terminal")
	}

	if _, err := fmt.Fprintf(tty, "%s [y/N]: ", question); err != nil {
		return false, fmt.Errorf("write confirm prompt: %w", err)
	}
	var resp string
	if _, err := fmt.Fscanln(tty, &resp); err != nil {
		// Fscanln returns "unexpected newline" for an empty enter —
		// treat that as a "no" rather than an error.
		return false, nil //nolint:nilerr // empty input is a valid "no"
	}
	switch resp {
	case "y", "Y", "yes", "Yes", "YES":
		return true, nil
	default:
		return false, nil
	}
}

// PromptSecretValue reads a secret value (`kvlt put <vault> <key>`
// with no `=` and no piped stdin) from /dev/tty with echo
// suppressed. Same /dev/tty rationale as PassphrasePromptTTY.
func PromptSecretValue(label string) (string, error) {
	tty, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return "", fmt.Errorf("open /dev/tty for secret prompt: %w", err)
	}
	defer func() { _ = tty.Close() }()

	if !term.IsTerminal(int(tty.Fd())) {
		return "", errors.New("/dev/tty is not a terminal")
	}

	if _, err := fmt.Fprintf(tty, "Enter value for %s: ", label); err != nil {
		return "", fmt.Errorf("write secret prompt: %w", err)
	}
	val, err := term.ReadPassword(int(tty.Fd()))
	_, _ = fmt.Fprintln(tty)
	if err != nil {
		return "", fmt.Errorf("read secret value: %w", err)
	}
	return string(val), nil
}
