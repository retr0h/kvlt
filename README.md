[![release](https://img.shields.io/github/release/retr0h/kvlt.svg?style=for-the-badge)](https://github.com/retr0h/kvlt/releases/latest)
[![go report card](https://goreportcard.com/badge/github.com/retr0h/kvlt?style=for-the-badge)](https://goreportcard.com/report/github.com/retr0h/kvlt)
[![license](https://img.shields.io/badge/license-MIT-brightgreen.svg?style=for-the-badge)](LICENSE)
[![build](https://img.shields.io/github/actions/workflow/status/retr0h/kvlt/go.yml?style=for-the-badge)](https://github.com/retr0h/kvlt/actions/workflows/go.yml)
[![release](https://img.shields.io/github/actions/workflow/status/retr0h/kvlt/release.yml?style=for-the-badge&label=release)](https://github.com/retr0h/kvlt/actions/workflows/release.yml)
[![powered by](https://img.shields.io/badge/powered%20by-goreleaser-green.svg?style=for-the-badge)](https://github.com/goreleaser)
[![just](https://img.shields.io/badge/just-command%20runner-blue?style=for-the-badge)](https://github.com/casey/just)
[![conventional commits](https://img.shields.io/badge/Conventional%20Commits-1.0.0-yellow.svg?style=for-the-badge)](https://conventionalcommits.org)
[![go reference](https://img.shields.io/badge/go-reference-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://pkg.go.dev/github.com/retr0h/kvlt)
![github commit activity](https://img.shields.io/github/commit-activity/m/retr0h/kvlt?style=for-the-badge)

<h1 align="center">
<pre>
█▄▀ █░█ █░░ ▀█▀
█░█ ▀▄▀ █▄▄ ░█░
</pre>
</h1>

<p align="center">🔐 Pluggable secrets vault. Local-first. No daemon.</p>

A single-binary secrets vault for projects that don't have HashiCorp
Vault and don't want one. Encrypts locally with AES-GCM out of the box;
swap in AWS Secrets Manager, Azure Key Vault, 1Password, or HashiCorp
Vault later without changing the call sites.

## ✨ Features

- 🔒 **AES-GCM local backend** — default, ships in the base binary, zero runtime deps
- 🪪 **Named vaults** — `kvlt get prod API_KEY`, never `kvlt get_aws(…)`; backends are an implementation detail
- 🔌 **Pluggable backends** — `aws-sm`, `azure-kv`, `1password`, `hashicorp-vault` behind build tags so consumers only pay for what they use
- 🔁 **Backend migration** — `kvlt vault migrate <name> --to-type aws-sm` is copy-then-swap; source stays intact on partial failure
- 🤫 **Stdin / TTY input modes** — `echo $VAL | kvlt put` keeps secrets out of shell history; bare `kvlt put` prompts with echo off
- 🚫 **No service, no daemon** — pure CLI; nothing listening, nothing persistent
- 📦 **Single static binary** — Go, CGO off, darwin / linux / windows × amd64 / arm64

## 📦 Install

```bash
curl -fsSL https://github.com/retr0h/kvlt/raw/main/install.sh | sh
```

Installs to `~/.local/bin` (or `/usr/local/bin` as root) — SHA256 checksums verified. Override with `KVLT_INSTALL_DIR=/some/path` or pin a version with `KVLT_VERSION=1.1.1`.

### 🔨 Build from source

```bash
git clone https://github.com/retr0h/kvlt.git
cd kvlt
go build -o kvlt .
install -m 755 kvlt ~/.local/bin/kvlt
```

Cloud backends opt in via build tags: `go build -tags aws -o kvlt .`

## 🚀 Quick start

```sh
kvlt vault create local-encryption dev    # bootstrap a vault
kvlt put dev API_KEY=sk-1234              # store a secret
echo "$DB_PASS" | kvlt put dev DB_PASS    # stdin → no shell history
kvlt put dev TOKEN                        # interactive, echo off
kvlt get dev API_KEY                      # retrieve
kvlt list-keys dev                        # names only, never values
```

Full command reference in [`docs/commands.md`](docs/commands.md).

## ⚙️ How It Works

`kvlt` is a CLI; nothing runs between invocations. Each command opens
the vault config, talks to the backend, and exits.

1. 🪪 **Pick a vault by name** — every verb takes a name (`dev`, `prod`, …); the name resolves to a backend through `vaults/<type>/<id>.yaml`
2. 🔐 **Default backend is local AES-GCM** — random 32-byte key in `.kvlt/secrets/local_encryption/<name>/.key` (`0600`, gitignored), `<nonce><ciphertext>` blobs in `<key>.enc`
3. 🔌 **Cloud backends sit behind build tags** — base binary is local-only; `-tags aws` adds Secrets Manager, `-tags azure` adds Key Vault, etc.
4. 🔁 **`migrate` is copy-then-swap** — list keys, copy each value to the new backend, write the new config, delete the old one. Source stays functional until the very last step

The contract every backend implements is four methods (`Get` / `Put` /
`List` / `Name`) — small on purpose. Anything fancier is layered on top
by callers, not pushed into the backend.

## 💡 Inspiration

- **[swamp](https://github.com/systeminit/swamp)** — the named-vault model, the `vaults/<type>/<id>.yaml` layout, and the copy-then-swap migrate semantics are lifted directly from swamp's vault subsystem
- **[SOPS](https://github.com/getsops/sops)** — the "encrypted files in a repo, no server required" mindset
- **[grind](https://github.com/retr0h/grind), [tlock](https://github.com/retr0h/tlock), [meshx](https://github.com/retr0h/meshx)** — sibling retr0h CLIs; same scaffold, same justfile setup, same MIT vibes

## 🔀 Alternatives

| Tool                                                      | Description                                       |
| --------------------------------------------------------- | ------------------------------------------------- |
| [HashiCorp Vault](https://www.vaultproject.io/)           | Full-featured secret-management platform          |
| [OpenBao](https://openbao.org/)                           | Open-source fork of Vault                         |
| [SOPS](https://github.com/getsops/sops)                   | Encrypted files in git, age/PGP/cloud-KMS keys    |
| [1Password CLI](https://developer.1password.com/docs/cli) | If you already live in 1Password                  |
| [pass](https://www.passwordstore.org/)                    | GPG-encrypted files, the Unix way                 |

`kvlt` is meant for the gap below "I need a Vault cluster" and above
"I have a `.env` file."

## 🗺️ Roadmap

- [x] 🪪 Project scaffold + CLI tree
- [ ] 🔐 `local_encryption` backend (AES-GCM)
- [ ] 🚪 `vault create` / `put` / `get` / `list-keys`
- [ ] 🔁 `vault migrate` (copy-then-swap)
- [ ] 🔌 AWS Secrets Manager backend (`-tags aws`)
- [ ] 🔌 Azure Key Vault backend (`-tags azure`)
- [ ] 🔌 1Password backend via `op` CLI (`-tags onepass`)
- [ ] 🔌 HashiCorp Vault / OpenBao backend (`-tags hcv`)

## 📚 Docs

- [docs/recipes.md](docs/recipes.md) — `.envrc` / `direnv`, `kvlt run`, GitLab + GitHub CI, Unix-pipe patterns, dotfiles, multi-vault setups
- [docs/architecture.md](docs/architecture.md) — provider interface, on-disk layout, backend internals, migration semantics
- [docs/development.md](docs/development.md) — setup, testing, conventions
- [docs/contributing.md](docs/contributing.md) — PR workflow

## 📄 License

The [MIT][] License.

[MIT]: LICENSE
