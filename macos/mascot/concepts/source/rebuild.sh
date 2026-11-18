#!/bin/bash
# Isolated design assets only. Does not build/launch Gator or install anything.
set -euo pipefail
p04m_root="$(cd "$(dirname "$0")/.." && pwd)"
p04m_tools="${TMPDIR:-/tmp}/gator-mascot-a1-tools"
p04m_arch="$(uname -m)"
mkdir -p "$p04m_tools"
p04m_flags=(-parse-as-library -swift-version 6 -target "${p04m_arch}-apple-macosx15.0")
swiftc "${p04m_flags[@]}" "$p04m_root/source/MascotLab.swift" -o "$p04m_tools/render"
"$p04m_tools/render" "$p04m_root"
swiftc "${p04m_flags[@]}" "$p04m_root/source/DesignBoards.swift" -o "$p04m_tools/boards"
"$p04m_tools/boards" "$p04m_root"
for p04m_id in A B C; do
    /usr/bin/usdcat "$p04m_root/models/Gator-$p04m_id.usda" -o "$p04m_root/models/Gator-$p04m_id.usdc"
    /usr/bin/usdzip -a "$p04m_root/models/Gator-$p04m_id.usdc" "$p04m_root/models/Gator-$p04m_id.usdz"
    /usr/bin/usdchecker "$p04m_root/models/Gator-$p04m_id.usdz" > "$p04m_root/evidence/usd-$p04m_id.log" 2>&1
done
swiftc "${p04m_flags[@]}" "$p04m_root/source/RealityKitProbe.swift" -o "$p04m_tools/reality-probe"
"$p04m_tools/reality-probe" "$p04m_root/models" > "$p04m_root/evidence/realitykit-import.log" 2>&1
swiftc "${p04m_flags[@]}" -D MASCOT_PREVIEW "$p04m_root/source/MascotLab.swift" "$p04m_root/source/Preview.swift" -o "$p04m_tools/ConceptPreview"
swiftc "${p04m_flags[@]}" -O -D MASCOT_BENCHMARK "$p04m_root/source/MascotLab.swift" "$p04m_root/source/Measure.swift" -o "$p04m_tools/measure"
"$p04m_tools/measure" "$p04m_root/evidence/performance.json"
swiftc "${p04m_flags[@]}" "$p04m_root/source/Validate.swift" -o "$p04m_tools/validate"
"$p04m_tools/validate" "$p04m_root"
ffmpeg -loglevel error -y -framerate 20 -i "$p04m_root/prototype/frames/%03d.png" -filter_complex '[0:v]split[a][b];[a]palettegen=reserve_transparent=1[p];[b][p]paletteuse=alpha_threshold=128' -loop 0 "$p04m_root/prototype/B-motion.gif"
ffmpeg -loglevel error -y -f lavfi -i 'color=c=0xF4F6F2:s=256x256:r=20:d=4' -framerate 20 -i "$p04m_root/prototype/frames/%03d.png" -filter_complex '[0:v][1:v]overlay=shortest=1:format=auto,format=yuv420p' -c:v libx264 -movflags +faststart "$p04m_root/prototype/B-motion.mp4"
printf 'Design-only preview compiled (not launched): %s\n' "$p04m_tools/ConceptPreview"
