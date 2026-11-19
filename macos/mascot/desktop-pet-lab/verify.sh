#!/bin/bash
set -euo pipefail
pet_root="$(cd "$(dirname "$0")" && pwd)"
macos_root="$(cd "$pet_root/../.." && pwd)"
pet_check_dir="$(mktemp -d /tmp/gator-pet-b1-check.XXXXXX)"
pet_flags=(-parse-as-library -swift-version 6 -strict-concurrency=complete -warnings-as-errors -O -target "$(uname -m)-apple-macosx15.0")
pet_runtime=("$macos_root/Sources/GatorMac/NativeMascotModel.swift" "$macos_root/Sources/GatorMac/NativeMascotPlayback.swift")
swiftc "${pet_flags[@]}" "${pet_runtime[0]}" "$pet_root/Geometry.swift" "$pet_root/CheckLogic.swift" -o "$pet_check_dir/logic"
"$pet_check_dir/logic"
swiftc "${pet_flags[@]}" "${pet_runtime[@]}" "$macos_root/mascot/2d/lab/CheckPlayback.swift" -o "$pet_check_dir/playback"
"$pet_check_dir/playback" "$macos_root/mascot/2d/assets"
swiftc "${pet_flags[@]}" "${pet_runtime[@]}" "$pet_root/Geometry.swift" "$pet_root/Screens.swift" "$pet_root/Panel.swift" "$pet_root/Latency.swift" -o "$pet_check_dir/latency"
for pet_tool in Control Pointer Capture; do
 swiftc "${pet_flags[@]}" "$pet_root/tools/$pet_tool.swift" -o "$pet_check_dir/$pet_tool"
done
swiftc "${pet_flags[@]}" "${pet_runtime[0]}" "$pet_root/Geometry.swift" "$pet_root/Screens.swift" "$pet_root/FocusFixture.swift" -o "$pet_check_dir/FocusFixture"
mkdir -p "$pet_check_dir/FocusFixture.app/Contents/MacOS"
cp "$pet_check_dir/FocusFixture" "$pet_check_dir/FocusFixture.app/Contents/MacOS/FocusFixture"
cat > "$pet_check_dir/FocusFixture.app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?><plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>dev.gator.pet.focus-fixture</string>
<key>CFBundleExecutable</key><string>FocusFixture</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>15.0</string>
<key>NSPrincipalClass</key><string>NSApplication</string>
</dict></plist>
PLIST
plutil -lint "$pet_check_dir/FocusFixture.app/Contents/Info.plist"
echo "Verified headless checks and compiled native tools: $pet_check_dir"
# latency and window-system probes require a controlled desktop; compilation does not launch them.
