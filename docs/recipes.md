# kvlt recipes

Concrete patterns for using `kvlt` with the rest of your toolchain. Every
example assumes you've already created a vault and added some secrets:

```bash
kvlt vault create --name dev                                       # SSH-key recipient by default
kvlt secret put --vault dev --key API_KEY --value sk-1234
kvlt secret put --vault dev --key DB_PASSWORD --value hunter2
kvlt secret put --vault dev --key AWS_ACCESS_KEY_ID --value AKIA…
echo "$LONG_TOKEN" | kvlt secret put --vault dev --key API_TOKEN   # stdin → no shell history
kvlt secret put --vault dev --key STRIPE_KEY                       # interactive prompt, echo off
kvlt secret import --vault dev --env ~/.env                        # bulk-import a dotenv file
kvlt secret import --vault dev --file ~/kc.yaml --key kubeconfig   # one whole file → one secret
```

The on-disk artifacts (`.kvlt/secrets/local_encryption/dev/*.age`) are valid age
containers —
`age -d -i ~/.ssh/id_ed25519 .kvlt/secrets/local_encryption/dev/API_KEY.age`
also works. `kvlt` is a convenience layer, not a format lock-in.

## 1. `direnv` — encrypted `.envrc`

The killer use case: project-level env vars, encrypted at rest, only your SSH
identity decrypts.

**`.envrc`** (committed to git, contains zero secrets):

```bash
# .envrc
eval "$(kvlt env --vault dev)"
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

The `.age` blobs are right there in the repo. Useless without an SSH private key
matching one of the public keys in `.kvlt/vaults/local_encryption/<id>.yaml`.

### Variant: only export specific keys

```bash
# .envrc
eval "$(kvlt env --vault dev --only AWS_ACCESS_KEY_ID,AWS_SECRET_ACCESS_KEY)"
```

### Variant: prefix every key

```bash
# .envrc
eval "$(kvlt env --vault dev --prefix MY_APP_)"
# → export MY_APP_API_KEY=… MY_APP_DB_PASSWORD=…
```

### Variant: use a non-default SSH key

```bash
# .envrc — point kvlt at a project-specific key
eval "$(kvlt env --vault dev -i ~/work/id_ed25519)"
# or set it once for the whole shell
export KVLT_PRIVATE_KEY=~/work/id_ed25519
```

## 2. `kvlt run` — scoped env injection

Same idea as `aws-vault exec` or `op run`: secrets enter the child process's
environment, never your shell's.

```bash
kvlt run --vault dev -- npm start
kvlt run --vault dev -- python manage.py migrate
kvlt run --vault dev -- bash -c 'curl -H "Authorization: Bearer $API_TOKEN" …'
```

Difference vs. `eval "$(kvlt env --vault dev)"`:

| Approach                         | Secrets land in…             | Persists?                | Best for                      |
| -------------------------------- | ---------------------------- | ------------------------ | ----------------------------- |
| `eval "$(kvlt env --vault dev)"` | Your interactive shell       | Yes (until you `exit`)   | dev loops, REPL               |
| `kvlt run --vault dev -- cmd`    | Just the child process       | No (gone when cmd exits) | one-off commands, scripts, CI |
| `direnv` + `eval`                | Your shell, _inside the dir_ | Yes, scoped to dir       | per-project default           |

`kvlt run` is the right pattern for anything you wouldn't want sitting in `env`
after the command finishes.

## 3. Sharing a vault with teammates

The whole point of putting encrypted blobs in git is that anyone with a matching
SSH private key can decrypt them — without ever exchanging private keys. This
section walks through adding teammates, removing them, and the rotation nuance.

### Initial setup — encrypt to multiple recipients

When you create the vault, pass each teammate's **public** key as a recipient.
Public keys are designed to be shared (you'd put them on GitHub); private keys
never leave the owner's machine.

```bash
# Pull pubkeys however you like:
#   - GitHub:   curl https://github.com/bob.keys > team-pubkeys/bob.pub
#   - Sent direct: bob pastes ~/.ssh/id_ed25519.pub in Slack
#   - LDAP / 1Password / a shared drive — public keys are not secret

kvlt vault create --name dev \
  -p ~/.ssh/id_ed25519.pub \      # you
  -p team-pubkeys/bob.pub \       # bob
  -p team-pubkeys/carol.pub       # carol

kvlt secret put --vault dev --key API_KEY --value sk-1234
git add .kvlt/vaults/ .kvlt/secrets/ && git commit -m "init dev vault" && git push
```

The vault YAML now lists three recipients; each `.age` blob has three header
stanzas (one per recipient), and any one of those private keys decrypts the
file. Bob clones the repo and runs `kvlt secret get` — kvlt reads
`~/.ssh/id_ed25519` on Bob's machine, finds Bob's matching stanza in the file
header, decrypts. You never need Bob's private key. Bob never needs yours.

### Inside the .age file (just so the model is concrete)

Every kvlt-encrypted file is a standard age container:

```
age-encryption.org/v1
-> ssh-ed25519 [your-fingerprint]
   [file key wrapped to your X25519]
-> ssh-ed25519 [bob-fingerprint]
   [file key wrapped to bob's X25519]
-> ssh-ed25519 [carol-fingerprint]
   [file key wrapped to carol's X25519]
--- [authenticated header tag]
[ciphertext body — one body, three ways in]
```

The body is encrypted once with a per-file random "file key"; each header stanza
wraps that file key to one recipient's pubkey. Decrypting requires the private
key of _one_ of the listed recipients — age tries each identity in order and
uses the first that matches.

### Adding a new teammate

Today (until `vault migrate` lands as a first-class command), the path is:
recreate the vault with the expanded recipient list, then re-`secret put` each
secret. The reason it's not a one-liner: kvlt doesn't currently store an
encryption identity that could re-wrap existing blobs without first decrypting
them.

```bash
# Carol joins; you add her pubkey alongside yours and Bob's
mv .kvlt/vaults/local_encryption/<old-id>.yaml /tmp/  # backup
kvlt vault create --name dev \
  -p ~/.ssh/id_ed25519.pub \
  -p team-pubkeys/bob.pub \
  -p team-pubkeys/carol.pub          # NEW

# Re-import the secrets (you already have them decryptable locally,
# so this round-trip is silent — just shells out values back through Put)
for k in $(kvlt secret list --vault dev 2>/dev/null); do
  v=$(kvlt secret get --vault dev --key "$k")
  echo -n "$v" | kvlt secret put --vault dev --key "$k"
done
git add .kvlt/vaults/ .kvlt/secrets/ && git commit && git push
```

When `vault migrate` ships, this collapses to a single command.

### Removing a teammate

```bash
# Recreate without Bob's pubkey and re-encrypt — same loop as above
```

The painful nuance: **old `.age` blobs still exist in git history, and Bob's
private key still decrypts them.** You can't unforget what was already
encrypted. Two implications:

1. After removing someone, **rotate the underlying secrets** (rotate the API key
   with the cloud provider, expire the database password, regenerate the OAuth
   token). The vault rotation only stops _new_ secrets from being readable by
   them.
2. Force-pushing to delete history doesn't help — anyone who pulled before the
   deletion still has the blobs locally.

This is true of every "encrypted files in git" system (SOPS, git-crypt, age
itself). It's not a kvlt limitation; it's a property of cryptography meeting
version control.

### CI is a "teammate" too

Generate a dedicated SSH keypair for CI, add its pubkey as a vault recipient,
store the private key as a masked CI variable. CI is now a normal recipient —
its decrypts go through the same multi-recipient code path as a human's.

```bash
# Generate a CI-only key
ssh-keygen -t ed25519 -N "" -f ci-identity -C kvlt-ci

# Add its pubkey as a vault recipient (regenerate the vault as above)
# Put the private key contents in your CI's masked-variable store as
# KVLT_PRIVATE_KEY_CONTENTS (then `rm ci-identity` locally).
```

In CI, write the masked variable to a file and point kvlt at it:

```yaml
# .gitlab-ci.yml or equivalent
script:
  - echo "$KVLT_PRIVATE_KEY_CONTENTS" > /tmp/ci-key && chmod 600 /tmp/ci-key
  - kvlt run --vault dev --private-key /tmp/ci-key -- ./deploy.sh
```

CI decrypts with its key; you decrypt with yours; teammate decrypts with theirs.
Nobody shares a private key with anybody.

## 4. GitLab CI

Encrypted secrets in the repo, **one** unencrypted CI variable: the SSH private
key.

**Setup, once, locally:**

```bash
# Generate a separate ed25519 key for CI (don't reuse your personal SSH key).
ssh-keygen -t ed25519 -N "" -f ci-identity -C kvlt-ci
# ci-identity      ← put this file's contents in GitLab CI/CD as KVLT_PRIVATE_KEY
# ci-identity.pub  ← add to your kvlt vault as a public-key recipient

kvlt vault create --name dev -p ci-identity.pub -p ~/.ssh/id_ed25519.pub
git add .kvlt/        # commit recipient list + encrypted blobs
```

> ⚠️ Treat `ci-identity` like any other SSH private key — store its contents in
> GitLab's masked variable `KVLT_PRIVATE_KEY`, then `rm` the local file (or move
> it to your password manager).

**`.gitlab-ci.yml`:**

```yaml
deploy:
  image: ghcr.io/retr0h/kvlt:latest # or curl-install kvlt in a before_script
  variables:
    KVLT_PRIVATE_KEY: $KVLT_PRIVATE_KEY # GitLab masked variable (file path or contents)
  script:
    - kvlt run --vault dev -- ./deploy.sh
```

`deploy.sh` sees every secret in `dev` as env vars, never logged, never
persisted. You replaced N GitLab variables with 1.

**Adding a teammate:** they push a commit that adds their SSH pubkey as a
recipient. Once `vault rotate` lands (planned),
`kvlt vault rotate --name dev --add ssh-ed25519:AAA…` will re-encrypt; until
then, recreate the vault with the expanded recipient list. CI continues working
with the unchanged `KVLT_PRIVATE_KEY`.

**Removing a teammate:** rotation removes them from new encryptions; old `.age`
blobs would still decrypt with their old key (we can't unforget what was already
encrypted) — also rotate the underlying secrets at the source (cloud provider
IAM, etc.). kvlt makes this rotation cheap; it doesn't make it unnecessary.

## 5. GitHub Actions

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
          KVLT_PRIVATE_KEY: ${{ secrets.KVLT_PRIVATE_KEY }}
        run: kvlt run --vault dev -- ./deploy.sh
```

## 6. Chaining with Unix tools

`kvlt secret get` writes raw bytes to stdout — no JSON, no trailing newline, no
decoration. It composes.

```bash
# Pipe a secret straight into a tool that wants stdin
kvlt secret get --vault dev --key DEPLOY_KEY | ssh-add -

# Wrap a command that wants a config file
kvlt secret get --vault dev --key kubeconfig > /tmp/kc && KUBECONFIG=/tmp/kc kubectl get pods
shred -u /tmp/kc

# Use a kvlt secret as a curl Bearer token without persisting it
curl -H "Authorization: Bearer $(kvlt secret get --vault dev --key API_TOKEN)" https://api.example.com/me

# Feed multiple secrets through tee for use+log:
kvlt secret get --vault dev --key DB_PASSWORD | tee >(pg_restore --password-stdin db.dump) | shasum
```

For scripting, structured output is one flag away:

```bash
kvlt secret get --vault dev --key API_KEY --json
# {"vault":"dev","key":"API_KEY","value":"sk-1234"}
```

Exit codes follow shell convention: `0` success, `1` general error, `2` not
found, `3` auth failure. So:

```bash
if ! kvlt secret get --vault dev --key OPTIONAL_KEY 2>/dev/null; then
  echo "secret not set; using default"
  OPTIONAL_KEY=fallback
fi
```

## 7. dotfiles + `chezmoi`

If you keep dotfiles in a repo, kvlt slots in for the secret bits:

```bash
# In your dotfiles repo:
kvlt vault create --name dotfiles
kvlt secret put --vault dotfiles --key GITHUB_TOKEN --value ghp_…
kvlt secret put --vault dotfiles --key ANTHROPIC_API_KEY --value sk-ant-…

# In ~/.bashrc / ~/.zshrc:
export GITHUB_TOKEN="$(kvlt secret get --vault dotfiles --key GITHUB_TOKEN 2>/dev/null)"
export ANTHROPIC_API_KEY="$(kvlt secret get --vault dotfiles --key ANTHROPIC_API_KEY 2>/dev/null)"
```

Now your dotfiles repo is fully checkable-in. New machine: clone the repo,
`ssh-add ~/.ssh/id_ed25519`, your shell secrets just work.

## 8. Per-project + global vaults

You can run multiple vaults; the names are local to each repository's
`.kvlt/vaults/` directory. Common patterns:

```bash
# In each project: a project-scoped vault
kvlt vault create --name dev
kvlt secret put --vault dev --key DB_PASSWORD --value …

# Globally (in $HOME): a personal vault for cross-cutting tokens
KVLT_REPO_PATH=~/.kvlt-personal kvlt vault create --name home
KVLT_REPO_PATH=~/.kvlt-personal kvlt secret put --vault home --key GITHUB_TOKEN --value …

# Then in your shell rc, expose the personal one everywhere:
export GITHUB_TOKEN="$(KVLT_REPO_PATH=~/.kvlt-personal kvlt secret get --vault home --key GITHUB_TOKEN)"
```

## 9. Migrating to a managed backend later

When the time comes (planned):

```bash
kvlt vault create --type aws-sm --name prod --region us-east-1
kvlt vault migrate --name dev --to-type aws-sm    # copy-then-swap, source intact
```

Every `kvlt secret get --vault prod …` call site in your code, `.envrc`, CI
configs keeps working — only the vault config file's `type` field changed. This
is the named-vault payoff: backend choice doesn't propagate into calling code.

## Threat-model reminder

`kvlt`'s local backend protects against:

- ✅ Plaintext secrets in your repo / dotfiles
- ✅ Plaintext secrets in unencrypted backups (the blobs are encrypted)
- ✅ Other users on the same machine reading your vaults (mode `0600`)
- ✅ A stolen laptop, _if_ full-disk encryption (FileVault / LUKS) is on
- ✅ A stolen `.age` blob without the matching SSH private key

It does **not** protect against:

- ❌ Malware running as your user (it has the same SSH key access you do)
- ❌ A compromised SSH private key (any tool reusing the key has the same
  exposure)
- ❌ Secrets that need to be shared with services that can't decrypt age blobs
  (use `kvlt run` to inject as env vars at the boundary)

When in doubt: `kvlt` is exactly as protective as your SSH private key is, and
no more.
