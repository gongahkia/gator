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
  const chrome = (compact ? 0 : 1) + (focused && !compact ? 3 : 0) + 1 + (showSubtitle ? 2 : 0) + (picker ? (compact ? 1 : 4) : 0) + (compact ? 1 : 2) + 1 + 2 + (showComposerAnchor ? 3 : 0);
  return { paddingX, panelWidth, contentWidth, compact, showSubtitle, showComposerAnchor, bodyRows: Math.max(1, rows - chrome) };
}

export function terminalCells(value: string): number {
  return [...graphemes.segment(value)].reduce((cells, part) => cells + (wide.test(part.segment) ? 2 : 1), 0);
}

export function formFieldTail(value: string, width: number): string {
  const text = value.replace(/\s+/g, " ");
  if (terminalCells(text) <= width) return text;
  const parts = [...graphemes.segment(text)].map((part) => part.segment);
  let tail = "", cells = 1;
  for (let i = parts.length - 1; i >= 0; i--) {
    const size = terminalCells(parts[i]);
    if (cells + size > Math.max(1, width)) break;
    tail = parts[i] + tail; cells += size;
  }
  return "…" + tail;
}

type PanelHint = { shortcut: string; action: string };
export function fitPanelHints(hints: PanelHint[], width: number, compact = false): PanelHint[] {
  if (!hints.length) return [];
  const last = hints.find((hint) => hint.shortcut.includes("Esc")) ?? hints[hints.length - 1];
  const cost = (hint: PanelHint) => terminalCells(compact ? hint.shortcut : `${hint.shortcut} to ${hint.action}`);
  if (hints.reduce((sum, hint) => sum + cost(hint), 0) + (hints.length - 1) * 3 <= width) return hints;
  const help = hints.find((hint) => hint.shortcut === "F1") ?? { shortcut: "F1", action: "shortcuts" };
  const selected: PanelHint[] = [];
  const includeHelp = !last.shortcut.includes("F1") && cost(last) + cost(help) + 3 <= width;
  let used = cost(last) + (includeHelp ? cost(help) + 3 : 0);
  for (const hint of hints.filter((hint) => hint !== last && hint.shortcut !== "F1")) {
    const addition = cost(hint) + 3;
    if (used + addition > width) break;
    selected.push(hint); used += addition;
  }
  if (includeHelp) selected.push(help);
  return [...selected, last];
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
