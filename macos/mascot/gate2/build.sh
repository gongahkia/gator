#!/bin/bash
set -euo pipefail
gate2_root="$(cd "$(dirname "$0")" && pwd)"
gate2_app="${GATOR_GATE2_APP:-/tmp/Gator-Mascot-P04M-A2-Gate2.app}"
if [[ -e "$gate2_app" ]]; then echo "Choose a fresh GATOR_GATE2_APP; destination exists." >&2; exit 1; fi
mkdir -p "$gate2_app/Contents/MacOS" "$gate2_app/Contents/Resources"
swiftc -parse-as-library -swift-version 6 -strict-concurrency=complete -warnings-as-errors -O \
  -target "$(uname -m)-apple-macosx15.0" \
  "$gate2_root/../validation/MascotRig.swift" "$gate2_root/../validation/RealtimeRenderer.swift" \
  "$gate2_root/Metrics.swift" "$gate2_root/Engine.swift" "$gate2_root/NativeMesh.swift" "$gate2_root/Presentation.swift" "$gate2_root/Gate2Lab.swift" \
  -o "$gate2_app/Contents/MacOS/Gate2Lab"
cp -R "$gate2_root/../validation/Assets" "$gate2_app/Contents/Resources/"
cp "$gate2_root/Assets/B-native-meshes.json" "$gate2_app/Contents/Resources/Assets/"
mkdir -p "$gate2_app/Contents/Resources/SourceRevision"
for gate2_source in "$gate2_root"/*.swift "$gate2_root"/build.sh "$gate2_root"/bake-mesh.py "$gate2_root"/../validation/MascotRig.swift "$gate2_root"/../validation/RealtimeRenderer.swift; do
  cp "$gate2_source" "$gate2_app/Contents/Resources/SourceRevision/"
done
python3 - "$gate2_app" <<'PY'
from pathlib import Path
import hashlib,json,subprocess,sys
root=Path(sys.argv[1]); sources=root/'Contents/Resources/SourceRevision'
digest=lambda p:hashlib.sha256(p.read_bytes()).hexdigest()
record={'head':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),'swiftFlags':'-O -swift-version 6 -strict-concurrency=complete -warnings-as-errors -target arm64-apple-macosx15.0','sourceFiles':{p.name:digest(p)for p in sources.iterdir()},'binarySHA256':digest(root/'Contents/MacOS/Gate2Lab'),'USDZSHA256':digest(root/'Contents/Resources/Assets/Gator-B.usdz'),'meshCacheSHA256':digest(root/'Contents/Resources/Assets/B-native-meshes.json')}
(root/'Contents/Resources/revision.json').write_text(json.dumps(record,indent=2))
PY
cat > "$gate2_app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>dev.gator.mascot.gate2.validation</string>
<key>CFBundleName</key><string>Gator Gate 2 Lab</string>
<key>CFBundleExecutable</key><string>Gate2Lab</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>15.0</string>
<key>NSPrincipalClass</key><string>NSApplication</string>
</dict></plist>
PLIST
plutil -lint "$gate2_app/Contents/Info.plist"
