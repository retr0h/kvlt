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

	"github.com/charmbracelet/lipgloss"
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
// Each role is a lipgloss.Style. lipgloss handles NO_COLOR and TTY
// detection (via termenv) so we don't reinvent it here. Empty
// styles fall through to plain text.
type Theme struct {
	Name      string
	Mute      lipgloss.Style
	Accent    lipgloss.Style
	OK        lipgloss.Style
	Err       lipgloss.Style
	Info      lipgloss.Style
	BannerTop lipgloss.Style
	BannerBot lipgloss.Style
}

// fg is shorthand for `lipgloss.NewStyle().Foreground(...)` so theme
// definitions stay scannable. Color values are xterm-256 ANSI
// indexes (matches install.sh exactly), or hex strings if a theme
// wants 24-bit truecolor.
func fg(c string) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(c))
}

// faint is for the Mute role — uses the dim attribute rather than a
// specific color so it adapts to the user's terminal background.
var faint = lipgloss.NewStyle().Faint(true)

// Built-in themes. Pick one with `KVLT_THEME=<name>` (default
// "ash"). Adding a new theme is a single Theme literal here +
// register in themes.
var (
	// ThemeOpencode mirrors opencode's installer palette — friendly
	// orange + soft green. The default for visual continuity between
	// `curl … | bash` and the installed binary.
	ThemeOpencode = Theme{
		Name:      "opencode",
		Mute:      faint,
		Accent:    fg("214"), // orange
		OK:        fg("114"), // soft green
		Err:       fg("203"),
		Info:      fg("110"), // cyan
		BannerTop: faint,
		BannerBot: fg("214"), // orange
	}

	// ThemeMetal leans into the kvlt/cult metal aesthetic — bone
	// muted, blood-red accent, cold-steel info. Errors stay brighter
	// red so they're distinct from the dull-red accent.
	ThemeMetal = Theme{
		Name:      "metal",
		Mute:      faint,
		Accent:    fg("124"), // blood red
		OK:        fg("108"), // ash green
		Err:       fg("203"), // brighter red — distinct from accent
		Info:      fg("67"),  // cold steel blue-grey
		BannerTop: faint,
		BannerBot: fg("124"),
	}

	// ThemeCrimson is metal with the saturation cranked — brighter
	// red across the board, slightly less austere than metal but
	// still firmly in the "danger / dark" register.
	ThemeCrimson = Theme{
		Name:      "crimson",
		Mute:      faint,
		Accent:    fg("160"), // bright crimson
		OK:        fg("107"),
		Err:       fg("196"),
		Info:      fg("103"),
		BannerTop: faint,
		BannerBot: fg("160"),
	}

	// ThemeNoir is monochrome — only greys and a single white accent.
	// For operators who hate color but still want hierarchy. Errors
	// remain red because no-color in errors is unsafe ergonomics.
	ThemeNoir = Theme{
		Name:      "noir",
		Mute:      faint,
		Accent:    fg("255"), // bright white
		OK:        fg("250"),
		Err:       fg("203"),
		Info:      fg("245"),
		BannerTop: faint,
		BannerBot: fg("255"),
	}

	// ThemeForge is the middle-ground compromise — orange ember kept
	// from opencode for the accent, but a steelier grey palette and
	// muted greens. Reads industrial rather than friendly.
	ThemeForge = Theme{
		Name:      "forge",
		Mute:      faint,
		Accent:    fg("208"), // ember orange
		OK:        fg("107"),
		Err:       fg("167"),
		Info:      fg("67"),
		BannerTop: faint,
		BannerBot: fg("208"),
	}

	// ThemeAsh — the desaturated take on the metal aesthetic. Brick-
	// dust accent (131) instead of blood red, soft greys throughout.
	// Atmospheric without the harshness; reads like a faded patch on
	// a denim jacket.
	ThemeAsh = Theme{
		Name:      "ash",
		Mute:      faint,
		Accent:    fg("131"), // dusty brick
		OK:        fg("108"), // ash green
		Err:       fg("167"),
		Info:      fg("103"), // muted blue-grey
		BannerTop: faint,
		BannerBot: fg("131"),
	}

	// ThemeOxblood pushes red as far toward black as it can go while
	// still reading as red — a very dark wine accent (88) on dim
	// muted greys. Brooding; the accent is barely a color at all,
	// which is what makes it feel kvlt rather than warning-light.
	// Errors stay brighter so they remain distinguishable from the
	// accent.
	ThemeOxblood = Theme{
		Name:      "oxblood",
		Mute:      faint,
		Accent:    fg("88"), // very dark wine
		OK:        fg("100"),
		Err:       fg("124"), // brighter than accent
		Info:      fg("60"),  // dusty navy
		BannerTop: faint,
		BannerBot: fg("88"),
	}

	// ThemeRust shifts the accent toward orange-red — weathered
	// industrial rather than gothic metal. 130 reads as oxidized
	// iron on a sun-bleached fence; pairs naturally with steel-grey
	// muted text.
	ThemeRust = Theme{
		Name:      "rust",
		Mute:      faint,
		Accent:    fg("130"), // rust orange-red
		OK:        fg("107"),
		Err:       fg("167"),
		Info:      fg("66"),
		BannerTop: faint,
		BannerBot: fg("130"),
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

// ThemeNames returns every registered theme name. The first element
// is the default (ash); subsequent are alphabetical so listings are
// deterministic.
func ThemeNames() []string {
	out := make([]string, 0, len(themes))
	for _, t := range themes {
		out = append(out, t.Name)
	}
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

// rendererFor returns a lipgloss renderer bound to w. lipgloss's
// global default detects against os.Stdout; for callers writing
// elsewhere (e.g., os.Stderr, a buffer in tests) we get accurate
// NO_COLOR / TTY behavior by binding the renderer to the actual
// destination.
func rendererFor(w io.Writer) *lipgloss.Renderer {
	if f, ok := w.(*os.File); ok {
		return lipgloss.NewRenderer(f)
	}
	// Fall back to the default renderer for non-file writers (tests,
	// buffers). lipgloss returns plain text when output isn't a TTY.
	return lipgloss.DefaultRenderer()
}

func render(w io.Writer, st lipgloss.Style, s string) string {
	return st.Renderer(rendererFor(w)).Render(s)
}

// Mute returns s rendered as secondary text per the active theme.
func Mute(w io.Writer, s string) string { return render(w, active.Mute, s) }

// Accent returns s rendered as the brand accent color.
func Accent(w io.Writer, s string) string { return render(w, active.Accent, s) }

// OK returns s in the success color.
func OK(w io.Writer, s string) string { return render(w, active.OK, s) }

// Err returns s in the error color.
func Err(w io.Writer, s string) string { return render(w, active.Err, s) }

// Info returns s in the cool-toned info/hint color.
func Info(w io.Writer, s string) string { return render(w, active.Info, s) }

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
// enough vertical room for a true middle stripe.
func Banner(w io.Writer) string {
	const top = "█▄▀ █░█ █░░ ▀█▀"
	const bot = "█░█ ▀▄▀ █▄▄ ░█░"
	return render(w, active.BannerTop, top) + "\n" +
		render(w, active.BannerBot, bot) + "\n"
}

// Success renders a leading "✓" mark in the OK color followed by msg.
// Falls back to "[ok]" when lipgloss decides not to color (NO_COLOR /
// non-TTY) — we detect that by rendering an empty span and checking
// for escape bytes.
func Success(w io.Writer, msg string) string {
	mark := OK(w, "✓")
	if !strings.ContainsRune(mark, 0x1b) {
		return "[ok] " + msg
	}
	return mark + " " + msg
}

// Failure mirrors Success for error one-liners.
func Failure(w io.Writer, msg string) string {
	mark := Err(w, "✗")
	if !strings.ContainsRune(mark, 0x1b) {
		return "[err] " + msg
	}
	return mark + " " + msg
}
