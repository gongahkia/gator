export type DraftLink = { start: number; end: number; raw: string; label: string; url: string };
export function safeWebLink(value: string) {
  try { const url = new URL(value); return ["http:", "https:"].includes(url.protocol) && !url.username && !url.password && !/[\x00-\x20]/.test(value); } catch { return false; }
}
export function draftLinks(text: string): DraftLink[] {
  const links: DraftLink[] = [];
  for (const match of text.matchAll(/\[([^\]\n]{1,500})\]\((https?:\/\/[^\s<>"()]+)\)|https?:\/\/[^\s<>"`]+/g)) {
    let raw = match[0], url = match[2] ?? raw;
    if (!match[2]) { url = url.replace(/[.,;!?]+$/, ""); while (url.endsWith(")") && (url.match(/\)/g)?.length ?? 0) > (url.match(/\(/g)?.length ?? 0)) url = url.slice(0, -1); raw = url; }
    if (safeWebLink(url)) links.push({ start: match.index, end: match.index + raw.length, raw, label: match[1] ?? url, url });
    if (links.length === 64) break;
  }
  return links;
}
export function markdownLink(label: string, url: string) {
  if (!safeWebLink(url) || !label.trim() || /[\r\n]/.test(label) || label.length > 500) throw new Error("Enter a label and an http or https link without embedded credentials.");
  return label === url ? url : `[${label.replace(/[\\\[\]]/g, "\\$&")}](${url.replace(/\(/g, "%28").replace(/\)/g, "%29")})`;
}
