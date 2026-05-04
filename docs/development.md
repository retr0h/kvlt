# Development Guide

## Prerequisites

- [Go](https://go.dev/dl/) 1.25+
- [just](https://github.com/casey/just) — command runner
- [golangci-lint](https://golangci-lint.run/) — Go linter

## Getting Started

```bash
git clone https://github.com/retr0h/kvlt.git
cd kvlt
just fetch    # Fetch shared justfiles
just deps     # Install tool dependencies
```

## Common Commands

```bash
just deps          # Install all dependencies
just test          # Run all tests (lint + format check + unit + coverage)
just ready         # Format, lint before committing
just go::unit      # Run unit tests only
just go::vet       # Run golangci-lint
just go::fmt       # Auto-format (gofumpt + golines)
just just::fmt     # Format justfiles
```

## Running

```bash
go run . version
go run . vault create local-encryption dev
go run . put dev API_KEY=hunter2
go run . get dev API_KEY
```

## Architecture

See [architecture.md](architecture.md) for the design — provider interface,
on-disk layout, built-in backends, the HTTP service, and how to add a new
backend.

Quick map:

```
main.go                          # entrypoint — calls cmd.Execute
cmd/                             # cobra command tree (CLI surface)
  ├── root.go                    # root command, viper/slog wiring
  ├── version.go                 # `kvlt version` (build identity)
  └── …                          # vault_create / put / get / list_keys / migrate
                                 # — all import pkg/kvlt the same way an
                                 # external Go program would
pkg/kvlt/                        # public, importable library
  ├── provider.go                # Provider interface, Config struct
  ├── errors.go                  # typed sentinels (ErrKeyNotFound, …)
  ├── store.go                   # Store + NewStore — opens .kvlt/, looks up
                                 # vaults by name, dispatches to providers
  ├── local.go                   # NewLocalProvider — AES-GCM backend (default)
  ├── backend_aws.go             # NewAWSProvider     (build tag: aws)
  ├── backend_azure.go           # NewAzureProvider   (build tag: azure)
  └── backend_1password.go       # NewOnePassProvider (build tag: onepass)
internal/version/                # build identity (ldflag targets)
internal/cli/                    # CLI-only helpers (TTY prompt, colored
                                 # output) — never imported by pkg/kvlt
```

Cloud backends sit behind build tags so the base binary has zero third-party
runtime deps beyond cobra/viper/slog. Operators who want aws-sm can
`go build -tags aws -o kvlt .` (or use a goreleaser variant build) without
forcing the AWS SDK on every consumer.

## Dependencies

| Package                  | Purpose                                |
| ------------------------ | -------------------------------------- |
| `spf13/cobra`            | CLI command tree                       |
| `spf13/viper`            | Config + env-var binding (KVLT_*)      |
| `lmittmann/tint`         | Colored slog output to TTY             |
| `caarlos0/go-version`    | `kvlt version` JSON output             |
| `golang.org/x/term`      | TTY detection for log coloring         |
| _(stdlib)_ `crypto/aes`  | Local AES-GCM backend                  |

Cloud backends pull in their respective SDKs; see each `backend_*.go` for
the exact import.

## How the Local Backend Works

1. On `vault create local-encryption <name>` we generate a random 32-byte
   AES key, write it to `.kvlt/secrets/local_encryption/<name>/.key` with
   `0600` permissions.
2. On `put`, we open the key file, `crypto/aes.NewCipher` + `cipher.NewGCM`,
   prepend a fresh 12-byte nonce, write `<nonce><ciphertext>` to
   `<name>/<key>.enc`.
3. On `get`, we read the file, split nonce/ciphertext, decrypt.
4. `list` walks the directory and returns the basenames (sans `.enc`).

The key file is **never** committed; `.gitignore` already excludes
`.kvlt/`. Operators rotating the key today must `migrate` to a new vault.

## Sister Projects

| Project                                                        | Description                              |
| -------------------------------------------------------------- | ---------------------------------------- |
| [tlock](https://github.com/retr0h/tlock)                       | Terminal lock screen for macOS           |
| [meshx](https://github.com/retr0h/meshx)                       | Glitched-out terminal Meshtastic client  |
| [grind](https://github.com/retr0h/grind)                       | (sister CLI)                             |
| [osapi-justfiles](https://github.com/osapi-io/osapi-justfiles) | Shared justfile recipes for Go projects  |
