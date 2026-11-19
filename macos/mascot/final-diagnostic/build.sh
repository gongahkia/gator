#!/bin/bash
set -euo pipefail
diagnostic_root="$(cd "$(dirname "$0")" && pwd)"
diagnostic_app="${GATOR_DIAGNOSTIC_APP:-/tmp/Gator-Mascot-P04M-A2-Final-Diagnostic.app}"
if [[ -e "$diagnostic_app" ]]; then echo 'Fresh diagnostic bundle destination required.' >&2; exit 1; fi
mkdir -p "$diagnostic_app/Contents/MacOS" "$diagnostic_app/Contents/Resources/Assets" "$diagnostic_app/Contents/Resources/SourceRevision"
swiftc -parse-as-library -swift-version 6 -strict-concurrency=complete -warnings-as-errors -O \
  -target "$(uname -m)-apple-macosx15.0" \
  "$diagnostic_root/../validation/MascotRig.swift" \
  "$diagnostic_root/../gate2/"{Metrics,Engine,NativeMesh,Presentation}.swift \
  "$diagnostic_root/DiagnosticLab.swift" -o "$diagnostic_app/Contents/MacOS/GatorDiagnostic"
cp "$diagnostic_root/../gate2/Assets/B-native-meshes.json" "$diagnostic_root/../validation/Assets/Gator-B.usdz" "$diagnostic_app/Contents/Resources/Assets/"
for diagnostic_source in "$diagnostic_root"/*.swift "$diagnostic_root"/build.sh "$diagnostic_root"/../gate2/{Metrics,Engine,NativeMesh,Presentation}.swift "$diagnostic_root"/../validation/MascotRig.swift; do
  cp "$diagnostic_source" "$diagnostic_app/Contents/Resources/SourceRevision/"
done
python3 - "$diagnostic_app" <<'PY'
from pathlib import Path
import hashlib,json,subprocess,sys
p=Path(sys.argv[1]);digest=lambda x:hashlib.sha256(x.read_bytes()).hexdigest()
result={'head':subprocess.check_output(['git','rev-parse','HEAD'],text=True).strip(),
        'flags':'-O -swift-version 6 -strict-concurrency=complete -warnings-as-errors -target arm64-apple-macosx15.0',
        'sources':{x.name:digest(x) for x in (p/'Contents/Resources/SourceRevision').iterdir()},
        'assets':{x.name:digest(x) for x in (p/'Contents/Resources/Assets').iterdir()},
        'executableSHA256':digest(p/'Contents/MacOS/GatorDiagnostic')}
(p/'Contents/Resources/revision.json').write_text(json.dumps(result,indent=2)+'\n')
PY
cat > "$diagnostic_app/Contents/Info.plist" <<'PLIST'
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0"><dict>
<key>CFBundleIdentifier</key><string>dev.gator.mascot.final.diagnostic</string>
<key>CFBundleName</key><string>Gator Final Diagnostic</string>
<key>CFBundleExecutable</key><string>GatorDiagnostic</string>
<key>CFBundlePackageType</key><string>APPL</string>
<key>LSMinimumSystemVersion</key><string>15.0</string>
<key>NSPrincipalClass</key><string>NSApplication</string>
</dict></plist>
PLIST
plutil -lint "$diagnostic_app/Contents/Info.plist"
