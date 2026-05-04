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
[![hovnokod](https://raw.githubusercontent.com/tekk/hovnokod-badge/main/assets/badges/hovnokod-for-the-badge.svg)](https://github.com/tekk/hovnokod-badge)

<h1 align="center">
<pre>
█▄▀ █░█ █░░ ▀█▀
█░█ ▀▄▀ █▄▄ ░█░
</pre>
</h1>

<p align="center">🔐 Pluggable secrets vault. Local-first. No daemon.</p>

A single-binary secrets vault for projects that don't have HashiCorp
Vault and don't want one. Encrypts with [age](https://github.com/FiloSottile/age)
using your existing SSH keys; named vaults give you a stable call
site (`kvlt get prod API_KEY`) regardless of whether the backend is
local age files today, SOPS or AWS Secrets Manager tomorrow.

## ✨ Features

- 🔐 **age + SSH keys** — encrypts with `~/.ssh/id_ed25519.pub`, decrypts with the matching private key. Borrows your existing protection chain (passphrase + ssh-agent + Touch ID via [secretive](https://github.com/maxgoedjen/secretive)); kvlt doesn't reinvent the lock.
- 🪪 **Named vaults** — `kvlt get prod API_KEY`, never `kvlt get_aws(…)`; the backend is an implementation detail. Same model as [swamp](https://github.com/systeminit/swamp)'s vault subsystem.
- 🔌 **Pluggable backends** — `Provider` interface + factory registry. SOPS, AWS Secrets Manager, Azure Key Vault, 1Password each become one new file behind a `//go:build <tag>` guard so the base binary stays dependency-light.
- 👥 **Multi-recipient** — encrypt to N SSH public keys, any one of those private keys can decrypt. The team-sharing escape hatch.
- 🤫 **Stdin / TTY input modes** — `echo $VAL | kvlt put` keeps secrets out of shell history; bare `kvlt put` prompts with echo off.
- 🐚 **Shell-friendly** — `kvlt env vault` for `eval "$(…)"` direnv integration; `kvlt run vault -- cmd` for scoped env injection like `aws-vault exec` / `op run`.
- 🚫 **No service, no daemon** — pure CLI; nothing listening, nothing persistent.
- 📦 **Single static binary** — Go, CGO off, darwin / linux / windows × amd64 / arm64.

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
kvlt vault create --name dev                                # bootstrap a vault (encrypts to ~/.ssh/id_ed25519.pub)
kvlt vault create --name prod -p ~/.ssh/team.pub            # encrypt to a non-default public key
kvlt secret put --vault dev --key API_KEY --value sk-1234   # store a secret (lands in shell history)
echo "$DB_PASS" | kvlt secret put --vault dev --key DB_PASS # stdin → no shell history
kvlt secret put --vault dev --key TOKEN                     # interactive, echo off
kvlt secret get --vault dev --key API_KEY                   # decrypt — prompts for SSH passphrase if not in agent
kvlt secret get --vault dev --key API_KEY -i ~/work/id_ed25519  # use a specific private key
kvlt secret list --vault dev                                # names only, never values
kvlt env --vault dev                                        # all secrets as `export KEY=VALUE` for `eval`
kvlt run --vault dev -- npm start                           # exec child with vault secrets in env
```

Override the default decrypt key globally with `KVLT_PRIVATE_KEY=/path/to/key`.

Full recipe collection in [docs/recipes.md](docs/recipes.md).

## 🛡️ Why SSH keys (and why this is meaningfully more secure)

Most secret stuff on a dev laptop is a plaintext file. `~/.aws/credentials`,
`~/.config/gh/hosts.yml` (your GitHub PAT), `.env` files, npm tokens in
`~/.npmrc`, GitLab tokens in `~/.config/glab-cli/`, every `.kube/config` —
all sitting there in plaintext, readable by any process running as you. Once
malware has user-level execution on your machine, every one of those is
immediate, no friction.

The clever bit isn't kvlt — it's **delegating the lock to the SSH
protection chain**, one of the few credential systems on a dev machine that
actually has a human-in-the-loop step:

| What's on disk                      | Attacker w/ user code-exec does | Result                                     |
| ----------------------------------- | ------------------------------- | ------------------------------------------ |
| `~/.aws/credentials` plaintext      | `cat`                           | full AWS access, instantly                 |
| `.env` with `STRIPE_KEY=…`          | `cat`                           | Stripe access, instantly                   |
| `gh` / `glab` / `npm` tokens        | `cat`                           | git host access, instantly                 |
| Swamp-style vault (key file in repo) | `cat key.txt && cat blob`       | decrypt, instantly — key file _is_ the secret |
| **kvlt + passphrase-locked SSH key** | reads `.age` + encrypted key file | **needs the passphrase**                  |
| **kvlt + ssh-agent (timed unlock)** | tries to decrypt                | needs the passphrase to (re-)unlock the agent |
| **kvlt + Secretive on macOS**       | tries to decrypt                | **needs your fingerprint** (Touch ID)      |

Every kvlt decrypt requires something that **isn't on disk**: your typed
passphrase, ssh-agent's in-memory unlock state, or a Touch ID prompt routed
through the Secure Enclave. Reading every file under `$HOME` gets the
attacker `.age` blobs — useless without the key — and an _encrypted_
private key file, useless without the passphrase. The credentials never
exist as plaintext at rest.

This is why a `.env` -> kvlt swap is a real upgrade, not just a re-shuffle.
The previous swamp-style approach (encryption key as a sibling text file)
didn't help: an attacker grabbing the vault would also grab the key. kvlt
moves the key out of the file system entirely.

**Honest about the limits:**

- **Cached ssh-agent unlock** — once the agent is unlocked, anything running
  as you can sign with it. Mitigate with `ssh-add -t 1h` for time-limited
  caching, or skip the agent entirely on macOS by using
  [Secretive](https://github.com/maxgoedjen/secretive) (key lives in the
  Secure Enclave, every signature requires Touch ID).
- **Keylogger on the box** captures the passphrase the next time you type
  it. Beyond software's job.
- **`.age` blobs are still copyable** — an attacker with the blobs can sit
  on them waiting for a future key compromise. Rotate keys and the
  underlying secrets when threat-modeling demands it.

In short: kvlt is exactly as protective as your SSH private key is, which
is far better than "as protective as a text file in `$HOME`."

## ⚙️ How It Works

`kvlt` is a CLI; nothing runs between invocations. Each command opens
the vault config, talks to the backend, and exits.

1. 🪪 **Pick a vault by name** — every verb takes a name (`dev`, `prod`, …); the name resolves to a backend through `vaults/<type>/<id>.yaml`
2. 🔐 **Default backend is `local` (age + SSH keys)** — `kvlt put` encrypts to one or more SSH public-key recipients via [age](https://github.com/FiloSottile/age); blobs land at `.kvlt/secrets/local_encryption/<vault>/<key>.age`. Decrypt requires the matching SSH private key — passphrase prompt fires on `/dev/tty` if your key isn't in ssh-agent already.
3. 🔌 **Backends are pluggable** — `Provider` interface + factory registry. Adding SOPS, AWS Secrets Manager, etc. is one new file behind a `//go:build <tag>` guard; the base binary stays dependency-light.
4. 🔁 **`migrate` is copy-then-swap** — list keys, copy each value to the new backend, write the new config, delete the old one. Source stays functional until the very last step. (Planned; the backend abstraction supports it cleanly.)

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
- [x] 🔐 `local` backend (age + SSH-key recipients)
- [x] 🚪 `vault create` / `put` / `get` / `list-keys`
- [x] 🐚 `kvlt env` / `kvlt run` for shell + child-process integration
- [x] 🔌 Pluggable backend registry (factory pattern)
- [ ] 🤝 ssh-agent integration (file-based passphrase prompt works today)
- [ ] 🔁 `vault migrate` (copy-then-swap)
- [ ] 🔌 SOPS backend (`-tags sops`)
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
