#!/bin/sh
set -eu

repository="gongahkia/gator"
install_root="${GATOR_INSTALL_DIR:-$HOME/.local/bin}"

case "$(uname -s)" in
  Darwin) goos="darwin" ;;
  Linux) goos="linux" ;;
  *) printf '%s\n' "Gator supports macOS and Linux through this installer." >&2; exit 1 ;;
esac

case "$(uname -m)" in
  arm64|aarch64) goarch="arm64" ;;
  x86_64|amd64) goarch="amd64" ;;
  *) printf '%s\n' "Unsupported architecture: $(uname -m)" >&2; exit 1 ;;
esac

archive="gator_${goos}_${goarch}.tar.gz"
base="https://github.com/${repository}/releases/latest/download"
temporary="$(mktemp -d "${TMPDIR:-/tmp}/gator-install.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT HUP INT TERM

curl --fail --location --silent --show-error "$base/$archive" -o "$temporary/$archive"
curl --fail --location --silent --show-error "$base/checksums.txt" -o "$temporary/checksums.txt"

expected="$(awk -v name="$archive" '$2 == name || $2 == "*" name { print $1; exit }' "$temporary/checksums.txt")"
if [ -z "$expected" ]; then
  printf '%s\n' "Release checksums do not include $archive." >&2
  exit 1
fi
if command -v shasum >/dev/null 2>&1; then
  actual="$(shasum -a 256 "$temporary/$archive" | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$temporary/$archive" | awk '{print $1}')"
else
  printf '%s\n' "Install requires shasum or sha256sum to verify the release." >&2
  exit 1
fi
if [ "$expected" != "$actual" ]; then
  printf '%s\n' "Release checksum did not match; Gator was not installed." >&2
  exit 1
fi

tar -xzf "$temporary/$archive" -C "$temporary"
binary="$(find "$temporary" -type f -name gator -perm -u+x -print -quit)"
if [ -z "$binary" ]; then
  printf '%s\n' "Release archive does not contain an executable Gator binary." >&2
  exit 1
fi
mkdir -p "$install_root"
install -m 0755 "$binary" "$install_root/gator"
printf 'Installed Gator to %s/gator\n' "$install_root"
case ":$PATH:" in
  *":$install_root:"*) ;;
  *) printf 'Add %s to PATH, then run: gator\n' "$install_root" ;;
esac
