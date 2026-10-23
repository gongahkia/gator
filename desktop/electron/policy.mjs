import { resolve, sep } from "node:path";
import { validUploadID, uploadLimits } from "./uploads.mjs";

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
    panel: ["type", "key", "panel"], approval: ["type", "interaction_id", "approval_token", "approved"],
    queue: ["type", "queue"],
    composer: ["type", "composer"],
    conversation: ["type", "conversation"],
    upload_submit: ["type", "upload"],
    update: ["type", "update"],
    memory: ["type", "memory"],
    receipt: ["type", "receipt"],
  }[frame.type];
  if (!allowed || Object.keys(frame).some((key) => !allowed.includes(key))) throw new Error("Unsupported desktop action");
  if (JSON.stringify(frame).length > 256 * 1024) throw new Error("Desktop action is too large");
  if (frame.type === "submit" && (typeof frame.text !== "string" || frame.text.length > 128 * 1024)) throw new Error("Invalid request");
  if (frame.type === "upload_submit") {
    const value = frame.upload;
    if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).some((key) => !["request_id", "context", "text", "files", "reply_to_update_id"].includes(key)) || (value.reply_to_update_id !== undefined && (typeof value.reply_to_update_id !== "string" || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/.test(value.reply_to_update_id))) || typeof value.request_id !== "string" || !/^[a-zA-Z0-9-]{1,64}$/.test(value.request_id) || typeof value.context !== "string" || value.context.length > 8192 || typeof value.text !== "string" || Buffer.byteLength(value.text) > 128 * 1024 || !Array.isArray(value.files) || (!value.files.length && !value.text.trim()) || value.files.length > uploadLimits.count || new Set(value.files).size !== value.files.length || value.files.some((id) => !validUploadID(id))) throw new Error("Invalid attachment request");
  }
  if (frame.type === "update") {
    const value = frame.update;
    if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).some(key => !["conversation_id", "id", "action", "version", "rating", "rating_version", "request_id"].includes(key)) || typeof value.conversation_id !== "string" || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(value.conversation_id) || typeof value.id !== "string" || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,127}$/.test(value.id) || !["delivered", "read", "resolve", "reopen", "older", "newer", "rate"].includes(value.action) || !Number.isSafeInteger(value.version) || value.version < 0) throw new Error("Invalid update receipt");
    if (value.action === "rate" ? !["useful", "noise", "wrong", "clear"].includes(value.rating) || !Number.isSafeInteger(value.rating_version) || value.rating_version < 0 || typeof value.request_id !== "string" || !/^[a-zA-Z0-9-]{1,64}$/.test(value.request_id) : value.rating !== undefined || value.rating_version !== undefined || value.request_id !== undefined) throw new Error("Invalid update feedback");
  }
  if (frame.type === "receipt") {
    const v = frame.receipt;
    if (!v || typeof v !== "object" || Array.isArray(v) || Object.keys(v).some(key => !["conversation_id", "run_id", "action_id", "correlation_id", "manifest_sha256", "action", "request_id"].includes(key)) || typeof v.conversation_id !== "string" || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(v.conversation_id) || typeof v.run_id !== "string" || !/^[a-zA-Z0-9_-]{1,96}$/.test(v.run_id) || typeof v.action_id !== "string" || !/^action-[a-f0-9]{16}$/.test(v.action_id) || typeof v.correlation_id !== "string" || !/^[a-f0-9]{32}$/.test(v.correlation_id) || typeof v.manifest_sha256 !== "string" || !/^[a-f0-9]{64}$/.test(v.manifest_sha256) || typeof v.request_id !== "string" || !/^[a-zA-Z0-9-]{1,64}$/.test(v.request_id) || !["inspect", "check_sent"].includes(v.action)) throw new Error("Invalid receipt inspection");
  }
  if (frame.type === "memory") {
    const v = frame.memory;
    if (!v || typeof v !== "object" || Array.isArray(v) || Object.keys(v).some(key => !["conversation_id", "id", "updated", "action", "text", "request_id"].includes(key)) || typeof v.conversation_id !== "string" || !/^[a-zA-Z0-9][a-zA-Z0-9_-]{0,95}$/.test(v.conversation_id) || typeof v.id !== "string" || !/^mem-[a-f0-9]{24}$/.test(v.id) || typeof v.updated !== "string" || v.updated.length > 64 || !Number.isFinite(Date.parse(v.updated)) || typeof v.request_id !== "string" || !/^[a-zA-Z0-9-]{1,64}$/.test(v.request_id) || !["correct", "forget", "undo"].includes(v.action) || (v.action === "correct" ? typeof v.text !== "string" || !v.text.trim() || Buffer.byteLength(v.text)>4096 : v.text !== undefined)) throw new Error("Invalid memory review");
  }
  if (frame.type === "navigate" && (typeof frame.text !== "string" || frame.text.length > 4096 || !/^\/[a-z][a-z0-9-]*(?: [a-z][a-z0-9-]*)*$/.test(frame.text) || ["/approve", "/deny", "/quit", "/exit"].includes(frame.text))) throw new Error("Invalid destination");
  if (frame.type === "key" && !["esc", "ctrl+o", "ctrl+p", "ctrl+x", "ctrl+b"].includes(frame.key)) throw new Error("Unsupported shortcut");
  if (frame.type === "approval" && (!Number.isSafeInteger(frame.interaction_id) || frame.interaction_id < 1 || typeof frame.approval_token !== "string" || !/^[a-f0-9]{64}$/.test(frame.approval_token) || typeof frame.approved !== "boolean")) throw new Error("Invalid approval");
  if (frame.type === "queue") {
    const queue = frame.queue;
    if (!queue || typeof queue !== "object" || Array.isArray(queue) || Object.keys(queue).some((key) => !["request_id", "id", "action", "text", "target_id", "placement"].includes(key))) throw new Error("Invalid queue action");
    if (typeof queue.request_id !== "string" || !/^[a-zA-Z0-9-]{1,64}$/.test(queue.request_id) || typeof queue.id !== "string" || !/^[a-f0-9]{64}$/.test(queue.id)) throw new Error("Invalid queue identity");
    if (!["edit", "remove", "earlier", "later", "move", "run", "guide"].includes(queue.action)) throw new Error("Invalid queue action");
    if (queue.action === "move" ? typeof queue.target_id !== "string" || !/^[a-f0-9]{64}$/.test(queue.target_id) || !["before", "after"].includes(queue.placement) : queue.target_id !== undefined || queue.placement !== undefined) throw new Error("Invalid queue destination");
    if (["edit", "guide"].includes(queue.action) ? typeof queue.text !== "string" || !queue.text.trim() || Buffer.byteLength(queue.text, "utf8") > (queue.action === "guide" ? 64 : 128) * 1024 : queue.text !== undefined) throw new Error("Invalid queued message");
  }
  if (frame.type === "composer") {
    const value = frame.composer;
    if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).some(key => !["id", "request_id", "action", "provider", "model", "effort"].includes(key)) || typeof value.id !== "string" || !/^[a-f0-9]{64}$/.test(value.id) || typeof value.request_id !== "string" || !/^[a-zA-Z0-9-]{1,64}$/.test(value.request_id)) throw new Error("Invalid composer settings");
    if (value.action === "model") {
      if (typeof value.provider !== "string" || !/^[a-z][a-z0-9-]{0,127}$/.test(value.provider) || typeof value.model !== "string" || !value.model.trim() || value.model.length > 512 || /[\r\n\0]/.test(value.model) || value.effort !== undefined) throw new Error("Invalid model selection");
    } else if (value.action === "effort") {
      if (!["low", "standard", "high"].includes(value.effort) || value.provider !== undefined || value.model !== undefined) throw new Error("Invalid work effort");
    } else throw new Error("Invalid composer action");
  }
  if (frame.type === "conversation") {
    const value = frame.conversation;
    if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).some((key) => !["id", "list_id"].includes(key)) || typeof value.id !== "string" || !/^[a-zA-Z0-9-]{1,128}$/.test(value.id) || typeof value.list_id !== "string" || !/^[a-f0-9]{64}$/.test(value.list_id)) throw new Error("Invalid conversation selection");
  }
  if (frame.type === "panel") {
    const panel = frame.panel;
    if (!panel || typeof panel !== "object" || Array.isArray(panel) || !/^[a-f0-9]{64}$/.test(panel.id)) throw new Error("Invalid view");
    if (Object.keys(panel).some((key) => !["id", "action", "row", "fields", "text"].includes(key))) throw new Error("Invalid view action");
    if (!["select", "activate", "key", "query", "filter"].includes(panel.action)) throw new Error("Invalid view action");
    if (panel.row !== undefined && (!Number.isSafeInteger(panel.row) || panel.row < 0 || panel.row > 10000)) throw new Error("Invalid row");
    if (panel.action === "key" && (typeof frame.key !== "string" || !/^(enter|esc|up|down|left|right|tab|shift\+tab|space|ctrl\+[ru]|f1|[a-z0-9])$/.test(frame.key))) throw new Error("Invalid view key");
    if (["query", "filter"].includes(panel.action) && (typeof panel.text !== "string" || panel.text.length > 4096)) throw new Error("Invalid search");
    if (panel.fields !== undefined && (!panel.fields || Array.isArray(panel.fields) || typeof panel.fields !== "object" || Object.entries(panel.fields).some(([key, value]) => !/^\d{1,5}$/.test(key) || typeof value !== "string" || value.length > 64 * 1024))) throw new Error("Invalid form");
  }
  return { ...frame, version: 1 };
}

export function shellQuote(value) {
  return `'${String(value).replaceAll("'", "'\\''")}'`;
}
