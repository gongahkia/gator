#!/bin/bash
# isolated validation bundle only; no Gator production build or launch
set -euo pipefail
mascot_validation_root="$(cd "$(dirname "$0")" && pwd)"
mascot_validation_app="${P04M_VALIDATION_APP:-/tmp/Gator-Mascot-A2-Lab.app}"
if [[ -e "$mascot_validation_app" ]]; then
  echo "Choose a fresh P04M_VALIDATION_APP; destination exists." >&2
  exit 1
fi
mkdir -p "$mascot_validation_app/Contents/MacOS" "$mascot_validation_app/Contents/Resources"
swiftc -parse-as-library -swift-version 6 -strict-concurrency=complete -warnings-as-errors -O \
  -target "$(uname -m)-apple-macosx15.0" \
  "$mascot_validation_root/MascotRig.swift" "$mascot_validation_root/RealtimeRenderer.swift" "$mascot_validation_root/RendererLab.swift" \
  -o "$mascot_validation_app/Contents/MacOS/MascotValidation"
cp -R "$mascot_validation_root/Assets" "$mascot_validation_app/Contents/Resources/"
cat > "$mascot_validation_app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>dev.gator.mascot.a2.validation</string>
<key>CFBundleName</key><string>Gator Mascot Validation</string>
<key>CFBundleExecutable</key><string>MascotValidation</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>15.0</string>
<key>NSPrincipalClass</key><string>NSApplication</string>
</dict></plist>
PLIST
plutil -lint "$mascot_validation_app/Contents/Info.plist"
echo "$mascot_validation_app"
