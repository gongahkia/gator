import { test } from "node:test";
import assert from "node:assert/strict";
import { assetPath, externalURL, shellQuote, validateFrame } from "../electron/policy.mjs";
import { resolve, join } from "node:path";

test("composer choices carry only bounded identities and canonical work effort", () => {
  const composer = { id: "a".repeat(64), request_id: "picker-1", action: "model", provider: "fixture", model: "model-one" };
  assert.equal(validateFrame({ type: "composer", composer }).version, 1);
  for (const change of [{ id: "old" }, { request_id: "" }, { model: "\nsecret" }, { action: "execute" }, { effort: "high" }, { endpoint: "https://example.com" }]) assert.throws(() => validateFrame({ type: "composer", composer: { ...composer, ...change } }));
  const effort = { id: composer.id, request_id: "effort-1", action: "effort", effort: "high" };
  assert.equal(validateFrame({ type: "composer", composer: effort }).version, 1);
  for (const change of [{ effort: "max" }, { provider: "openai" }, { model: "unlisted" }]) assert.throws(() => validateFrame({ type: "composer", composer: { ...effort, ...change } }));
});

test("renderer cannot launch processes, acknowledge handoffs, or inject transport fields", () => {
  for (const frame of [{ type: "external_done" }, { type: "exit" }, { type: "submit", text: "ok", program: "sh" }, { type: "key", key: "ctrl+g" }, { type: "navigate", text: "/approve" }]) assert.throws(() => validateFrame(frame));
  assert.deepEqual(validateFrame({ type: "navigate", text: "/overview" }), { version: 1, type: "navigate", text: "/overview" });
});

test("panel actions require a view identity and bounded typed values", () => {
  const frame = { type: "panel", key: "ctrl+r", panel: { id: "a".repeat(64), action: "key", fields: { 0: "review me" } } };
  assert.equal(validateFrame(frame).version, 1);
  assert.equal(validateFrame({ type: "panel", panel: { id: frame.panel.id, action: "filter", text: "completed" } }).version, 1);
  for (const text of [undefined, false, "x".repeat(4097)]) assert.throws(() => validateFrame({ type: "panel", panel: { id: frame.panel.id, action: "filter", text } }));
  for (const change of [{ id: "" }, { row: -1 }, { action: "execute" }, { fields: { "-1": "bad" } }, { fields: { 0: false } }, { program: "sh" }]) assert.throws(() => validateFrame({ ...frame, panel: { ...frame.panel, ...change } }));
  assert.throws(() => validateFrame({ type: "approval", interaction_id: 0, approved: true }));
  assert.throws(() => validateFrame({ type: "approval", interaction_id: 1, approved: "true" }));
  assert.throws(() => validateFrame({ type: "approval", interaction_id: 1, approved: true }));
  assert.equal(validateFrame({ type: "approval", interaction_id: 1, approval_token: "a".repeat(64), approved: true }).version, 1);
});

test("app resources stay inside the compiled assets and links stay web-only", () => {
  const root = resolve("test-ui");
  assert.equal(assetPath("gator://app/assets/main.js", root), join(root, "assets/main.js"));
  for (const url of ["file:///etc/passwd", "gator://other/index.html", "gator://app/%2f..%2fprivate", "gator://app/%00", "gator://app/%5c..%5cprivate"]) assert.equal(assetPath(url, "/tmp/gator-ui"), null);
  for (const url of ["javascript:alert(1)", "file:///etc/passwd", "https://user:pass@example.com", "gator://app/index.html"]) assert.equal(externalURL(url), null);
  assert.equal(externalURL("https://example.com/docs"), "https://example.com/docs");
  assert.equal(shellQuote("folder's $(touch /tmp/nope)"), "'folder'\\''s $(touch /tmp/nope)'");
});

test("queue actions bind to a message or active run and accept bounded text only", () => {
  const queue = { request_id: "request-1", id: "a".repeat(64), action: "edit", text: "Updated request" };
  assert.equal(validateFrame({ type: "queue", queue }).version, 1);
  for (const change of [{ id: "" }, { request_id: "" }, { action: "execute" }, { text: " " }, { text: "🐊".repeat(32769) }, { program: "sh" }]) assert.throws(() => validateFrame({ type: "queue", queue: { ...queue, ...change } }));
  assert.throws(() => validateFrame({ type: "queue", queue: { ...queue, action: "remove" } }));
  assert.throws(() => validateFrame({ type: "queue", queue: { ...queue, action: "guide", text: "x".repeat(65537) } }));
  assert.equal(validateFrame({ type: "queue", queue: { request_id: "move", id: queue.id, action: "earlier" } }).version, 1);
  const drop = { request_id: "drop", id: queue.id, action: "move", target_id: "b".repeat(64), placement: "before" };
  assert.equal(validateFrame({ type: "queue", queue: drop }).version, 1);
  for (const invalid of [{ target_id: "" }, { placement: "middle" }, { text: "replace the message" }, { action: "remove" }, { position: 0 }]) assert.throws(() => validateFrame({ type: "queue", queue: { ...drop, ...invalid } }));
});

test("recent conversation selection requires the exact projected list and a bounded identity", () => {
  const conversation = { id: "work-one", list_id: "a".repeat(64) };
  assert.equal(validateFrame({ type: "conversation", conversation }).version, 1);
  for (const change of [{ id: "../other" }, { id: "" }, { list_id: "" }, { program: "sh" }]) assert.throws(() => validateFrame({ type: "conversation", conversation: { ...conversation, ...change } }));
});
