#!/bin/sh
set -eu

cd "$(dirname "$0")/.."

uv sync --locked --extra dev
make check

check_tmp_dir=$(mktemp -d)
trap 'rm -rf "$check_tmp_dir"' EXIT

uv build --out-dir "$check_tmp_dir/dist"
set -- "$check_tmp_dir"/dist/*.whl
if [ "$#" -ne 1 ] || [ ! -f "$1" ]; then
    echo "expected exactly one built wheel" >&2
    exit 1
fi

uv venv --python 3.12 "$check_tmp_dir/venv"
uv pip install --python "$check_tmp_dir/venv/bin/python" "$1"
test "$("$check_tmp_dir/venv/bin/hcb" --json-schema-version)" = "1"
TERM=dumb NO_COLOR=1 "$check_tmp_dir/venv/bin/hcb" --help >/dev/null

if [ "$(uname -s)" = Linux ]; then
    make calendar-mouse-smoke
fi
