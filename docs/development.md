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
go run . vault create local dev
go run . secret put dev API_KEY=hunter2
go run . secret get dev API_KEY
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
  ├── store.go                   # newStore() shared helper
  ├── errors.go                  # exit-code mapping (mapGetError)
  ├── vault.go                   # `vault` parent command
  ├── vault_create.go            # `vault create <type> <name>`
  ├── vault_list.go              # `vault list`
  ├── vault_info.go              # `vault info <name>`
  ├── secret.go                  # `secret` parent command
  ├── secret_put.go              # `secret put <vault> <key>[=val]`
  ├── secret_get.go              # `secret get <vault> <key>`
  ├── secret_list.go             # `secret list <vault>`
  ├── env.go                     # `env <vault>` (eval-style export)
  └── run.go                     # `run <vault> -- cmd…` (exec wrapper)
pkg/kvlt/                        # public, importable library
  ├── provider.go                # Provider interface, Config struct, Describer
  ├── errors.go                  # typed sentinels (ErrKeyNotFound, …)
  ├── factory.go                 # ProviderFactory + RegisterBackend registry
  ├── store.go                   # Store + NewStore + Create + Open + List
  ├── identity.go                # SSH identity / passphrase loading helpers
  ├── local.go                   # LocalProvider (age + SSH recipients) — default
  └── backend_aws.go             # NewAWSProvider (build tag: aws, planned —
                                 # only if a real dev→prod use case lands)
internal/version/                # build identity (ldflag targets)
internal/cli/                    # CLI-only helpers (TTY prompt, themed
                                 # output) — never imported by pkg/kvlt
```

Cloud backends sit behind build tags so the base binary has zero third-party
runtime deps beyond cobra/viper. Operators who want aws-sm can
`go build -tags aws -o kvlt .` (or use a goreleaser variant build) without
forcing the AWS SDK on every consumer.

## Dependencies

| Package                     | Purpose                                 |
| --------------------------- | --------------------------------------- |
| `spf13/cobra`               | CLI command tree                        |
| `spf13/viper`               | Config + env-var binding (KVLT\_\*)     |
| `caarlos0/go-version`       | `kvlt version` JSON output              |
| `golang.org/x/term`         | TTY detection + passphrase prompts      |
| `golang.org/x/crypto/ssh`   | SSH key parsing (recipients/identities) |
| `filippo.io/age` + `agessh` | Encryption + SSH-key support            |

The library (`pkg/kvlt`) does no logging — errors flow back through `RunE` to a
single rendering path in `cmd/root.go`. Cloud backends pull in their respective
SDKs; see each `backend_*.go` for the exact import.

## How the Local Backend Works

The local backend defers all crypto to `filippo.io/age`. There is no master key
file — decryption uses the user's existing SSH private key.

1. On `vault create local <name>` we read the recipient list (default
   `~/.ssh/id_ed25519.pub`, override with `--public-key`), validate each
   recipient parses as an SSH or age public key, and write the vault config YAML
   at `.kvlt/vaults/local_encryption/{id}.yaml` with the canonical recipient
   strings stored in `settings.recipients`.
2. On `secret put`, we read the recipients from the config, call `age.Encrypt`
   with them, write the resulting age container to
   `.kvlt/secrets/local_encryption/<vault>/<key>.age` (mode `0600`, atomic via
   tempfile + fsync + rename).
3. On `secret get`, we read the `.age` blob, ask the IdentityResolver for
   available age identities (the user's SSH private keys, prompting for
   passphrase on `/dev/tty` if needed), call `age.Decrypt`, return the
   plaintext.
4. `secret list` walks the directory and returns the basenames sans `.age` — no
   decryption involved, so list never prompts for a passphrase.

The on-disk `.age` blobs are valid age containers —
`age -d -i ~/.ssh/id_ed25519 …` decrypts them directly without kvlt. That's a
deliberate property keeping kvlt a convenience layer rather than a format
lock-in. Operators rotating recipients today must `vault migrate` to a new vault
(planned).

## Test Conventions

**One `Test<FunctionName>` per public function**, with a table-driven body
covering every relevant scenario. Avoid scattering one-off
`Test<Func>_<scenario>` functions across the file — they fragment what should be
a single inventory of cases per behavior.

```go
func TestCanonicalizeType(t *testing.T) {
    t.Parallel()
    cases := []struct {
        name, in, want string
    }{
        {"local alias",          "local",            TypeLocalEncryption},
        {"hyphen typo",          "local-encryption", TypeLocalEncryption},
        {"already canonical",    TypeLocalEncryption, TypeLocalEncryption},
        {"unknown passes through", "unknown",         "unknown"},
        {"empty passes through", "",                 ""},
        {"case-sensitive",       "AWS-SM",           "AWS-SM"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            t.Parallel()
            if got := CanonicalizeType(tc.in); got != tc.want {
                t.Fatalf("CanonicalizeType(%q) = %q, want %q", tc.in, got, tc.want)
            }
        })
    }
}
```

Rules:

- **One test function per public function.** `TestNewStore`, `TestStore_Create`,
  `TestStore_Open`, etc. Internal helpers (`stringsToAny`, `validateVaultName`)
  follow the same convention when they have meaningful branching.
- **Each `cases` entry has a `name` field** so failures point at the scenario,
  not an index.
- **Each subtest runs `t.Parallel()`** unless it touches process-wide state
  (`t.Setenv`, registry mutations).
- **Subtest setup that's identical across cases lives outside the loop**;
  per-case fixtures live inside `t.Run`.
- **Fatalf message includes the inputs and got/want.** A failure should make
  diagnosis possible without rerunning under a debugger.
- **Unhappy paths assert via `errors.Is` against package sentinels**
  (`ErrInvalidConfig`, `ErrKeyNotFound`, …), not against error message strings.
- **Helpers (`newTestStore`, `generateSSHKeyPair`) call `t.Helper()`** so test
  failures point at the call site.
- **Assert at system boundaries, not on internal state.** Round-trip via `Put` →
  `Get` rather than mutating unexported fields. White-box access to fields is
  reserved for read-only assertions on construction.
- **No mocks for `filippo.io/age`.** It's audited and stable — round-trip
  behavior is the contract we test. Defensive `os.*` error returns inside
  `writeFileAtomic` and friends are intentionally not chased to 100%;
  introducing a filesystem abstraction to cover them adds more risk than it
  removes.

## Sister Projects

| Project                                                        | Description                             |
| -------------------------------------------------------------- | --------------------------------------- |
| [tlock](https://github.com/retr0h/tlock)                       | Terminal lock screen for macOS          |
| [meshx](https://github.com/retr0h/meshx)                       | Glitched-out terminal Meshtastic client |
| [grind](https://github.com/retr0h/grind)                       | (sister CLI)                            |
| [osapi-justfiles](https://github.com/osapi-io/osapi-justfiles) | Shared justfile recipes for Go projects |
