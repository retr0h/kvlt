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
	"errors"
	"fmt"
	"os"
	"os/exec"

	"github.com/spf13/cobra"
)

// runCmd injects every secret in a vault as an env var into a child
// process and execs it. Same pattern as `aws-vault exec` and
// `op run`. Secrets enter the child's environment, never the parent
// shell's — so `kvlt run dev -- npm start` lets npm see API_KEY but
// `env` afterward in your shell does not.
//
// Argument parsing requires the `--` separator before the command,
// so flags consumed by kvlt don't accidentally bind to the child:
//
//	kvlt run dev -- npm start
//	kvlt run dev --only A,B -- python deploy.py
//	kvlt run dev -- bash -c 'echo $API_KEY'
var runCmd = &cobra.Command{
	Use:   "run <vault> [--only K1,K2] -- <cmd> [args…]",
	Short: "Inject vault secrets as env vars into a child process",
	Long: `Decrypt every secret in <vault> and exec <cmd> with those
secrets in its environment. The parent shell is unaffected.

Same model as aws-vault exec / op run — best for one-off commands
where the secrets should not persist after the command exits:

  kvlt run dev -- npm start
  kvlt run dev -- python manage.py migrate
  kvlt run dev -- bash -c 'curl -H "Authorization: Bearer $API_TOKEN" …'`,
	Args: func(cmd *cobra.Command, args []string) error {
		// Cobra's ArgsLenAtDash returns the index where `--` appears
		// in the original argv, or -1 if no `--`. Require at least
		// vault and a command after the dash.
		if cmd.ArgsLenAtDash() == -1 {
			return errors.New("missing `--` separator before the command (e.g. `kvlt run dev -- ls`)")
		}
		if cmd.ArgsLenAtDash() < 1 {
			return errors.New("missing vault name before `--`")
		}
		if len(args) <= cmd.ArgsLenAtDash() {
			return errors.New("missing command after `--`")
		}
		return nil
	},
	RunE: runRun,
}

var runOnlyKeys []string

func init() {
	runCmd.Flags().StringSliceVar(&runOnlyKeys, "only", nil,
		"comma-separated list of keys to inject (default: all)")
	rootCmd.AddCommand(runCmd)
}

func runRun(cmd *cobra.Command, args []string) error {
	dash := cmd.ArgsLenAtDash()
	vaultName := args[0]
	childArgv := args[dash:]

	ctx := context.Background()
	store, err := newStore()
	if err != nil {
		return err
	}
	provider, err := store.Open(vaultName)
	if err != nil {
		return mapGetError(err)
	}

	keys, err := provider.List(ctx)
	if err != nil {
		return err
	}
	if len(runOnlyKeys) > 0 {
		keys = filterKeys(keys, runOnlyKeys)
	}

	// Inherit the parent environment, then layer on vault secrets.
	// Vault secrets win over inherited collisions on purpose — the
	// vault is the authoritative source for keys it manages, and a
	// stale env-var leftover from a different project shouldn't
	// shadow today's value.
	childEnv := os.Environ()
	for _, k := range keys {
		val, err := provider.Get(ctx, k)
		if err != nil {
			return mapGetError(err)
		}
		childEnv = append(childEnv, fmt.Sprintf("%s=%s", k, val))
	}

	// exec.Command + Run lets us pass through stdio so the child's
	// I/O works naturally — interactive REPLs, progress bars, TTY
	// programs all behave as if invoked directly. We propagate the
	// child's exit status so shell pipelines (`kvlt run … && deploy`)
	// branch correctly.
	child := exec.Command(childArgv[0], childArgv[1:]...) //nolint:gosec // user explicitly chose this command
	child.Env = childEnv
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr

	if err := child.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			os.Exit(exitErr.ExitCode())
		}
		return fmt.Errorf("run %q: %w", childArgv[0], err)
	}
	return nil
}
