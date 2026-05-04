# Contributing to kvlt

First off, thanks for taking the time to contribute!

## How Can I Contribute?

### Reporting Bugs

- Use the [GitHub issue tracker](https://github.com/retr0h/kvlt/issues) to
  report bugs
- Include your OS, Go version, and the kvlt version (`kvlt version`)
- Include steps to reproduce the issue
- Note which backend (local_encryption, aws-sm, …) is in use

### Suggesting Features

- Open an issue describing the feature you'd like to see
- Explain why this feature would be useful
- Consider whether it fits the project's scope (a small, pluggable secrets
  vault — not a full secret-management platform)

### Code Contributions

#### Small Fixes

Small changes like typos, grammar fixes, and formatting can be submitted
directly as a pull request.

#### Larger Changes

For bug fixes, new features, or significant changes:

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make your changes
4. Ensure the project builds: `go build -o kvlt .`
5. Run the linter: `just go::vet`
6. Commit using [Conventional Commits](https://conventionalcommits.org/) format
7. Push to your fork and open a pull request

### Commit Messages

This project uses [Conventional Commits](https://conventionalcommits.org/).
Format: `type(scope): description`

Types: `feat`, `fix`, `docs`, `chore`, `ci`, `build`, `test`, `refactor`

Examples:

```
feat: add 1Password backend
fix: handle missing key file on first run
docs: clarify migrate copy-then-swap semantics
```

## Development Setup

### Prerequisites

- Go 1.25+
- [just](https://github.com/casey/just) — command runner
- [golangci-lint](https://golangci-lint.run/) — Go linter

### Building

```bash
git clone https://github.com/retr0h/kvlt.git
cd kvlt
go build -o kvlt .
```

### Testing

```bash
just test        # fmt-check + unit tests with race detector
just go::unit    # unit tests only
```

## Code Style

- Follow existing patterns in the codebase
- Multi-line function signatures
- Backend implementations live under `internal/kvlt/backend_*.go`, behind
  build tags when they pull in cloud-vendor SDKs — the base binary stays
  dependency-light
- Errors should name the vault and key (when safe to expose) so an operator
  can act without diffing logs
