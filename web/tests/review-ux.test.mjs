import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

test("review UI preserves bounded repair and acceptance controls", async () => {
  const source = await readFile(new URL("../src/App.jsx", import.meta.url), "utf8");
  assert.match(source, /proposal_digest/);
  assert.match(source, /Approve bounded fix/);
  assert.match(source, /Acceptance contract — editable before approval/);
  assert.match(source, /recorded command/);
});
