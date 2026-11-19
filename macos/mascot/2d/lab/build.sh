#!/bin/bash
set -euo pipefail
mascot_root="$(cd "$(dirname "$0")/.." && pwd)"
mascot_app="${GATOR_PLAYBACK_APP:-/tmp/Gator-Mascot-2D-Playback.app}"
if [[ -e "$mascot_app" ]]; then echo 'Choose a fresh playback app destination.' >&2; exit 1; fi
mkdir -p "$mascot_app/Contents/MacOS" "$mascot_app/Contents/Resources"
mascot_runtime="$mascot_root/../../Sources/GatorMac"
swiftc -parse-as-library -swift-version 6 -strict-concurrency=complete -warnings-as-errors -O -target "$(uname -m)-apple-macosx15.0" \
  "$mascot_runtime/NativeMascotModel.swift" "$mascot_runtime/NativeMascotPlayback.swift" "$mascot_root/lab/PlaybackLab.swift" -o "$mascot_app/Contents/MacOS/MascotPlayback"
cp -R "$mascot_root/assets" "$mascot_app/Contents/Resources/Mascot"
cat > "$mascot_app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>dev.gator.mascot.playback.lab</string>
<key>CFBundleName</key><string>Gator 2D Playback</string>
<key>CFBundleExecutable</key><string>MascotPlayback</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>15.0</string>
<key>NSPrincipalClass</key><string>NSApplication</string>
</dict></plist>
PLIST
plutil -lint "$mascot_app/Contents/Info.plist"
