import { test } from "node:test";
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { Sessions } from "../electron/sessions.mjs";

class Worker extends EventEmitter {
  closed = false;
  frames = [];
  focuses = [];
  constructor(id) { super(); this.state = { conversation_id: id, title: id, running: false, queued: 0, foreground: false }; }
  async ready() {}
  async focus(foreground) { this.focuses.push(foreground); this.change({ foreground, session_request: undefined }); }
  send(frame) { this.frames.push(frame); }
  change(patch) { this.state = { ...this.state, ...patch }; this.emit("state", this.state); }
  async stop() { this.closed = true; this.emit("closed"); }
  async prepareUpdate() { await this.stop(); }
}
const setup = () => {
  let count = 0;
  return new Sessions(request => new Worker(request.conversation_id || `chat-${++count}`));
};

test("chat navigation retains original execution, approvals and unread results", async () => {
  const sessions = setup(); await sessions.open(); const first = sessions.current;
  first.change({ running: true, pending_approval: { id: 8, token: "first-only" }, queued: 2 });
  await sessions.open({ new: true }); const second = sessions.current;
  assert.notEqual(first, second); assert.equal(first.closed, false); assert.equal(first.state.running, true);
  assert.equal(first.state.foreground, false); assert.equal(second.state.foreground, true);
  assert.equal(sessions.activity().find(row => row.id === first.state.conversation_id).status, "Needs review");
  assert.throws(() => sessions.assertSend({ type: "navigate", text: "/data" }), /other chats/);
  await assert.rejects(sessions.prepareUpdate(), /all chats/);
  first.change({ running: false, pending_approval: undefined, queued: 0, messages: [{ activity: { state: "failed" } }] });
  assert.equal(sessions.activity()[0].status, "Needs attention");
  await sessions.open({ conversation_id: first.state.conversation_id });
  assert.equal(sessions.current, first); assert.equal(sessions.workers.size, 2); assert.deepEqual(sessions.activity(), []);
  assert.deepEqual(first.frames, []); // Navigation sends no prompt or approval.
  await sessions.stop(); assert.equal(first.closed, true); assert.equal(second.closed, true);
});

test("session capacity never evicts running or queued work and admissions are bounded", async () => {
  const sessions = setup();
  for (let i = 0; i < 3; i++) { await sessions.open({ new: true }); sessions.current.change({ running: true }); }
  await sessions.open({ new: true });
  assert.throws(() => sessions.assertSend({ type: "upload_submit" }), /Three chats/);
  await assert.rejects(sessions.open({ new: true }), /Four chats/);
  assert.equal(sessions.workers.size, 4); assert.equal([...sessions.workers].some(worker => worker.closed), false);
  const first = [...sessions.workers][0]; first.change({ running: false, queued: 1 });
  sessions.assertSend({ type: "upload_submit" });
  await assert.rejects(sessions.open({ new: true }), /Four chats/);
  first.change({ queued: 0 });
  await sessions.open({ new: true }); assert.equal(first.closed, true); assert.equal(sessions.workers.size, 4);
  await sessions.stop();
});

test("failed chat startup restores focus and leaves original work running", async () => {
  const first = new Worker("original"), failed = new Worker("failed");
  failed.ready = async () => { throw new Error("unavailable conversation"); };
  let count = 0; const sessions = new Sessions(() => count++ ? failed : first);
  await sessions.open(); first.change({ running: true });
  await assert.rejects(sessions.open({ new: true }), /unavailable/);
  assert.equal(sessions.current, first); assert.equal(first.state.foreground, true);
  assert.equal(first.state.running, true); assert.equal(first.closed, false); assert.equal(failed.closed, true);
  await sessions.stop();
});

test("host navigation events are consumed once and reconnect targets only the selected chat", async () => {
  const sessions = setup(); await sessions.open(); const first = sessions.current;
  first.change({ running: true, session_request: { sequence: 1, new: true } });
  await new Promise(resolve => setImmediate(resolve));
  assert.equal(sessions.workers.size, 2); assert.notEqual(sessions.current, first);
  const second = sessions.current; await sessions.restart();
  assert.equal(second.closed, true); assert.equal(first.closed, false); assert.equal(first.state.running, true);
  assert.equal(sessions.current.state.conversation_id, second.state.conversation_id);
  await sessions.stop();
});

test("profile maintenance closes idle cached chats and refuses pending background work", async () => {
  const sessions = setup(); await sessions.open(); const first = sessions.current;
  await sessions.open({ new: true });
  first.change({ queued: 1 });
  await assert.rejects(sessions.prepareProfileAction({ type: "navigate", text: "/data" }), /other chats/);
  assert.equal(first.closed, false);
  first.change({ queued: 0 });
  await sessions.prepareProfileAction({ type: "navigate", text: "/data" });
  assert.equal(first.closed, true); assert.equal(sessions.workers.size, 1); assert.deepEqual(sessions.activity(), []);
  await sessions.stop();
});
