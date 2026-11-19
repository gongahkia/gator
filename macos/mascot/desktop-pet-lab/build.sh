#!/bin/bash
set -euo pipefail
pet_root="$(cd "$(dirname "$0")" && pwd)"
macos_root="$(cd "$pet_root/../.." && pwd)"
pet_app="${GATOR_PET_LAB_APP:-/tmp/Gator-Mascot-P04M-B1-Desktop-Pet-Lab.app}"
if [[ -e "$pet_app" ]]; then echo 'Choose a fresh isolated app path.' >&2; exit 1; fi
mkdir -p "$pet_app/Contents/MacOS" "$pet_app/Contents/Resources"
swiftc -parse-as-library -swift-version 6 -strict-concurrency=complete -warnings-as-errors -O -target "$(uname -m)-apple-macosx15.0" \
 "$macos_root/Sources/GatorMac/NativeMascotModel.swift" "$macos_root/Sources/GatorMac/NativeMascotPlayback.swift" \
 "$pet_root/Geometry.swift" "$pet_root/Screens.swift" "$pet_root/Panel.swift" "$pet_root/Lab.swift" -o "$pet_app/Contents/MacOS/PetLab"
cp -R "$macos_root/mascot/2d/assets" "$pet_app/Contents/Resources/Mascot"
cp /tmp/Gator-Native-P04M-2D.app/Contents/Resources/GatorMenuBar.png "$pet_app/Contents/Resources/"
cat > "$pet_app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?><!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>dev.gator.pet.b1.lab</string>
<key>CFBundleName</key><string>Gator Pet Lab</string>
<key>CFBundleExecutable</key><string>PetLab</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>15.0</string>
<key>NSPrincipalClass</key><string>GatorPetLabApplication</string>
</dict></plist>
PLIST
plutil -lint "$pet_app/Contents/Info.plist"
echo "Built $pet_app"
