# kvlt recipes

Concrete patterns for using `kvlt` with the rest of your toolchain.
Every example assumes you've already created a vault and added some
secrets:

```bash
kvlt vault create local dev                    # SSH-key recipient by default
kvlt put dev API_KEY=sk-1234
kvlt put dev DB_PASSWORD=hunter2
kvlt put dev AWS_ACCESS_KEY_ID=AKIA…
echo "$LONG_TOKEN" | kvlt put dev API_TOKEN    # stdin → no shell history
kvlt put dev STRIPE_KEY                        # interactive prompt, echo off
```

The on-disk artifacts (`.kvlt/secrets/local/dev/*.age`) are valid age
containers — `age -d -i ~/.ssh/id_ed25519 .kvlt/secrets/local/dev/API_KEY.age`
also works. `kvlt` is a convenience layer, not a format lock-in.

## 1. `direnv` — encrypted `.envrc`

The killer use case: project-level env vars, encrypted at rest, only
your SSH identity decrypts.

**`.envrc`** (committed to git, contains zero secrets):

```bash
# .envrc
eval "$(kvlt env dev)"
```

**You, in the project:**

```
$ cd project && direnv allow
direnv: loading .envrc
[Touch ID prompt — or silent if ssh-agent already has the key]
direnv: export +AWS_ACCESS_KEY_ID +DB_PASSWORD +API_KEY +API_TOKEN

$ aws s3 ls         # works — secret is in env, scoped to this dir
$ cd ..             # leaving the dir → direnv unsets everything
```

**Anyone else who clones the repo:**

```
$ cd project && direnv allow
direnv: loading .envrc
kvlt: no decrypt identity available for vault "dev"
direnv: error: command failed
```

The `.age` blobs are right there in the repo. Useless without an SSH
private key matching one of the recipients in
`vaults/local/<id>.yaml`.

### Variant: only export specific keys

```bash
# .envrc
eval "$(kvlt env dev --only AWS_ACCESS_KEY_ID,AWS_SECRET_ACCESS_KEY)"
```

### Variant: prefix every key

```bash
# .envrc
eval "$(kvlt env dev --prefix MY_APP_)"
# → export MY_APP_API_KEY=… MY_APP_DB_PASSWORD=…
```

## 2. `kvlt run` — scoped env injection

Same idea as `aws-vault exec` or `op run`: secrets enter the child
process's environment, never your shell's.

```bash
kvlt run dev -- npm start
kvlt run dev -- python manage.py migrate
kvlt run dev -- bash -c 'curl -H "Authorization: Bearer $API_TOKEN" …'
```

Difference vs. `eval "$(kvlt env dev)"`:

| Approach | Secrets land in… | Persists? | Best for |
|---|---|---|---|
| `eval "$(kvlt env dev)"` | Your interactive shell | Yes (until you `exit`) | dev loops, REPL |
| `kvlt run dev -- cmd`     | Just the child process    | No (gone when cmd exits) | one-off commands, scripts, CI |
| `direnv` + `eval`         | Your shell, *inside the dir* | Yes, scoped to dir | per-project default |

`kvlt run` is the right pattern for anything you wouldn't want
sitting in `env` after the command finishes.

## 3. GitLab CI

Encrypted secrets in the repo, **one** unencrypted CI variable: the
age identity.

**Setup, once, locally:**

```bash
# Generate a separate age-native key for CI (don't reuse your SSH key).
age-keygen -o ci-identity.txt
# AGE-SECRET-KEY-1QQPQQRZ…    ← put this in GitLab CI/CD variables as KVLT_IDENTITY
# Public key: age1xyz…         ← add to your kvlt vault as a recipient

kvlt vault rotate dev --add age1xyz…
git add vaults/ .kvlt/        # commit recipient list + re-encrypted blobs
```

> ⚠️ Treat `ci-identity.txt` like an SSH private key — store the
> contents in GitLab's masked variable `KVLT_IDENTITY`, then `rm` the
> local file (or move it to your password manager).

**`.gitlab-ci.yml`:**

```yaml
deploy:
  image: ghcr.io/retr0h/kvlt:latest    # or curl-install kvlt in a before_script
  variables:
    KVLT_IDENTITY: $KVLT_IDENTITY      # GitLab masked variable
  script:
    - kvlt run dev -- ./deploy.sh
```

`deploy.sh` sees every secret in `dev` as env vars, never logged,
never persisted. You replaced N GitLab variables with 1.

**Adding a teammate:** they push a commit that adds their SSH pubkey
as a recipient. Re-encrypt with `kvlt vault rotate dev --add ssh-ed25519:AAA…`.
Old blobs are decrypted with the existing identity, re-encrypted
including the new recipient. CI continues working with the unchanged
`KVLT_IDENTITY`.

**Removing a teammate:** `kvlt vault rotate dev --remove ssh-ed25519:AAA…`.
The old `.age` blobs would still decrypt with their old key (we can't
unforget what was already encrypted) — you should also rotate the
underlying secrets at the source (cloud provider IAM, etc.). kvlt
makes this rotation cheap; it doesn't make it unnecessary.

## 4. GitHub Actions

Same pattern, GitHub flavor:

```yaml
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - name: Install kvlt
        run: curl -fsSL https://github.com/retr0h/kvlt/raw/main/install.sh | sh
      - name: Deploy
        env:
          KVLT_IDENTITY: ${{ secrets.KVLT_IDENTITY }}
        run: kvlt run dev -- ./deploy.sh
```

## 5. Chaining with Unix tools

`kvlt get` writes raw bytes to stdout — no JSON, no trailing
newline, no decoration. It composes.

```bash
# Pipe a secret straight into a tool that wants stdin
kvlt get dev DEPLOY_KEY | ssh-add -

# Wrap a command that wants a config file with -in-place
kvlt get dev kubeconfig > /tmp/kc && KUBECONFIG=/tmp/kc kubectl get pods
shred -u /tmp/kc

# Use a kvlt secret as a curl Bearer token without persisting it
curl -H "Authorization: Bearer $(kvlt get dev API_TOKEN)" https://api.example.com/me

# Feed multiple secrets through tee for use+log:
kvlt get dev DB_PASSWORD | tee >(pg_restore --password-stdin db.dump) | shasum
```

For scripting, structured output is one flag away:

```bash
kvlt get dev API_KEY --json
# {"vault":"dev","key":"API_KEY","value":"sk-1234"}
```

Exit codes follow shell convention: `0` success, `1` general error,
`2` not found, `3` auth failure. So:

```bash
if ! kvlt get dev OPTIONAL_KEY 2>/dev/null; then
  echo "secret not set; using default"
  OPTIONAL_KEY=fallback
fi
```

## 6. dotfiles + `chezmoi`

If you keep dotfiles in a repo, kvlt slots in for the secret bits:

```bash
# In your dotfiles repo:
kvlt vault create local dotfiles
kvlt put dotfiles GITHUB_TOKEN=ghp_…
kvlt put dotfiles ANTHROPIC_API_KEY=sk-ant-…

# In ~/.bashrc / ~/.zshrc:
export GITHUB_TOKEN="$(kvlt get dotfiles GITHUB_TOKEN 2>/dev/null)"
export ANTHROPIC_API_KEY="$(kvlt get dotfiles ANTHROPIC_API_KEY 2>/dev/null)"
```

Now your dotfiles repo is fully checkable-in. New machine: clone the
repo, `ssh-add ~/.ssh/id_ed25519`, your shell secrets just work.

## 7. Per-project + global vaults

You can run multiple vaults; the names are local to each
repository's `vaults/` directory. Common patterns:

```bash
# In each project: a project-scoped vault
kvlt vault create local dev
kvlt put dev DB_PASSWORD=…

# Globally (in $HOME): a personal vault for cross-cutting tokens
KVLT_REPO=~/.kvlt-personal kvlt vault create local home
KVLT_REPO=~/.kvlt-personal kvlt put home GITHUB_TOKEN=…

# Then in your shell rc, expose the personal one everywhere:
export GITHUB_TOKEN="$(KVLT_REPO=~/.kvlt-personal kvlt get home GITHUB_TOKEN)"
```

## 8. Migrating to a managed backend later

When the time comes:

```bash
kvlt vault create aws-sm prod --region us-east-1
kvlt vault migrate dev --to-type aws-sm    # copy-then-swap, source intact
```

Every `kvlt get prod ...` call site in your code, `.envrc`, CI configs
keeps working — only the vault config file's `type` field changed.
This is the named-vault payoff: backend choice doesn't propagate into
calling code.

## Threat-model reminder

`kvlt`'s local backend protects against:

- ✅ Plaintext secrets in your repo / dotfiles
- ✅ Plaintext secrets in unencrypted backups (the blobs are encrypted)
- ✅ Other users on the same machine reading your vaults (mode `0600`)
- ✅ A stolen laptop, *if* full-disk encryption (FileVault / LUKS) is on
- ✅ A stolen `.age` blob without the matching SSH private key

It does **not** protect against:

- ❌ Malware running as your user (it has the same SSH key access you do)
- ❌ A compromised SSH private key (any tool reusing the key has the same exposure)
- ❌ Secrets that need to be shared with services that can't decrypt age blobs (use `kvlt run` to inject as env vars at the boundary)

When in doubt: `kvlt` is exactly as protective as your SSH private
key is, and no more.
