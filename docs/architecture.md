# kvlt architecture

`kvlt` is a small, pluggable secrets vault. This document covers the
named-vault model, on-disk layout, provider interface, built-in backends,
and operational primitives (migration, redaction, error handling).
Implementation details live next to the code; this file is the conceptual map.

## Architecture

The vault system is built around a **named-vault** architecture where:

- **Named vaults**: Each vault instance has a user-defined name configured in
  `vaults/{vault-type}/{id}.yaml`.
- **Vault types**: The underlying storage system (`local_encryption`, plus
  optional `sops`, `aws-sm`, `azure-kv`, `1password`, `hashicorp-vault` behind
  build tags) is specified per vault. The CLI accepts `local` as a typo-friendly
  alias for `local_encryption`.
- **Clean interface**: All vaults implement a common `Provider` interface for
  consistent access patterns (see below).
- **CLI-only surface**: every command opens the vault, talks to the backend, and
  exits. There is no daemon, no listening port, no persistent state between
  invocations. Other Go programs can also import the `Provider` directly and
  skip the CLI entirely.

Callers reference vaults by **name**, never by backend type. This keeps a
project's call sites stable as backends change underneath them — the same
`kvlt get prod API_KEY` works whether `prod` is a local AES-GCM file today and
AWS Secrets Manager tomorrow.

## Secret Storage

Vault configurations are tracked in git under `vaults/`, organized by type:

```
vaults/
  {vault-type}/
    {vault-id}.yaml             # vault configuration (tracked in git)
```

Encrypted local secrets live under the runtime datastore directory (default
`.kvlt/secrets/`), organized by type and name:

```
.kvlt/secrets/
  local_encryption/
    {vault-name}/
      {secret-key}.age          # age-encrypted blobs (one per secret)
```

There is **no key file on disk**. Decryption uses the user's SSH private key
(typically in `~/.ssh/`, optionally protected by a passphrase, optionally cached
in ssh-agent). kvlt never writes a master key — that's age's role, and the
recipient/identity model keeps the secret material in the user's existing
protection chain.

The `.kvlt/` directory is `.gitignore`d at project init for safety, though the
contents are encrypted: an attacker reading a checked-in `.age` blob still needs
one of the recipient SSH private keys to do anything with it. Cloud-backend
vaults store nothing under `.kvlt/secrets/` — secrets live in the upstream
service.

## Provider Interface

Every backend implements the `Provider` interface in `pkg/kvlt/provider.go`:

```go
type Provider interface {
    // Get retrieves a secret by key. Returns an error if the key is
    // missing — backends do not silently return "" for unknown keys,
    // since callers can't distinguish that from an empty value.
    Get(ctx context.Context, key string) (string, error)

    // Put stores a secret. Overwrites without warning — versioning, if
    // any, is the backend's concern.
    Put(ctx context.Context, key, value string) error

    // List returns every key currently stored in this vault. Values are
    // never returned, even in debug logs.
    List(ctx context.Context) ([]string, error)

    // Name returns the vault's user-defined name (not the backend type).
    // Used in error messages and audit lines.
    Name() string
}
```

The interface is intentionally minimal — anything fancier (rotation, lease,
audit) is layered on top by callers, not pushed into the backend contract.
Four methods is the contract; everything else is layered on top.

## Vault Configuration

Each named vault is described by a YAML file:

```yaml
# vaults/local_encryption/8f4e2d1c9a3b4c5dae7f0a1b2c3d4e5f.yaml
id: 8f4e2d1c9a3b4c5dae7f0a1b2c3d4e5f
name: dev
type: local_encryption
settings:
  recipients:
    - ssh-ed25519 AAAAC3NzaC1lZDI1NTE5… john@laptop
    # add more recipients to share decrypt access — each line is one
    # SSH (or age-native `age1…`) public key. Anyone with the
    # matching private key can decrypt every secret in this vault.
```

```yaml
# vaults/aws-sm/2b6c1f0e-7d8a-4e3b-9f2c-0a1b2c3d4e5f.yaml
id: 2b6c1f0e-7d8a-4e3b-9f2c-0a1b2c3d4e5f
name: prod
type: aws-sm
settings:
  region: us-east-1
  secret_prefix: myapp/ # optional
  # auth uses the default AWS credential chain (IAM role, env, profile)
```

**Rules:**

- The `id` is auto-assigned on `kvlt vault create` and **never** edited by hand.
- `name` is the user-facing reference used in CLI verbs and Go API calls.
- `type` selects the backend; changing it requires `kvlt vault migrate`, not a
  hand-edit.
- `settings` is backend-specific; each backend's loader validates the schema.

## Built-in Backends

### `local` (default; on-disk type identifier `local_encryption`)

The default backend. CLI alias is `local`; the on-disk type identifier in vault
config files is `local_encryption`.

- **Encryption**: [age](https://github.com/FiloSottile/age) (X25519 +
  ChaCha20-Poly1305). kvlt does not own the cipher primitives — every
  encrypt/decrypt operation goes through `filippo.io/age`. The on-disk blob is a
  valid age container readable by `age -d` / `rage -d` directly with one of the
  recipient identities.
- **Recipients**: one or more SSH or age public keys, listed in
  `settings.recipients` in the vault config YAML. `kvlt vault create` defaults
  to `~/.ssh/id_ed25519.pub`; `--public-key` / `-p` (repeatable) adds more.
- **Identities**: the user's SSH private key (`~/.ssh/id_ed25519` etc.).
  Discovered by `DefaultIdentityResolver`, which loads the conventional
  ed25519/ecdsa/rsa keys in priority order. Passphrase-protected keys prompt on
  `/dev/tty` with echo off — same UX as `ssh-keygen -y -f`. ssh-agent
  integration is planned (file-based passphrase prompt works today).
- **Storage**: each secret stored as a complete age container in
  `.kvlt/secrets/local_encryption/{vault-name}/{key}.age` with mode `0600`.
  Atomic writes (tempfile + chmod + fsync + rename) so a half-written file can
  never exist on disk.
- **List**: walks the directory, returns basenames sans `.age`. No decryption
  involved — listing keys does not invoke the IdentityResolver, so it never
  prompts for a passphrase.
- **Multi-recipient**: encrypt to N SSH pubkeys; any one of those private keys
  can decrypt. The team-sharing escape hatch.
- **Rotation**: `kvlt vault migrate` to a new vault is the supported path;
  in-place key rotation is intentionally not supported (it complicates the
  copy-then-swap safety model below).

### `aws-sm` (build tag: `aws`)

AWS Secrets Manager via `aws-sdk-go-v2`.

- **Auth**: standard AWS credential chain — IAM role > env vars > profile > SSO.
  No credentials in YAML or git.
- **Region**: required (no implicit default).
- **Secret prefix**: optional; prepended to every key for namespacing inside a
  shared AWS account.
- **Errors**: missing secrets, auth failures, and rate-limits each map to a
  distinct error type so callers can retry intelligently.

### `azure-kv` (build tag: `azure`)

Azure Key Vault via `azure-sdk-for-go`. Auth via `DefaultAzureCredential`
(managed identity > env > Azure CLI).

### `1password` (build tag: `onepass`)

Shells out to the `op` CLI. Auth options follow `op`'s own model — service
account token (CI), desktop-app integration (local dev), or Connect server.
Secret keys map to `op://<vault>/<item>/<field>` URIs; a bare key resolves to
the `password` field by default.

### `hashicorp-vault` (build tag: `hcv`)

HashiCorp Vault or OpenBao via the HTTP API. Configurable mount path and KV
version (v1 vs v2).

## Vault Migration

`kvlt vault migrate <name> --to-type <target> [--dry-run]` migrates a vault to a
different backend in place. The vault **name** is preserved, so all existing
references continue to resolve identically.

### How it works

1. List every secret key in the source vault.
2. Copy each secret value from the current backend to a new provider instance
   configured for the target type.
3. Write a new vault config file pointing to the target backend.
4. Delete the old config file.

### Safety model

- **Secrets are copied, not moved.** The source backend retains its secrets
  until the operator explicitly cleans up. If anything fails mid-copy, the
  original vault remains fully functional.
- **Save-new-then-delete-old config swap.** The new config file is written
  before the old one is removed. If the delete step fails, you end up with an
  orphaned config but the vault works correctly on the new backend.
- **Same-type migrations are rejected.** The target type must differ from the
  current type — there's no in-place rotation today.
- **Dry-run support.** `--dry-run` prints the secret count and type change
  without copying anything.

The copy-then-swap shape gives us the safety property: any partial failure
leaves the source intact; the user has not lost data.

## Error Handling

All errors include:

- The **vault name** (the user-facing identifier — not the UUID).
- The **operation** (`get`, `put`, `list`, `migrate`).
- The **key** when it's safe to expose (`get` of a missing key includes the key
  name; auth failures suppress it).
- A **suggested resolution** when one exists ("run
  `kvlt vault create --name dev`", "check `aws sts get-caller-identity`", …).

Categories the CLI distinguishes:

- **Configuration errors** — invalid vault name, missing config, malformed
  backend settings.
- **Authentication errors** — credential chain exhausted, expired token.
- **Network/transport errors** — backend unreachable, timeout. Retry-safe.
- **Not-found** — distinct from auth failures so callers can make the
  vault-key-vs-permission distinction.
- **Validation** — duplicate vault name, illegal characters in key.

## Security Considerations

### Credential management

- No backend credentials in vault config files or in git.
- Cloud backends use the platform's native credential chain (IAM, managed
  identity, `op` CLI, …).
- Rotate credentials at the platform level; kvlt re-reads them per-call.

### Secret access patterns

- Keys names are not values — but they're not secret either. Use descriptive but
  not revealing names.
- The local backend's `.key` file is `0600` and gitignored. Treat it like an SSH
  private key.

### Logging

- Secret values are **never logged**, even at `--debug`. The `Get` path does not
  pass values through `slog.Debug` calls.
- `List` only returns key names; values are never serialized in list responses.
- Migration logs include vault name + key count, never key contents.

## Extensibility

To add a new backend:

1. Add `pkg/kvlt/backend_<type>.go` behind a Go build tag matching the variant
   goreleaser will produce (`//go:build <tag>`).
2. Expose a `New<Type>Provider(...)` constructor returning the concrete type
   (e.g. `*AWSProvider`); the type implements the `Provider` interface.
3. Register the backend in the type registry so `kvlt vault create <type>`
   recognizes it.
4. Add a config-validation function for the YAML `settings` block.
5. Document configuration in this file under "Built-in Backends".
6. Add a goreleaser variant build entry if the backend pulls in cloud-vendor
   SDKs — the base binary should never grow new runtime deps.

The interface is small enough that a plain-text-file backend, a Redis backend,
or an SSM Parameter Store backend each take well under 200 lines.
