#!/bin/bash
set -euo pipefail
native_root="$(cd "$(dirname "$0")/.." && pwd)"
repo_root="$(cd "$native_root/.." && pwd)"
native_scratch="${GATOR_NATIVE_BUILD_DIR:-/tmp/gator-native-build}"
native_destination="${GATOR_NATIVE_APP_PATH:-/tmp/Gator-Native.app}"
native_stage="$(mktemp -d /tmp/gator-native-app.XXXXXX)"
trap 'rm -rf "$native_stage"' EXIT
swift build --package-path "$native_root" --scratch-path "$native_scratch"
native_bin="$(swift build --package-path "$native_root" --scratch-path "$native_scratch" --show-bin-path)"
mkdir -p "$native_stage/Gator.app/Contents/MacOS" "$native_stage/Gator.app/Contents/Resources"
cp "$native_bin/GatorMac" "$native_stage/Gator.app/Contents/MacOS/GatorMac"
cp "$native_root/Resources/Info.plist" "$native_stage/Gator.app/Contents/Info.plist"
(cd "$repo_root" && go build -trimpath -o "$native_stage/Gator.app/Contents/Resources/gator-runtime" ./cmd/gator)
chmod 755 "$native_stage/Gator.app/Contents/MacOS/GatorMac" "$native_stage/Gator.app/Contents/Resources/gator-runtime"
plutil -lint "$native_stage/Gator.app/Contents/Info.plist"
if [ -e "$native_destination" ]; then
  echo "Destination already exists: $native_destination. Choose a fresh GATOR_NATIVE_APP_PATH." >&2
  exit 1
fi
mv "$native_stage/Gator.app" "$native_destination"
echo "Built $native_destination"
echo "Launch: open '$native_destination'"
