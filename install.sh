#!/bin/sh
#
# kvlt installer
# Usage: curl -fsSL https://github.com/retr0h/kvlt/raw/main/install.sh | sh
#
# Env overrides:
#   KVLT_VERSION       install a specific version (e.g. 1.1.1) instead of latest
#   KVLT_INSTALL_DIR   force install destination, skipping the default rules

set -eu

err() {
    printf 'kvlt: %s\n' "$1" >&2
    exit 1
}

have() {
    command -v "$1" >/dev/null 2>&1
}

http_get() {
    if have curl; then
        curl -fsSL "$1"
    elif have wget; then
        wget -qO- "$1"
    else
        err "neither curl nor wget found on PATH"
    fi
}

detect_os() {
    raw=$(uname -s)
    case "$raw" in
        Darwin)  os=darwin ;;
        Linux)   os=linux ;;
        *)       err "unsupported OS: $raw — build from source: https://github.com/retr0h/kvlt#-build-from-source" ;;
    esac
}

detect_arch() {
    machine=$(uname -m)
    case "$machine" in
        arm64|aarch64) arch=arm64 ;;
        x86_64|amd64)  arch=amd64 ;;
        *)             err "unsupported architecture: $machine" ;;
    esac
}

resolve_version() {
    if [ -n "${KVLT_VERSION:-}" ]; then
        version=${KVLT_VERSION#v}
        return
    fi
    tag=$(http_get https://api.github.com/repos/retr0h/kvlt/releases/latest \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' \
        | head -n1)
    if [ -z "$tag" ]; then
        err "could not determine latest version from GitHub API"
    fi
    version=${tag#v}
}

path_contains() {
    case ":$PATH:" in
        *":$1:"*) return 0 ;;
        *)        return 1 ;;
    esac
}

resolve_install_dir() {
    needs_symlink=0
    if [ -n "${KVLT_INSTALL_DIR:-}" ]; then
        install_dir=$KVLT_INSTALL_DIR
        return
    fi
    if [ "$(id -u)" = "0" ]; then
        install_dir=/usr/local/bin
        return
    fi
    if path_contains "$HOME/.local/bin"; then
        install_dir=$HOME/.local/bin
        return
    fi
    if path_contains "$HOME/bin"; then
        install_dir=$HOME/bin
        return
    fi
    install_dir=$HOME/.kvlt/bin
    needs_symlink=1
}

setup_tmp() {
    tmp=$(mktemp -d 2>/dev/null || mktemp -d -t kvlt-install)
    trap 'rm -rf "$tmp"' EXIT
}

download() {
    base=https://github.com/retr0h/kvlt/releases/download/v${version}
    asset=kvlt_${version}_${os}_${arch}

    if have curl; then
        curl -fsSL -o "$tmp/kvlt" "$base/$asset" \
            || err "failed to download $base/$asset"
        curl -fsSL -o "$tmp/checksums.txt" "$base/checksums.txt" \
            || err "failed to download $base/checksums.txt"
    else
        wget -q -O "$tmp/kvlt" "$base/$asset" \
            || err "failed to download $base/$asset"
        wget -q -O "$tmp/checksums.txt" "$base/checksums.txt" \
            || err "failed to download $base/checksums.txt"
    fi
}

verify_checksum() {
    asset=kvlt_${version}_${os}_${arch}
    expected=$(grep " $asset\$" "$tmp/checksums.txt" | awk '{print $1}')
    if [ -z "$expected" ]; then
        err "no checksum entry for $asset in checksums.txt"
    fi
    if have shasum; then
        actual=$(shasum -a 256 "$tmp/kvlt" | awk '{print $1}')
    elif have sha256sum; then
        actual=$(sha256sum "$tmp/kvlt" | awk '{print $1}')
    else
        err "neither shasum nor sha256sum found on PATH"
    fi
    if [ "$expected" != "$actual" ]; then
        printf 'kvlt: checksum mismatch for %s\n  expected: %s\n  actual:   %s\n' \
            "$asset" "$expected" "$actual" >&2
        exit 1
    fi
}

strip_quarantine() {
    [ "$os" = "darwin" ] || return 0
    xattr -d com.apple.quarantine "$tmp/kvlt" 2>/dev/null || true
}

install_binary() {
    mkdir -p "$install_dir" || err "cannot create $install_dir"
    install -m 755 "$tmp/kvlt" "$install_dir/kvlt" \
        || err "cannot write to $install_dir/kvlt"
}

maybe_symlink() {
    [ "$needs_symlink" = "1" ] || return 0
    if [ -w /usr/local/bin ]; then
        ln -sf "$install_dir/kvlt" /usr/local/bin/kvlt 2>/dev/null || true
    fi
}

print_summary() {
    printf 'kvlt v%s installed to %s/kvlt\n' "$version" "$install_dir"
    if ! path_contains "$install_dir"; then
        printf '\nAdd this to your shell rc:\n  export PATH="%s:$PATH"\n' "$install_dir"
    fi
}

main() {
    detect_os
    detect_arch
    resolve_version
    resolve_install_dir
    setup_tmp
    download
    verify_checksum
    strip_quarantine
    install_binary
    maybe_symlink
    print_summary
}

main "$@"
