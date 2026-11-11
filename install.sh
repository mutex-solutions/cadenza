#!/bin/sh
# cadenza installer: the agent-side command: downloads the latest release for your OS and architecture.
#
#   curl -fsSL https://raw.githubusercontent.com/mutex-solutions/cadenza/main/install.sh | sh
#
# Env overrides:
#   CADENZA_VERSION=v0.1.0          install a specific tag (default: latest)
#   CADENZA_INSTALL_DIR=~/.local/bin   where to put the binary
#   CADENZA_SKIP_CHECKSUM=1         install without verifying checksums.txt (not recommended)
#
# Everything runs inside main(), called on the last line, so a download cut off
# mid-transfer executes nothing.
set -eu

main() {

REPO="mutex-solutions/cadenza"
BINARY="cadenza"

info()  { printf '  %s\n' "$1"; }
err()   { printf 'error: %s\n' "$1" >&2; exit 1; }

# --- Detect OS ---
os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  linux)  os=linux ;;
  darwin) os=darwin ;;
  *) err "unsupported OS: $os" ;;
esac

# --- Detect arch ---
arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  arm64|aarch64) arch=arm64 ;;
  *) err "unsupported architecture: $arch" ;;
esac

# --- Resolve version ---
version="${CADENZA_VERSION:-}"
if [ -z "$version" ]; then
  info "Finding the latest release…"
  version=$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | grep '"tag_name":' | head -1 | sed -E 's/.*"([^"]+)".*/\1/')
  [ -n "$version" ] || err "could not determine latest version (is a release published?)"
fi

archive="${BINARY}_${os}_${arch}.tar.gz"
url="https://github.com/$REPO/releases/download/$version/$archive"

# --- Install dir ---
install_dir="${CADENZA_INSTALL_DIR:-}"
if [ -z "$install_dir" ]; then
  if [ -w "/usr/local/bin" ] 2>/dev/null; then
    install_dir="/usr/local/bin"
  else
    install_dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$install_dir"

# --- Download + extract ---
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
info "Downloading cadenza $version ($os/$arch)…"
curl -fsSL "$url" -o "$tmp/$archive" || err "download failed: $url"

verify_checksum

tar -xzf "$tmp/$archive" -C "$tmp" || err "extract failed"
[ -f "$tmp/$BINARY" ] || err "binary not found in archive"

install -m 0755 "$tmp/$BINARY" "$install_dir/$BINARY" 2>/dev/null \
  || { mv "$tmp/$BINARY" "$install_dir/$BINARY" && chmod 0755 "$install_dir/$BINARY"; }

info "Installed to $install_dir/$BINARY"

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) printf '\n  Add %s to your PATH:\n    export PATH="%s:$PATH"\n' "$install_dir" "$install_dir" ;;
esac

printf '\n  Next: run "cadenza login <server-url>" to pair this machine.\n'
printf '  Guide: https://github.com/mutex-solutions/cadenza/blob/main/docs/self-hosting.md\n'
}

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | cut -d' ' -f1
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$1" | cut -d' ' -f1
  else
    return 1
  fi
}

verify_checksum() {
  if [ "${CADENZA_SKIP_CHECKSUM:-}" = "1" ]; then
    info "Checksum verification skipped (CADENZA_SKIP_CHECKSUM=1)."
    return 0
  fi
  curl -fsSL "https://github.com/$REPO/releases/download/$version/checksums.txt" -o "$tmp/checksums.txt" \
    || err "could not download checksums.txt (set CADENZA_SKIP_CHECKSUM=1 to install anyway)"
  expected=$(awk -v f="$archive" '$2 == f || $2 == "*" f { print $1; exit }' "$tmp/checksums.txt")
  [ -n "$expected" ] || err "no checksum for $archive in checksums.txt"
  actual=$(sha256_of "$tmp/$archive") || err "need sha256sum or shasum to verify the download (or set CADENZA_SKIP_CHECKSUM=1)"
  [ "$actual" = "$expected" ] || err "checksum mismatch for $archive: expected $expected, got $actual"
  info "Checksum verified."
}

main "$@"
