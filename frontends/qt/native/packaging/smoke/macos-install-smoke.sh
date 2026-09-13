#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <install-root>" >&2
  exit 2
fi

executable="$1/Hot Cross Buns.app/Contents/MacOS/Hot Cross Buns"
if [[ ! -x "$executable" ]]; then
  echo "missing installed macOS executable: $executable" >&2
  exit 1
fi

HCB_BENCHMARK_EXIT_AFTER_LOAD=1 QT_QPA_PLATFORM=offscreen "$executable"
