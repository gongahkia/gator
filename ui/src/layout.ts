const graphemes = new Intl.Segmenter(undefined, { granularity: "grapheme" });
const wide = /[\p{Extended_Pictographic}\u1100-\u115F\u2E80-\uA4CF\uAC00-\uD7A3\uF900-\uFAFF\uFE10-\uFE6F\uFF01-\uFF60\uFFE0-\uFFE6]/u;

export function panelGeometry(columns: number, rows: number, focused: boolean, picker: boolean, hasSubtitle: boolean) {
  const paddingX = columns < 32 ? 0 : columns < 48 ? 1 : 2;
  const availableWidth = Math.max(1, columns - 2 * paddingX);
  const panelWidth = focused ? Math.min(78, availableWidth) : availableWidth;
  const contentWidth = Math.max(1, panelWidth - (focused ? 4 : 0));
  const compact = rows < 16;
  const showSubtitle = hasSubtitle && !compact;
  const showComposerAnchor = !compact;
  const chrome = 1 + (showSubtitle ? 2 : 0) + (picker ? compact ? 1 : 4 : 0) + 2 + 2 + (showComposerAnchor ? 3 : 0) + (compact ? 0 : 1);
  return { paddingX, panelWidth, contentWidth, compact, showSubtitle, showComposerAnchor, bodyRows: Math.max(1, rows - chrome) };
}

export function wrapTerminalText(value: string, width: number): string[] {
  const limit = Math.max(1, width);
  const lines: string[] = [];
  for (const sourceLine of value.split("\n")) {
    let line = "";
    let cells = 0;
    for (const part of graphemes.segment(sourceLine)) {
      const glyph = part.segment;
      const size = wide.test(glyph) ? 2 : 1;
      if (line && cells + size > limit) { lines.push(line); line = ""; cells = 0; }
      line += glyph; cells += size;
    }
    lines.push(line);
  }
  return lines;
}
