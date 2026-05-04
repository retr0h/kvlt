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
	"context"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/retr0h/kvlt/internal/cli"
)

var (
	importVault     string
	importFile      string
	importEnv       string
	importKey       string
	importPrefix    string
	importOverwrite bool
)

// secretImportCmd reads a file from disk and stores its contents as
// secret(s). Two modes, mutually exclusive — exactly one of --file or
// --env names the path to read:
//
//	--file <path> --key <name>   one file → one secret stored under <name>
//	--env <path>                 dotenv file → one secret per KEY=VALUE line
//
// The verb is shaped this way — rather than `secret put --file` — so
// "this command reads a file from disk" lives in one place. `put` is
// for a single value supplied inline, on stdin, or interactively.
var secretImportCmd = &cobra.Command{
	Use:   "import",
	Short: "Import a file into a vault — one secret, or many via --env",
	Long: `Two modes, mutually exclusive — exactly one is required:

  --file <path> --key <name>   store the file's bytes verbatim under <name>
  --env <path>                 parse the file as KEY=VALUE (dotenv) and
                               store one secret per line

Single-file mode (--file) reads bytes verbatim — no newline trimming —
so secrets that are whole files (kubeconfig, SSL key, service-account
JSON) preserve trailing whitespace and final newlines.

Dotenv mode (--env) skips blank lines and ` + "`#`" + ` comments, supports plain
` + "`KEY=value`" + `, ` + "`KEY=\"quoted\"`" + `, and ` + "`KEY='single'`" + `. Shell variable
interpolation (` + "`${OTHER}`" + `) is intentionally not supported — secret
values should be literal, not derived from the host environment.

Examples:
  kvlt secret import --vault dev --key kubeconfig --file ~/kc.yaml
  kvlt secret import --vault dev --env ~/.env
  kvlt secret import --vault dev --env ~/.env --prefix MY_APP_
  kvlt secret import --vault dev --env ~/.env --overwrite`,
	Args: cobra.NoArgs,
	RunE: runSecretImport,
}

func init() {
	secretImportCmd.Flags().StringVarP(&importVault, "vault", "v", "",
		"target vault name (required)")
	secretImportCmd.Flags().StringVarP(&importFile, "file", "f", "",
		"path to a single file to store as one secret (use with --key)")
	secretImportCmd.Flags().StringVarP(&importEnv, "env", "e", "",
		"path to a KEY=VALUE dotenv file to import in bulk")
	secretImportCmd.Flags().StringVarP(&importKey, "key", "k", "",
		"secret key for --file mode (required with --file)")
	secretImportCmd.Flags().StringVar(&importPrefix, "prefix", "",
		"prepend prefix to every imported key (--env mode only)")
	secretImportCmd.Flags().BoolVar(&importOverwrite, "overwrite", false,
		"overwrite keys that already exist (default: error if any collide)")
	_ = secretImportCmd.MarkFlagRequired("vault")
	secretImportCmd.MarkFlagsMutuallyExclusive("file", "env")
	secretImportCmd.MarkFlagsOneRequired("file", "env")
	secretImportCmd.MarkFlagsRequiredTogether("file", "key")
	secretCmd.AddCommand(secretImportCmd)
}

func runSecretImport(_ *cobra.Command, _ []string) error {
	// MarkFlagsMutuallyExclusive + MarkFlagsOneRequired guarantee that
	// exactly one of importFile/importEnv is non-empty by the time we
	// get here, so we just branch on which one was set.
	if importPrefix != "" && importEnv == "" {
		return fmt.Errorf("--prefix only applies with --env (batch mode)")
	}

	path := importFile
	if importEnv != "" {
		path = importEnv
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read %q: %w", path, err)
	}

	store, err := newStoreForRead("")
	if err != nil {
		return err
	}
	provider, err := store.Open(importVault)
	if err != nil {
		return mapGetError(err)
	}
	ctx := context.Background()

	if importEnv != "" {
		return runImportEnv(ctx, provider, raw, importEnv)
	}
	return runImportFile(ctx, provider, raw, importFile)
}

// runImportFile stores the file's bytes verbatim under --key. We do
// not trim a trailing newline — secrets that are whole files often
// have meaningful trailing whitespace; preserving exact bytes is the
// safe default.
func runImportFile(ctx context.Context, provider importPutter, raw []byte, path string) error {
	if !importOverwrite {
		existing, lerr := provider.List(ctx)
		if lerr != nil {
			return lerr
		}
		if slices.Contains(existing, importKey) {
			return fmt.Errorf(
				"would overwrite existing key %q — pass --overwrite to allow",
				importKey,
			)
		}
	}

	if err := provider.Put(ctx, importKey, string(raw)); err != nil {
		return fmt.Errorf("put %q: %w", importKey, err)
	}

	out := os.Stdout
	_, _ = fmt.Fprintln(out, cli.Success(out, fmt.Sprintf("imported %s into vault %s %s",
		cli.Accent(out, importKey),
		cli.Accent(out, importVault),
		cli.Mute(out, "(from "+path+")"))))
	return nil
}

// runImportEnv parses raw as dotenv and stores one secret per pair.
// We collision-check up front before any Put so the import is
// all-or-nothing rather than partially-applied if the 47th key
// happens to collide.
func runImportEnv(ctx context.Context, provider importPutter, raw []byte, path string) error {
	pairs, parseErr := parseDotenv(string(raw))
	if parseErr != nil {
		return fmt.Errorf("parse %q: %w", path, parseErr)
	}
	if len(pairs) == 0 {
		return fmt.Errorf("no KEY=VALUE pairs found in %q", path)
	}

	if !importOverwrite {
		existing, lerr := provider.List(ctx)
		if lerr != nil {
			return lerr
		}
		exists := make(map[string]struct{}, len(existing))
		for _, k := range existing {
			exists[k] = struct{}{}
		}
		var collisions []string
		for _, p := range pairs {
			if _, hit := exists[importPrefix+p.Key]; hit {
				collisions = append(collisions, importPrefix+p.Key)
			}
		}
		if len(collisions) > 0 {
			return fmt.Errorf(
				"would overwrite %d existing key(s): %s — pass --overwrite to allow",
				len(collisions),
				strings.Join(collisions, ", "),
			)
		}
	}

	for _, p := range pairs {
		key := importPrefix + p.Key
		if err := provider.Put(ctx, key, p.Value); err != nil {
			return fmt.Errorf("put %q: %w", key, err)
		}
	}

	out := os.Stdout
	_, _ = fmt.Fprintln(out, cli.Success(out, fmt.Sprintf("imported %s into vault %s %s",
		cli.Accent(out, fmt.Sprintf("%d secret(s)", len(pairs))),
		cli.Accent(out, importVault),
		cli.Mute(out, "(from "+path+")"))))
	return nil
}

// importPutter is the small slice of the Provider surface this file
// needs — declared locally so tests can swap in a fake without
// pulling the whole pkg/kvlt interface.
type importPutter interface {
	Put(ctx context.Context, key, value string) error
	List(ctx context.Context) ([]string, error)
}

// dotenvPair is one parsed KEY=VALUE entry from a .env file.
type dotenvPair struct {
	Key, Value string
}

// parseDotenv parses a small but practical subset of the dotenv
// format used by direnv, npm, docker-compose, Django, etc:
//
//   - blank lines and lines starting with `#` are skipped
//   - leading `export ` is stripped so files produced by `kvlt env`
//     (or any shell-sourceable .env) round-trip cleanly
//   - `KEY=value`, `KEY="quoted value"`, `KEY='single quoted'` work
//   - inline comments are NOT supported (a `#` inside an unquoted
//     value is part of the value, matching docker-compose behavior)
//   - shell variable interpolation (`${OTHER}`) is NOT supported and
//     is intentional: secret values should be literal, not derived
//     from the host environment at import time
//
// Anything that doesn't match `KEY=…` is rejected with a line number
// so the operator gets an actionable error.
func parseDotenv(s string) ([]dotenvPair, error) {
	var out []dotenvPair
	lineNum := 0
	for line := range strings.SplitSeq(s, "\n") {
		lineNum++
		raw := strings.TrimSpace(line)
		if raw == "" || strings.HasPrefix(raw, "#") {
			continue
		}
		// Strip a leading `export ` so shell-sourceable .env files
		// (including our own `kvlt env` output) round-trip without
		// the keyword leaking into the key name.
		raw = strings.TrimPrefix(raw, "export ")
		raw = strings.TrimLeft(raw, " \t")
		eq := strings.IndexByte(raw, '=')
		if eq <= 0 {
			return nil, fmt.Errorf("line %d: expected KEY=VALUE, got %q", lineNum, raw)
		}
		key := strings.TrimSpace(raw[:eq])
		val := raw[eq+1:]

		// Strip surrounding matched quotes if present. We only handle
		// the common cases (whole-value quoted); embedded escapes are
		// left as-is since secrets shouldn't need shell-style escapes.
		if len(val) >= 2 {
			if (val[0] == '"' && val[len(val)-1] == '"') ||
				(val[0] == '\'' && val[len(val)-1] == '\'') {
				val = val[1 : len(val)-1]
			}
		}

		if key == "" {
			return nil, fmt.Errorf("line %d: empty key", lineNum)
		}
		out = append(out, dotenvPair{Key: key, Value: val})
	}
	return out, nil
}
