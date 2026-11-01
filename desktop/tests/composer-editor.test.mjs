import test from "node:test";
import assert from "node:assert/strict";
import { editComposer } from "../electron/composer-editor.mjs";

const request = { context: "conversation:work-fixture", text: "Kept λ draft" };
function owner() {
  return { state: { conversation_id: "work-fixture" }, prepared: 0, completed: 0,
    async prepareComposerEditor(text) { this.prepared++; this.state.input = text; this.state.external = { kind: "editor", program: "host-prepared", args: ["private-file"] }; },
    async completeExternal(error) { this.completed++; this.error = error; delete this.state.external; this.state.input = "Saved editor text"; }
  };
}
test("composer editor rejects wrong ownership, executable injection and non-idle state before handoff", async () => {
  for (const state of [{ running: true }, { queued: 1 }, { pending_approval: {} }, { panel: {} }, { external: {} }, { conversation_id: "work-other" }]) {
    const backend = owner(); Object.assign(backend.state, state);
    await assert.rejects(editComposer(backend, request, () => assert.fail("dispatched")), /idle conversation/);
    assert.equal(backend.prepared, 0);
  }
  const backend = owner();
  for (const bad of [{ ...request, program: "/bin/sh" }, { ...request, text: 42 }, { ...request, text: "x".repeat(128 * 1024 + 1) }]) await assert.rejects(editComposer(backend, bad, () => assert.fail("dispatched")));
  assert.equal(backend.prepared, 0);
});
test("editor uses only the Go-prepared job and waits for completion even after native errors", async () => {
  for (const fail of [false, true]) {
    const backend = owner();
    const result = await editComposer(backend, request, async job => { assert.equal(job.program, "host-prepared"); assert.equal(backend.completed, 0); if (fail) throw new Error("fixture launcher failed"); return ""; });
    assert.equal(backend.completed, 1); assert.equal(result.text, "Saved editor text"); assert.equal(result.context, request.context); assert.equal(Boolean(result.error), fail);
  }
});
