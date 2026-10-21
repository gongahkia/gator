import { test } from "node:test";
import assert from "node:assert/strict";
import { assetPath, externalURL, shellQuote, validateFrame } from "../electron/policy.mjs";

test("renderer cannot launch processes, acknowledge handoffs, or inject transport fields", () => {
  for (const frame of [{ type: "external_done" }, { type: "exit" }, { type: "submit", text: "ok", program: "sh" }, { type: "key", key: "ctrl+g" }, { type: "navigate", text: "/approve" }]) assert.throws(() => validateFrame(frame));
  assert.deepEqual(validateFrame({ type: "navigate", text: "/overview" }), { version: 1, type: "navigate", text: "/overview" });
});

test("panel actions require a view identity and bounded typed values", () => {
  const frame = { type: "panel", key: "ctrl+r", panel: { id: "a".repeat(64), action: "key", fields: { 0: "review me" } } };
  assert.equal(validateFrame(frame).version, 1);
  for (const change of [{ id: "" }, { row: -1 }, { action: "execute" }, { fields: { "-1": "bad" } }, { fields: { 0: false } }, { program: "sh" }]) assert.throws(() => validateFrame({ ...frame, panel: { ...frame.panel, ...change } }));
  assert.throws(() => validateFrame({ type: "approval", interaction_id: 0, approved: true }));
  assert.throws(() => validateFrame({ type: "approval", interaction_id: 1, approved: "true" }));
});

test("app resources stay inside the compiled assets and links stay web-only", () => {
  assert.equal(assetPath("gator://app/assets/main.js", "/tmp/gator-ui"), "/tmp/gator-ui/assets/main.js");
  for (const url of ["file:///etc/passwd", "gator://other/index.html", "gator://app/%2f..%2fprivate", "gator://app/%00", "gator://app/%5c..%5cprivate"]) assert.equal(assetPath(url, "/tmp/gator-ui"), null);
  for (const url of ["javascript:alert(1)", "file:///etc/passwd", "https://user:pass@example.com", "gator://app/index.html"]) assert.equal(externalURL(url), null);
  assert.equal(externalURL("https://example.com/docs"), "https://example.com/docs");
  assert.equal(shellQuote("folder's $(touch /tmp/nope)"), "'folder'\\''s $(touch /tmp/nope)'");
});
