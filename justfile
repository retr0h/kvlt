set allow-duplicate-variables

# Optional modules: import? allows `just fetch` to work before .just/remote/ exists.

import? '.just/remote/go.just'
import? '.just/remote/md.just'
import? '.just/remote/just.just'

# No documentation site, so md formats every markdown file in the repository.

md_site_dir := ""

# Coverage measured 64.6% on Go 1.27.3, which counts a multi-line statement once
# where 1.27.1 counted each line (65.0% then). Declaring the real floor keeps the
# gate on and stops regressions.

go_coverage_target := "64"

# --- Fetch ---

# List the recipes
default:
    @just --list

# Fetch shared justfiles from osapi-justfiles
fetch:
    mkdir -p .just/remote
    curl -sSfL https://raw.githubusercontent.com/osapi-io/osapi-justfiles/refs/heads/main/go/go.just -o .just/remote/go.just
    curl -sSfL https://raw.githubusercontent.com/osapi-io/osapi-justfiles/refs/heads/main/md/md.just -o .just/remote/md.just
    curl -sSfL https://raw.githubusercontent.com/osapi-io/osapi-justfiles/refs/heads/main/just/just.just -o .just/remote/just.just

# --- Top-level orchestration ---

# Install all dependencies
deps:
    just go-deps
    just go-mod

# Run all tests
test:
    just just-fmt-check
    just go-test

# Format, lint before committing
ready:
    just just-fmt
    just md-fmt
    just go-fmt
    just go-vet
