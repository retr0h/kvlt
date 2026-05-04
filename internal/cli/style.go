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

package cli

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"golang.org/x/term"
)

// Theme is a six-role palette covering every place kvlt emits styled
// text. Roles are kept stable across themes so a theme swap is a
// pure recolor — no callers change.
//
//	Mute    labels, secondary metadata ("name:", "version:")
//	Accent  primary highlight — paths, IDs, vault names, headlines
//	OK      success events (vault created, secret stored)
//	Err     errors at the CLI boundary
//	Info    cool-toned hints and pointers
//	Banner* the two banner lines — Top/Bot let themes decide whether
//	        the brand color sits above or below the implicit midline
//
// Each field is a raw SGR escape string, applied as `code + text + reset`
// at render time. Empty string disables coloring for that role,
// which is how the no-color and NO_COLOR fallbacks short-circuit.
type Theme struct {
	Name      string
	Mute      string
	Accent    string
	OK        string
	Err       string
	Info      string
	BannerTop string
	BannerBot string
}

// Built-in themes. Pick one with `KVLT_THEME=<name>` (default
// "opencode"). Adding a new theme is a single Theme literal here +
// register in themes.
var (
	// ThemeOpencode mirrors opencode's installer palette — friendly
	// orange + soft green. The default for visual continuity between
	// `curl … | bash` and the installed binary.
	ThemeOpencode = Theme{
		Name:      "opencode",
		Mute:      "\033[0;2m",
		Accent:    "\033[38;5;214m", // orange
		OK:        "\033[38;5;114m", // soft green
		Err:       "\033[0;31m",
		Info:      "\033[38;5;110m", // cyan
		BannerTop: "\033[38;5;240m", // grey
		BannerBot: "\033[38;5;214m", // orange
	}

	// ThemeMetal leans into the kvlt/cult metal aesthetic — bone
	// muted, blood-red accent, cold-steel info. Errors stay brighter
	// red so they're distinct from the dull-red accent.
	ThemeMetal = Theme{
		Name:      "metal",
		Mute:      "\033[38;5;253;2m", // bone, faint
		Accent:    "\033[38;5;124m",   // blood red
		OK:        "\033[38;5;108m",   // ash green
		Err:       "\033[38;5;203m",   // brighter red — distinct from accent
		Info:      "\033[38;5;67m",    // cold steel blue-grey
		BannerTop: "\033[38;5;253;2m", // bone faint
		BannerBot: "\033[38;5;124m",   // blood red
	}

	// ThemeCrimson is metal with the saturation cranked — brighter
	// red across the board, slightly less austere than metal but
	// still firmly in the "danger / dark" register.
	ThemeCrimson = Theme{
		Name:      "crimson",
		Mute:      "\033[38;5;245m",
		Accent:    "\033[38;5;160m", // bright crimson
		OK:        "\033[38;5;107m",
		Err:       "\033[38;5;196m",
		Info:      "\033[38;5;103m",
		BannerTop: "\033[38;5;245m",
		BannerBot: "\033[38;5;160m",
	}

	// ThemeNoir is monochrome — only greys and a single white accent.
	// For operators who hate color but still want hierarchy. Errors
	// remain red because no-color in errors is unsafe ergonomics.
	ThemeNoir = Theme{
		Name:      "noir",
		Mute:      "\033[38;5;240m",
		Accent:    "\033[38;5;255m", // bright white
		OK:        "\033[38;5;250m",
		Err:       "\033[38;5;203m",
		Info:      "\033[38;5;245m",
		BannerTop: "\033[38;5;240m",
		BannerBot: "\033[38;5;255m",
	}

	// ThemeForge is the middle-ground compromise — orange ember kept
	// from opencode for the accent, but a steelier grey palette and
	// muted greens. Reads industrial rather than friendly.
	ThemeForge = Theme{
		Name:      "forge",
		Mute:      "\033[38;5;244m",
		Accent:    "\033[38;5;208m", // ember orange
		OK:        "\033[38;5;107m",
		Err:       "\033[38;5;167m",
		Info:      "\033[38;5;67m",
		BannerTop: "\033[38;5;244m",
		BannerBot: "\033[38;5;208m",
	}

	// ThemeAsh — the desaturated take on the metal aesthetic. Brick-
	// dust accent (131) instead of blood red, soft greys throughout.
	// Atmospheric without the harshness; reads like a faded patch on
	// a denim jacket.
	ThemeAsh = Theme{
		Name:      "ash",
		Mute:      "\033[38;5;245m",
		Accent:    "\033[38;5;131m", // dusty brick
		OK:        "\033[38;5;108m", // ash green
		Err:       "\033[38;5;167m",
		Info:      "\033[38;5;103m", // muted blue-grey
		BannerTop: "\033[38;5;245m",
		BannerBot: "\033[38;5;131m",
	}

	// ThemeOxblood pushes red as far toward black as it can go while
	// still reading as red — a very dark wine accent (88) on dim
	// muted greys. Brooding; the accent is barely a color at all,
	// which is what makes it feel kvlt rather than warning-light.
	// Errors stay brighter so they remain distinguishable from the
	// accent.
	ThemeOxblood = Theme{
		Name:      "oxblood",
		Mute:      "\033[38;5;240m",
		Accent:    "\033[38;5;88m", // very dark wine
		OK:        "\033[38;5;100m",
		Err:       "\033[38;5;124m", // brighter than accent
		Info:      "\033[38;5;60m",  // dusty navy
		BannerTop: "\033[38;5;240m",
		BannerBot: "\033[38;5;88m",
	}

	// ThemeRust shifts the accent toward orange-red — weathered
	// industrial rather than gothic metal. 130 reads as oxidized
	// iron on a sun-bleached fence; pairs naturally with steel-grey
	// muted text.
	ThemeRust = Theme{
		Name:      "rust",
		Mute:      "\033[38;5;243m",
		Accent:    "\033[38;5;130m", // rust orange-red
		OK:        "\033[38;5;107m",
		Err:       "\033[38;5;167m",
		Info:      "\033[38;5;66m",
		BannerTop: "\033[38;5;243m",
		BannerBot: "\033[38;5;130m",
	}
)

// themes is the registry consulted by ResolveTheme. The first entry
// is the default when KVLT_THEME is unset.
var themes = []*Theme{
	&ThemeAsh,
	&ThemeOpencode,
	&ThemeMetal,
	&ThemeCrimson,
	&ThemeNoir,
	&ThemeForge,
	&ThemeOxblood,
	&ThemeRust,
}

// active is the currently-selected theme. Resolved from KVLT_THEME at
// package init; a CLI flag could override it later by calling
// SetTheme. Defaults to ash — dusty brick + bone, the kvlt house
// palette.
var active = &ThemeAsh

func init() {
	if t, ok := lookupTheme(os.Getenv("KVLT_THEME")); ok {
		active = t
	}
}

// SetTheme replaces the active theme. Returns false if name is
// unknown — callers can fall back to the default and warn.
func SetTheme(name string) bool {
	t, ok := lookupTheme(name)
	if !ok {
		return false
	}
	active = t
	return true
}

// ActiveTheme returns the currently-active theme. Useful for
// `kvlt themes` listing or for tests that want to assert behavior
// under a specific palette.
func ActiveTheme() *Theme { return active }

// ThemeNames returns every registered theme name, sorted. The first
// element is the default (opencode); subsequent are alphabetical so
// listings are deterministic.
func ThemeNames() []string {
	out := make([]string, 0, len(themes))
	for _, t := range themes {
		out = append(out, t.Name)
	}
	// Default first, rest sorted.
	first := out[0]
	rest := append([]string(nil), out[1:]...)
	sort.Strings(rest)
	return append([]string{first}, rest...)
}

func lookupTheme(name string) (*Theme, bool) {
	if name == "" {
		return nil, false
	}
	want := strings.ToLower(strings.TrimSpace(name))
	for _, t := range themes {
		if strings.EqualFold(t.Name, want) {
			return t, true
		}
	}
	return nil, false
}

const reset = "\033[0m"

// useColor reports whether ANSI escapes should be emitted on the
// given writer. Honors the de-facto standard NO_COLOR env var (see
// https://no-color.org/) and only colorizes when the underlying fd
// is a TTY — piped or redirected output stays plain.
func useColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	type fder interface{ Fd() uintptr }
	f, ok := w.(fder)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// wrap returns s sandwiched between code and reset when colorEnabled,
// otherwise s alone. Centralized so every helper goes through the
// same on/off switch — adding a NO_COLOR check elsewhere is a smell.
func wrap(colorEnabled bool, code, s string) string {
	if !colorEnabled || code == "" {
		return s
	}
	return code + s + reset
}

// Mute returns s rendered as secondary text per the active theme.
func Mute(w io.Writer, s string) string { return wrap(useColor(w), active.Mute, s) }

// Accent returns s rendered as the brand accent color (orange in
// opencode, blood red in metal, white in noir, …).
func Accent(w io.Writer, s string) string { return wrap(useColor(w), active.Accent, s) }

// OK returns s in the success color.
func OK(w io.Writer, s string) string { return wrap(useColor(w), active.OK, s) }

// Err returns s in the error color.
func Err(w io.Writer, s string) string { return wrap(useColor(w), active.Err, s) }

// Info returns s in the cool-toned info/hint color.
func Info(w io.Writer, s string) string { return wrap(useColor(w), active.Info, s) }

// Field renders a `label: value` line styled MUTED-then-NC, padded
// so successive labels of varying length still align. Width is the
// column where the value starts (label + ":" + spaces fills to it).
//
// Used by `vault info` and `vault list` so the on-disk vault layout
// reads consistently with the install summary.
func Field(w io.Writer, width int, label, value string) {
	colon := label + ":"
	pad := max(width-len(colon), 1)
	_, _ = fmt.Fprintf(w, "%s%s%s\n", Mute(w, colon), strings.Repeat(" ", pad), value)
}

// Banner returns the kvlt block-letter logo, themed via the active
// theme's BannerTop/BannerBot colors. Line-level coloring (rather
// than the half-block midline trick) — 2-row letterforms don't give
// us enough vertical room for a true middle stripe.
//
// On non-color outputs the banner falls back to plain block letters.
func Banner(w io.Writer) string {
	if !useColor(w) {
		return "█▄▀ █░█ █░░ ▀█▀\n█░█ ▀▄▀ █▄▄ ░█░\n"
	}
	top := active.BannerTop
	bot := active.BannerBot
	return top + "█▄▀ █░█ █░░ ▀█▀" + reset + "\n" +
		bot + "█░█ ▀▄▀ █▄▄ ░█░" + reset + "\n"
}

// Success renders a leading "✓" mark in the OK color followed by msg.
// Falls back to "[ok]" on NO_COLOR / non-TTY for log-friendliness.
func Success(w io.Writer, msg string) string {
	if useColor(w) {
		return OK(w, "✓") + " " + msg
	}
	return "[ok] " + msg
}

// Failure mirrors Success for error one-liners.
func Failure(w io.Writer, msg string) string {
	if useColor(w) {
		return Err(w, "✗") + " " + msg
	}
	return "[err] " + msg
}
