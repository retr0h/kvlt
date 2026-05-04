# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with
code in this repository.

## Project Overview

kvlt is a small, dependency-light secrets vault written in Go. It ships
as a single binary with no daemon — every command opens the vault, talks
to the backend, and exits. The default backend encrypts secrets locally
with AES-GCM and has no third-party runtime deps. Cloud backends (AWS
Secrets Manager, Azure Key Vault, 1Password, HashiCorp Vault) sit behind
build tags so consumers who don't need them never pay the SDK cost.

The named-vault design is borrowed from [swamp](https://github.com/systeminit/swamp)'s
vault subsystem: callers reference vaults by user-defined name, never by
backend type, so a project can start on `local_encryption` in dev and
migrate to `aws-sm` in prod without touching application code.

## Architecture

See [docs/architecture.md](docs/architecture.md) for the full design — the
`Provider` interface, on-disk layout, built-in backends, migration
semantics, and how to add a new backend.

```
main.go                          # entrypoint — calls cmd.Execute
cmd/                             # cobra command tree (CLI surface)
                                 #   imports pkg/kvlt the same way an
                                 #   external Go project would — the CLI
                                 #   dogfoods the public API
pkg/kvlt/                        # public, importable vault library
                                 #   import "github.com/retr0h/kvlt/pkg/kvlt"
                                 #   Provider interface, New* constructors,
                                 #   built-in backends, typed errors
internal/version/                # build identity (ldflag targets) — internal
                                 #   so external imports can't depend on
                                 #   the goreleaser stamping shape
internal/cli/                    # CLI-only helpers (interactive prompts,
                                 #   output formatting) — never imported
                                 #   by the public package
docs/                            # human docs (architecture + dev guides)
.github/                         # CI workflows + repo settings
```

**Rule of thumb:** if a behavior is useful to a downstream Go program
(open a vault, read a secret, list keys, migrate backends), it lives
under `pkg/kvlt/`. If it only makes sense in the context of running
`kvlt` on a terminal (TTY prompt with echo off, colored output, JSON
flag handling), it lives under `internal/`. The CLI is a thin
adapter — never duplicate logic from the package into commands.

## Key Technical Details

- **Zero runtime dependencies** for the local backend — only stdlib `crypto/aes`
  and `crypto/cipher`. Pure Go; CGO is off in goreleaser.
- **Cross-platform** — darwin/linux/windows × amd64/arm64 in the release
  matrix. No platform-specific syscalls in the core.
- **Backends behind build tags** — `aws`, `azure`, `onepass`, `hcv`. The base
  binary stays small; cloud variants are separate goreleaser builds.
- **Named vaults, not backend types** — every CLI verb takes a vault name; the
  name resolves to a backend via `vaults/<type>/<id>.yaml`.
- **`migrate` is copy-then-swap, never move** — source vault keeps its secrets
  until config is deleted, so a partial failure leaves the original intact.
- **Secret values never logged**, even at `--debug`. List operations return
  key names only.

## Building

```bash
go build -o kvlt .              # base binary (local_encryption only)
go build -tags aws -o kvlt .    # base + AWS Secrets Manager backend
go run . version                # quick sanity check
```

## Usage

```bash
# Bootstrap a local vault
kvlt vault create local-encryption dev

# Store / retrieve
kvlt put dev API_KEY=sk-1234
echo "$TOKEN" | kvlt put dev TOKEN     # stdin keeps it out of shell history
kvlt get dev API_KEY
kvlt list-keys dev

# Move backends without changing references
kvlt vault migrate dev --to-type aws-sm
```

## Code Standards

- Follow [Conventional Commits](https://www.conventionalcommits.org/) for
  commit messages
- Use `testify/suite` with table-driven tests
- Multi-line function signatures
- All `.go` files include the MIT copyright header at the top of the file
- golangci-lint with: errcheck, errname, govet, prealloc, predeclared, revive,
  staticcheck, unused
- Backend implementations live under `pkg/kvlt/backend_*.go`, behind
  build tags when they pull in cloud-vendor SDKs — the base binary stays
  dependency-light
- All public types are constructed via `New*` functions (`NewLocalProvider`,
  `NewStore`, …); zero-value structs of the package's exported types are not
  meant to be used directly
- Constructors return concrete types when they only have one shape
  (`*LocalProvider`); they return `Provider` only when the caller really
  shouldn't care which backend is on the other end
- Errors should name the vault and key (when safe to expose) so an operator
  can act without diffing logs
- Secret values must never appear in logs, even at `--debug`. Redact at the
  source — don't rely on a log filter

## Verification

After completing work, run these checks:

1. `go build -o kvlt .` — base binary compiles
2. `just go::vet` — golangci-lint
3. `just test` — fmt-check + unit tests
4. `go run . version` — sanity check entrypoint

## Roadmap

- [ ] `local_encryption` backend (AES-GCM)
- [ ] `vault create` / `put` / `get` / `list-keys` CLI verbs
- [ ] `kvlt vault migrate` (copy-then-swap-config)
- [ ] AWS Secrets Manager backend (`-tags aws`)
- [ ] Azure Key Vault backend (`-tags azure`)
- [ ] 1Password backend via `op` CLI (`-tags onepass`)
- [ ] HashiCorp Vault / OpenBao backend (`-tags hcv`)
