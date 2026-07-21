#!/bin/sh
set -eu

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
wsl=false
for probe in /proc/sys/kernel/osrelease /proc/version; do
  if [ -r "$probe" ] && grep -qi microsoft "$probe" && grep -qi wsl2 "$probe"; then
    wsl=true
    break
  fi
done
if [ "$wsl" != true ]; then
  echo "wsl-smoke: must run inside WSL2" >&2
  exit 1
fi
if [ "$(go env GOOS)" != linux ]; then
  echo "wsl-smoke: Go must target Linux" >&2
  exit 1
fi
for tool in git go make rg ctags; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "wsl-smoke: missing required tool: $tool" >&2
    exit 1
  fi
done

cd "$root"
make test
make build
tmp=$(mktemp -d "${TMPDIR:-/tmp}/paw-wsl-smoke.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
XDG_CONFIG_HOME="$tmp/config" ./bin/paw gather --instruction "validate WSL2 runtime" > "$tmp/gather.json"
test -s "$tmp/gather.json"
