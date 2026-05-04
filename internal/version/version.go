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

// Package version is the single source of build identity for the
// `kvlt version` cobra command. Lives in its own package so any
// future surface (in-app reporter, HTTP /version endpoint) can
// import it without forming a cycle through cmd/. goreleaser stamps
// the ldflag targets at link time; `go run` / `go build` of a
// working tree leaves them empty (caarlos0/go-version backfills
// devel defaults from debug.ReadBuildInfo).
package version

import (
	goversion "github.com/caarlos0/go-version"
)

// Build-time identity. Populated by goreleaser via -ldflags -X at
// link time; left blank for `go run` / `go build` of a working
// tree (caarlos0/go-version backfills "(devel)" defaults from
// debug.ReadBuildInfo when the strings are empty). Exported so
// goreleaser's -X flags can target them — the goreleaser config
// targets internal/version.{Version,Commit,…}.
//
//nolint:revive
var (
	Version   = ""
	Commit    = ""
	TreeState = ""
	Date      = ""
	BuiltBy   = ""
)

// BuildInfo stitches the goreleaser ldflag values into a
// goversion.Info — used by `kvlt version` for JSON output. Single
// source of truth so any future surface can show identical data.
func BuildInfo() goversion.Info {
	return goversion.GetVersionInfo(
		goversion.WithAppDetails(
			"kvlt",
			"a pluggable secrets vault — local AES-GCM by default, cloud backends optional.\n",
			"https://github.com/retr0h/kvlt",
		),
		func(i *goversion.Info) {
			if Commit != "" {
				i.GitCommit = Commit
			}
			if TreeState != "" {
				i.GitTreeState = TreeState
			}
			if Date != "" {
				i.BuildDate = Date
			}
			if Version != "" {
				i.GitVersion = Version
			}
			if BuiltBy != "" {
				i.BuiltBy = BuiltBy
			}
		},
	)
}
