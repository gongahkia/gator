#!/bin/bash
set -euo pipefail
mascot_root="$(cd "$(dirname "$0")/../.." && pwd)"
mascot_export="${GATOR_MASCOT_EXPORT:-$(mktemp -d /tmp/gator-mascot-export.XXXXXX)}"
mkdir -p "$mascot_export"
swiftc -parse-as-library -swift-version 6 -strict-concurrency=complete -warnings-as-errors -O -target "$(uname -m)-apple-macosx15.0" \
 "$mascot_root/validation/MascotRig.swift" "$mascot_root/gate2/Metrics.swift" "$mascot_root/gate2/Engine.swift" \
 "$mascot_root/2d/source/AnimationRecipe.swift" "$mascot_root/2d/source/ClassicGator.swift" "$mascot_root/2d/source/Export.swift" -o "$mascot_export/export"
"$mascot_export/export" "$mascot_export/assets" "$mascot_root/validation/Assets/Gator-B.usdz"
echo "Exported to $mascot_export/assets; compare before replacing the checked-in authoring output."
