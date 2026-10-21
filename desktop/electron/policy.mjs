import { resolve, sep } from "node:path";

export const APP_URL = "gator://app/index.html";

export function assetPath(url, root) {
  const parsed = new URL(url);
  if (parsed.protocol !== "gator:" || parsed.hostname !== "app" || parsed.username || parsed.password || parsed.port) return null;
  let path;
  try { path = decodeURIComponent(parsed.pathname); } catch { return null; }
  if (path.includes("\0") || path.includes("\\")) return null;
  const absolute = resolve(root, `.${path}`);
  return absolute.startsWith(`${resolve(root)}${sep}`) ? absolute : null;
}

export function externalURL(value) {
  if (typeof value !== "string" || value.length > 8192) return null;
  try {
    const url = new URL(value);
    return ["https:", "http:"].includes(url.protocol) && !url.username && !url.password ? url.href : null;
  } catch { return null; }
}

export function validateFrame(frame) {
  if (!frame || typeof frame !== "object" || Array.isArray(frame)) throw new Error("Invalid desktop action");
  // Process launch, executable paths, environment, and exit acknowledgements
  // never come from the renderer. External handoffs use a separate host method.
  const allowed = {
    submit: ["type", "text"], navigate: ["type", "text"], key: ["type", "key", "text"],
    panel: ["type", "key", "panel"], approval: ["type", "interaction_id", "approved"],
  }[frame.type];
  if (!allowed || Object.keys(frame).some((key) => !allowed.includes(key))) throw new Error("Unsupported desktop action");
  if (JSON.stringify(frame).length > 256 * 1024) throw new Error("Desktop action is too large");
  if (frame.type === "submit" && (typeof frame.text !== "string" || frame.text.length > 128 * 1024)) throw new Error("Invalid request");
  if (frame.type === "navigate" && (typeof frame.text !== "string" || frame.text.length > 4096 || !/^\/[a-z][a-z0-9-]*(?: [a-z][a-z0-9-]*)*$/.test(frame.text) || ["/approve", "/deny", "/quit", "/exit"].includes(frame.text))) throw new Error("Invalid destination");
  if (frame.type === "key" && !["esc", "ctrl+o", "ctrl+p", "ctrl+x", "ctrl+b"].includes(frame.key)) throw new Error("Unsupported shortcut");
  if (frame.type === "approval" && (!Number.isSafeInteger(frame.interaction_id) || frame.interaction_id < 1 || typeof frame.approved !== "boolean")) throw new Error("Invalid approval");
  if (frame.type === "panel") {
    const panel = frame.panel;
    if (!panel || typeof panel !== "object" || Array.isArray(panel) || !/^[a-f0-9]{64}$/.test(panel.id)) throw new Error("Invalid view");
    if (Object.keys(panel).some((key) => !["id", "action", "row", "fields", "text"].includes(key))) throw new Error("Invalid view action");
    if (!["select", "activate", "key", "query"].includes(panel.action)) throw new Error("Invalid view action");
    if (panel.row !== undefined && (!Number.isSafeInteger(panel.row) || panel.row < 0 || panel.row > 10000)) throw new Error("Invalid row");
    if (panel.action === "key" && (typeof frame.key !== "string" || !/^(enter|esc|up|down|left|right|tab|shift\+tab|space|ctrl\+[ru]|f1|[a-z0-9])$/.test(frame.key))) throw new Error("Invalid view key");
    if (panel.action === "query" && (typeof panel.text !== "string" || panel.text.length > 4096)) throw new Error("Invalid search");
    if (panel.fields !== undefined && (!panel.fields || Array.isArray(panel.fields) || typeof panel.fields !== "object" || Object.entries(panel.fields).some(([key, value]) => !/^\d{1,5}$/.test(key) || typeof value !== "string" || value.length > 64 * 1024))) throw new Error("Invalid form");
  }
  return { ...frame, version: 1 };
}

export function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}
