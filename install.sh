#!/bin/sh
# shellcheck shell=sh
set -eu

repo="gongahkia/paw-cli"
tmp_dir=""

log() {
  printf '%s\n' "$*"
}

fail() {
  printf 'error: %s\n' "$*" >&2
  exit 1
}

cleanup() {
  if [ -n "$tmp_dir" ] && [ -d "$tmp_dir" ]; then
    rm -rf "$tmp_dir"
  fi
}

download() {
  url=$1
  out=$2
  if command -v curl >/dev/null 2>&1; then
    curl -fsSL "$url" -o "$out"
  elif command -v wget >/dev/null 2>&1; then
    wget -qO "$out" "$url"
  else
    fail "curl or wget is required"
  fi
}

sha256_file() {
  file=$1
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{ print $1 }'
  elif command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$file" | awk '{ print $1 }'
  else
    fail "sha256sum or shasum is required"
  fi
}

detect_os() {
  case "$(uname -s)" in
    Linux) printf 'linux\n' ;;
    Darwin) printf 'darwin\n' ;;
    *) fail "unsupported OS: $(uname -s)" ;;
  esac
}

detect_arch() {
  case "$(uname -m)" in
    x86_64 | amd64) printf 'amd64\n' ;;
    arm64 | aarch64) printf 'arm64\n' ;;
    *) fail "unsupported architecture: $(uname -m)" ;;
  esac
}

latest_tag() {
  api_file="$tmp_dir/latest.json"
  download "https://api.github.com/repos/$repo/releases/latest" "$api_file"
  tag="$(sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' "$api_file" | head -n 1)"
  if [ -z "$tag" ]; then
    fail "could not determine latest release tag"
  fi
  printf '%s\n' "$tag"
}

choose_install_dir() {
  if [ -n "${PAW_INSTALL_DIR:-}" ]; then
    printf '%s\n' "$PAW_INSTALL_DIR"
    return
  fi
  if [ -d /usr/local/bin ] && [ -w /usr/local/bin ]; then
    printf '%s\n' "/usr/local/bin"
    return
  fi
  if [ -z "${HOME:-}" ]; then
    fail "HOME is unset; set PAW_INSTALL_DIR"
  fi
  printf '%s\n' "$HOME/.local/bin"
}

tmp_dir="$(mktemp -d)"
trap cleanup EXIT HUP INT TERM

os="$(detect_os)"
arch="$(detect_arch)"

if [ -n "${PAW_VERSION:-}" ]; then
  case "$PAW_VERSION" in
    v*) tag="$PAW_VERSION" ;;
    *) tag="v$PAW_VERSION" ;;
  esac
else
  tag="$(latest_tag)"
fi
version="${tag#v}"
archive="paw_${version}_${os}_${arch}.tar.gz"
base_url="https://github.com/$repo/releases/download/$tag"

log "installing paw $version for $os/$arch"
download "$base_url/checksums.txt" "$tmp_dir/checksums.txt"
download "$base_url/$archive" "$tmp_dir/$archive"

expected="$(awk -v file="$archive" '$2 == file { print $1 }' "$tmp_dir/checksums.txt")"
if [ -z "$expected" ]; then
  fail "checksum for $archive not found"
fi
actual="$(sha256_file "$tmp_dir/$archive")"
if [ "$expected" != "$actual" ]; then
  fail "checksum mismatch for $archive"
fi

mkdir -p "$tmp_dir/extract"
tar -xzf "$tmp_dir/$archive" -C "$tmp_dir/extract"
if [ ! -f "$tmp_dir/extract/paw" ]; then
  fail "archive did not contain paw binary"
fi

install_dir="$(choose_install_dir)"
mkdir -p "$install_dir"
if [ ! -w "$install_dir" ]; then
  fail "install directory is not writable: $install_dir"
fi
cp "$tmp_dir/extract/paw" "$install_dir/paw"
chmod 0755 "$install_dir/paw"

log "installed: $install_dir/paw"
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) log "add to PATH: export PATH=\"$install_dir:\$PATH\"" ;;
esac
log "next: paw doctor models"
