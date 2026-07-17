#!/bin/sh
set -eu

mode=${1:-test}
script_dir=$(CDPATH= cd "$(dirname "$0")" && pwd)
root_dir=$(CDPATH= cd "$script_dir/.." && pwd)
test_root=${GATOR_TEST_ROOT:-"$root_dir/.gator-test"}

mkdir -p "$test_root/cache" "$test_root/config" "$test_root/data" "$test_root/state"
cd "$root_dir"

case "$mode" in
	test)
		exec env \
			GATOR_TEST_ROOT="$test_root" \
			TMPDIR="$test_root" \
			XDG_CACHE_HOME="$test_root/cache" \
			XDG_CONFIG_HOME="$test_root/config" \
			XDG_DATA_HOME="$test_root/data" \
			XDG_STATE_HOME="$test_root/state" \
			NVIM_LOG_FILE="$test_root/nvim-test.log" \
			nvim --headless --noplugin -i NONE -u tests/minimal_init.lua -c "lua dofile('tests/run.lua')"
		;;
	lint)
		exec env \
			GATOR_TEST_ROOT="$test_root" \
			TMPDIR="$test_root" \
			XDG_CACHE_HOME="$test_root/cache" \
			XDG_CONFIG_HOME="$test_root/config" \
			XDG_DATA_HOME="$test_root/data" \
			XDG_STATE_HOME="$test_root/state" \
			NVIM_LOG_FILE="$test_root/nvim-lint.log" \
			nvim --headless --noplugin -i NONE -u NONE -c "lua dofile('tests/lint.lua')"
		;;
	indexer)
		exec env \
			GATOR_TEST_ROOT="$test_root" \
			TMPDIR="$test_root" \
			XDG_CACHE_HOME="$test_root/cache" \
			XDG_CONFIG_HOME="$test_root/config" \
			XDG_DATA_HOME="$test_root/data" \
			XDG_STATE_HOME="$test_root/state" \
			cargo test --manifest-path crates/gator-index/Cargo.toml
		;;
	*)
		echo "usage: $0 [test|lint|indexer]" >&2
		exit 64
		;;
esac
