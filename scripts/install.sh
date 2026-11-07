#!/bin/sh
set -eu

# Install an already downloaded and reviewed terminal artifact into a fresh prefix.
# Fetching release archives and replacing existing installations stay explicit.
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
  printf '%s\n' 'Usage: scripts/install.sh ARTIFACT_DIRECTORY [PREFIX]' \
    'Build with make package, or extract a matching terminal release:' \
    'https://github.com/gongahkia/gator/releases' >&2
  exit 1
fi
if ! command -v bun >/dev/null 2>&1; then
  printf '%s\n' 'Gator requires Bun 1.4.2 or newer on PATH.' >&2
  exit 1
fi
artifact=$(cd "$1" && pwd)
prefix=${2:-"$HOME/.local"}
exec bun "$artifact/ui/scripts/install.mjs" "$artifact" "$prefix"
