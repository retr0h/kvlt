#!/usr/bin/env bash
#
# kvlt installer
# Usage: curl -fsSL https://github.com/retr0h/kvlt/raw/main/install.sh | bash
#
# Env overrides:
#   KVLT_VERSION       install a specific version (e.g. 1.1.1) instead of latest
#   KVLT_INSTALL_DIR   force install destination, skipping the default rules

set -euo pipefail
APP=kvlt

# Visual style matches kvlt's `ash` runtime theme so curl|bash and
# the installed binary share a palette. ACCENT is the dusty-brick
# (xterm 131, #af5f5f); named by role, not hue, so a future palette
# swap is one line.
MUTED='\033[0;2m'
RED='\033[0;31m'
ACCENT='\033[38;5;131m'
NC='\033[0m' # reset

err() {
    printf "${RED}kvlt: %s${NC}\n" "$1" >&2
    exit 1
}

print_message() {
    local level=$1
    local message=$2
    local color=""
    case $level in
        info)    color="${NC}" ;;
        warning) color="${ACCENT}" ;;
        error)   color="${RED}" ;;
    esac
    printf "${color}${message}${NC}\n"
}

have() {
    command -v "$1" >/dev/null 2>&1
}

# unbuffered_sed picks the right -u/-l/no-buffer flag for the local
# sed; required so the progress reader sees curl trace lines as they
# arrive rather than after the download completes.
unbuffered_sed() {
    if echo | sed -u -e "" >/dev/null 2>&1; then
        sed -nu "$@"
    elif echo | sed -l -e "" >/dev/null 2>&1; then
        sed -nl "$@"
    else
        local pad
        pad="$(printf "\n%512s" "")"
        sed -ne "s/$/\\${pad}/" "$@"
    fi
}

print_progress() {
    local bytes="$1"
    local length="$2"
    [ "$length" -gt 0 ] || return 0

    local width=50
    local percent=$(( bytes * 100 / length ))
    [ "$percent" -gt 100 ] && percent=100
    local on=$(( percent * width / 100 ))
    local off=$(( width - on ))

    local filled
    filled=$(printf "%*s" "$on" "")
    filled=${filled// /■}
    local empty
    empty=$(printf "%*s" "$off" "")
    empty=${empty// /･}

    printf "\r${ACCENT}%s%s %3d%%${NC}" "$filled" "$empty" "$percent" >&4
}

# download_with_progress reads curl --trace-ascii output to drive a
# block-character progress bar. Falls back to plain curl/wget when:
#   - stderr is not a TTY (CI, piped to file)
#   - curl is unavailable (we use wget without progress)
#   - the trace plumbing fails for any reason
download_with_progress() {
    local url="$1"
    local output="$2"

    if [ -t 2 ]; then
        exec 4>&2
    else
        exec 4>/dev/null
    fi

    local tmp_dir=${TMPDIR:-/tmp}
    local basename="${tmp_dir}/kvlt_install_$$"
    local tracefile="${basename}.trace"

    rm -f "$tracefile"
    mkfifo "$tracefile"

    # Hide cursor while the bar animates.
    printf "\033[?25l" >&4
    trap "trap - RETURN; rm -f \"$tracefile\"; printf '\033[?25h' >&4; exec 4>&-" RETURN

    (
        curl --trace-ascii "$tracefile" -fsSL -o "$output" "$url"
    ) &
    local curl_pid=$!

    unbuffered_sed \
        -e 'y/ACDEGHLNORTV/acdeghlnortv/' \
        -e '/^0000: content-length:/p' \
        -e '/^<= recv data/p' \
        "$tracefile" | \
    {
        local length=0
        local bytes=0

        while IFS=" " read -r -a line; do
            [ "${#line[@]}" -lt 2 ] && continue
            local tag="${line[0]} ${line[1]}"

            if [ "$tag" = "0000: content-length:" ]; then
                length="${line[2]}"
                length=$(echo "$length" | tr -d '\r')
                bytes=0
            elif [ "$tag" = "<= recv" ]; then
                local size="${line[3]}"
                bytes=$(( bytes + size ))
                if [ "$length" -gt 0 ]; then
                    print_progress "$bytes" "$length"
                fi
            fi
        done
    }

    wait $curl_pid
    local ret=$?
    echo "" >&4
    return $ret
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

# fetch downloads $url to $output, using the styled progress bar when
# possible and degrading gracefully otherwise. Either curl or wget is
# acceptable; the progress UI only fires for the curl + TTY path.
fetch() {
    local url="$1"
    local output="$2"
    if have curl && [ -t 2 ]; then
        download_with_progress "$url" "$output" || curl -fsSL -o "$output" "$url"
    elif have curl; then
        curl -fsSL -o "$output" "$url"
    elif have wget; then
        wget -q -O "$output" "$url"
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

    print_message info "\n${MUTED}Installing ${NC}kvlt ${MUTED}version: ${NC}$version"
    fetch "$base/$asset" "$tmp/kvlt" \
        || err "failed to download $base/$asset"
    fetch "$base/checksums.txt" "$tmp/checksums.txt" \
        || err "failed to download $base/checksums.txt"
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
        printf "${RED}kvlt: checksum mismatch for %s${NC}\n  expected: %s\n  actual:   %s\n" \
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
    printf "\n"
    printf "${MUTED}█▄▀ █░█ █░░ ▀█▀${NC}   ${MUTED}installed to${NC} ${ACCENT}%s/kvlt${NC}\n" "$install_dir"
    printf "${MUTED}█░█ ▀▄▀ █▄▄ ░█░${NC}   ${MUTED}version${NC} ${NC}%s${NC}\n" "$version"
    printf "\n"
    if ! path_contains "$install_dir"; then
        print_message warning "Add this to your shell rc:"
        printf "  ${NC}export PATH=\"%s:\$PATH\"${NC}\n\n" "$install_dir"
    fi
    printf "${MUTED}Bootstrap a vault:${NC}\n"
    printf "  kvlt vault create --name dev   ${MUTED}# encrypts to ~/.ssh/id_ed25519.pub${NC}\n"
    printf "\n"
    printf "${MUTED}Docs:${NC} https://github.com/retr0h/kvlt\n"
    printf "\n"
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
