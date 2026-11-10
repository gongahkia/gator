import { test } from "node:test";
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { Sessions } from "../src/sessions.mjs";

class Worker extends EventEmitter {
  closed = false;
  frames = [];
  focuses = [];
  constructor(id) { super(); this.state = { conversation_id: id, title: id, running: false, queued: 0, foreground: false }; }
  async ready() {}
  async focus(foreground) { this.focuses.push(foreground); this.change({ foreground, session_request: undefined }); }
  send(frame) { this.frames.push(frame); }
  async apply(frame) { this.send(frame); }
  change(patch) { this.state = { ...this.state, ...patch }; this.emit("state", this.state); }
  async stop() { this.closed = true; this.emit("closed"); }
  async prepareUpdate() { await this.stop(); }
}
const setup = () => {
  let count = 0;
  return new Sessions(request => new Worker(request.conversation_id || `chat-${++count}`));
};

class DialogueWorker extends Worker {
  configured = [];
  bootstraps = [];
  controls = [];
  results = [];
  configureDialogue(host) { this.configured.push(host); }
  async bootstrapDialogue(parent,turn) { this.bootstraps.push({parent,turn}); this.change({running:true,dialogue_task:{parent_id:parent,conversation_id:this.state.conversation_id,turn_id:turn,state:"running",run_id:"child-operation",can_guide:true,can_stop:true}}); }
  async controlDialogue(control) { this.controls.push(control); }
  async finishDialogue(result) { this.results.push(result); this.change({dialogue_dispatch:undefined,dialogue_busy:false}); }
}

test("dialogue dispatch starts one background owner and returns results without moving focus",async()=>{
  const parent=new DialogueWorker("parent"),child=new DialogueWorker("child"),spawned=[];
  const sessions=new Sessions(request=>{spawned.push(request);return request.conversation_id==="child"?child:parent});
  try{
    await sessions.open();parent.change({running:true,dialogue_busy:true,dialogue:[{id:"original-turn",task_chat:"child"}],dialogue_dispatch:{sequence:1,kind:"task",turn_id:"original-turn",child_id:"child"}});
    await sessions.settled();await new Promise(resolve=>queueMicrotask(resolve));
    assert.equal(sessions.current,parent);assert.deepEqual(child.focuses,[]);assert.deepEqual(child.bootstraps,[{parent:"parent",turn:"original-turn"}]);
    assert.deepEqual(parent.results,[{turn_id:"original-turn",child_id:"child"}]);
    assert.equal(parent.configured.at(-1).peers[0].run_id,"child-operation");
    parent.change({dialogue_dispatch:{sequence:1,kind:"task",turn_id:"original-turn",child_id:"child"}});await sessions.settled();assert.equal(child.bootstraps.length,1);
    parent.change({dialogue_dispatch:{sequence:2,kind:"guide",turn_id:"correction",child_id:"child",target_run:"child-operation",instruction:"Use the new budget"}});await sessions.settled();
    assert.deepEqual(child.controls,[{parent_id:"parent",turn_id:"original-turn",request_id:"correction",run_id:"child-operation",action:"guide",instruction:"Use the new budget"}]);
    assert.equal(sessions.current,parent);assert.equal(sessions.canOpen("child"),true);assert.equal(sessions.canOpen("unrelated"),false);
    assert.equal(spawned.length,2);
  }finally{await sessions.stop()}
});

test("an unconfirmed background admission or control is reported and never automatically replayed",async()=>{
  const parent=new DialogueWorker("parent"),child=new DialogueWorker("child");
  child.bootstrapDialogue=async()=>{throw new Error("Admission acknowledgement lost; inspect retained work")};
  const sessions=new Sessions(request=>request.conversation_id==="child"?child:parent);
  try{
    await sessions.open();parent.change({running:true,dialogue_dispatch:{sequence:1,kind:"task",turn_id:"turn",child_id:"child"}});await sessions.settled();
    assert.match(parent.results[0].error,/acknowledgement lost/);assert.equal(sessions.current,parent);
    parent.change({title:"Another snapshot"});await sessions.settled();assert.equal(parent.results.length,1);
    parent.change({dialogue_dispatch:{sequence:2,kind:"stop",turn_id:"stop-turn",child_id:"missing",target_run:"old-operation"}});await sessions.settled();
    assert.match(parent.results[1].error,/original task owner is unavailable/);assert.equal(child.controls.length,0);
  }finally{await sessions.stop()}
});

test("plan cancellation reports the original owner's confirmed receipt without moving focus", async () => {
  const parent = new DialogueWorker("parent"), child = new DialogueWorker("child");
  const sessions = new Sessions(request => request.conversation_id === "child" ? child : parent);
  try {
    await sessions.open();
    parent.change({ running: true, dialogue_dispatch: { sequence: 1, kind: "task", turn_id: "plan-turn", child_id: "child" } });
    await sessions.settled();
    child.controlDialogue = async control => { child.controls.push(control); return { request_id: control.request_id, cancelled_plan: true }; };
    parent.change({ dialogue_dispatch: { sequence: 2, kind: "stop", turn_id: "stop-plan", child_id: "child", target_run: "exact-plan-revision" } });
    await sessions.settled();
    assert.deepEqual(parent.results.at(-1), { turn_id: "stop-plan", child_id: "child", cancelled_plan: true });
    assert.equal(child.controls.at(-1).run_id, "exact-plan-revision");
    assert.equal(sessions.current, parent);
    assert.equal(parent.state.running, true);
    child.controlDialogue = async () => { throw new Error("Plan revision changed"); };
    parent.change({ dialogue_dispatch: { sequence: 3, kind: "stop", turn_id: "stale-stop", child_id: "child", target_run: "stale-plan-revision" } });
    await sessions.settled();
    assert.match(parent.results.at(-1).error, /Plan revision changed/);
    assert.equal(parent.results.at(-1).cancelled_plan, undefined);
  } finally { await sessions.stop(); }
});

test("plan revision admission releases the parent conversation while its original owner prepares", async () => {
  const parent = new DialogueWorker("parent"), child = new DialogueWorker("child");
  const sessions = new Sessions(request => request.conversation_id === "child" ? child : parent);
  try {
    await sessions.open();
    parent.change({ running: true, dialogue_dispatch: { sequence: 1, kind: "task", turn_id: "plan-turn", child_id: "child" } });
    await sessions.settled();
    child.controlDialogue = async control => { child.controls.push(control); return { request_id: control.request_id, revising_plan: true }; };
    parent.change({ dialogue_busy: true, dialogue_dispatch: { sequence: 2, kind: "revise_plan", turn_id: "correction-turn", child_id: "child", target_run: "exact-plan-epoch", instruction: "Keep the comparison small" } });
    await sessions.settled();
    assert.equal(parent.state.dialogue_busy, false);
    assert.equal(sessions.current, parent);
    assert.deepEqual(parent.results.at(-1), { turn_id: "correction-turn", child_id: "child", revising_plan: true });
    assert.deepEqual(child.controls.at(-1), { parent_id: "parent", turn_id: "plan-turn", request_id: "correction-turn", run_id: "exact-plan-epoch", action: "revise_plan", instruction: "Keep the comparison small" });
  } finally { await sessions.stop(); }
});

test("a plan correction rechecks the task limit while stop remains available", async () => {
  const parent = new DialogueWorker("parent"), child = new DialogueWorker("child"), others = [new DialogueWorker("other-a"), new DialogueWorker("other-b")];
  const sessions = new Sessions(request => request.conversation_id === "child" ? child : others.find(worker => worker.state.conversation_id === request.conversation_id) || parent);
  try {
    await sessions.open();
    parent.change({ running: true, dialogue_dispatch: { sequence: 1, kind: "task", turn_id: "plan-turn", child_id: "child" } });
    await sessions.settled(); child.change({ running: false });
    for (const other of others) { await sessions.open({ conversation_id: other.state.conversation_id }); other.change({ running: true }); }
    await sessions.open({ conversation_id: "parent" });
    parent.change({ dialogue_dispatch: { sequence: 2, kind: "revise_plan", turn_id: "over-capacity", child_id: "child", target_run: "plan-epoch", instruction: "Keep it small" } });
    await sessions.settled();
    assert.match(parent.results.at(-1).error, /Three tasks are working/); assert.equal(child.controls.length, 0);
    parent.change({ dialogue_dispatch: { sequence: 3, kind: "stop", turn_id: "stop-plan", child_id: "child", target_run: "plan-epoch" } });
    await sessions.settled();
    assert.equal(child.controls.at(-1).action, "stop"); assert.equal(parent.results.at(-1).error, undefined);
    parent.change({ running: false }); child.change({ running: true });
    parent.change({ dialogue_dispatch: { sequence: 4, kind: "revise_plan", turn_id: "local-over-capacity", child_id: "parent", owner_task: "local-plan", target_run: "local-plan-epoch", instruction: "Keep it small" } });
    await sessions.settled();
    assert.match(parent.results.at(-1).error, /Three tasks are working/); assert.equal(parent.controls.length, 0);
    child.change({ running: false });
    parent.controlDialogue = async control => { parent.controls.push(control); return { request_id: control.request_id, revising_plan: true }; };
    parent.change({ dialogue_dispatch: { sequence: 5, kind: "revise_plan", turn_id: "local-with-capacity", child_id: "parent", owner_task: "local-plan", target_run: "local-plan-epoch", instruction: "Keep it small" } });
    await sessions.settled();
    assert.deepEqual(parent.controls.at(-1), { parent_id: "parent", turn_id: "local-plan", request_id: "local-with-capacity", run_id: "local-plan-epoch", action: "revise_plan", instruction: "Keep it small" });
    assert.equal(parent.results.at(-1).revising_plan, true); assert.equal(sessions.current, parent);
  } finally { await sessions.stop(); }
});

test("managed navigation waits for the new owner's applied view and preserves original execution", async () => {
  const original = new Worker("original"), view = new Worker("view");
  let release, applied;
  const gate = new Promise(resolve => { release = resolve; }), dispatched = new Promise(resolve => { applied = resolve; });
  view.apply = async frame => { view.send(frame); applied(); await gate; view.change({panel: {title: "Ongoing goals"}}); };
  const sessions = new Sessions(request => request.view ? view : original);
  await sessions.open(); original.change({running: true, pending_approval: {id: 7, token: "original-only"}});
  let finished = false;
  const opening = sessions.open({view: "/goals"}).then(() => { finished = true; });
  await dispatched;
  assert.equal(finished, false); assert.equal(sessions.switching, true); assert.equal(view.state.panel, undefined);
  assert.equal(original.state.running, true); assert.equal(original.state.pending_approval.token, "original-only");
  release(); await opening;
  assert.equal(sessions.current, view); assert.equal(view.state.panel.title, "Ongoing goals"); assert.equal(sessions.switching, false);
  assert.deepEqual(original.frames, []); await sessions.stop();
});

test("a failed managed view acknowledgement retires only the new view and restores the original owner", async () => {
  const original = new Worker("original"), failed = new Worker("failed");
  failed.apply = async () => { throw new Error("view was not confirmed"); };
  const sessions = new Sessions(request => request.view ? failed : original);
  await sessions.open(); original.change({running: true, queued: 2});
  await assert.rejects(sessions.open({view: "/goals"}), /not confirmed/);
  assert.equal(sessions.current, original); assert.equal(original.state.foreground, true); assert.equal(original.state.running, true); assert.equal(original.state.queued, 2);
  assert.equal(failed.closed, true); assert.equal(sessions.workers.size, 1); assert.equal(sessions.switching, false);
  await sessions.stop();
});

test("direct ancestry navigation uses only the current owner's exact parent even when idle", async () => {
  const sessions = setup(); await sessions.open(); const parent = sessions.current;
  await sessions.open({ new: true }); const child = sessions.current;
  child.change({ branch: { conversation_id: parent.state.conversation_id, revision_id: "saved-revision" } });
  assert.deepEqual(sessions.activity(), []);
  assert.equal(sessions.canOpen(parent.state.conversation_id), true);
  for (const id of ["unrelated", parent.state.conversation_id + "-suffix", "../private", "", undefined, { id: parent.state.conversation_id }]) assert.equal(sessions.canOpen(id), false);
  await sessions.open({ conversation_id: parent.state.conversation_id });
  assert.equal(sessions.current, parent); assert.deepEqual(parent.frames, []);
  assert.equal(sessions.canOpen(parent.state.conversation_id), false); // No cached ancestry authority.
  await sessions.open({ conversation_id: child.state.conversation_id });
  child.change({ branch: undefined });
  assert.equal(sessions.canOpen(parent.state.conversation_id), false);
  child.change({ branch: { conversation_id: parent.state.conversation_id, revision_id: "saved-revision" } });
  await child.stop();
  assert.equal(sessions.canOpen(parent.state.conversation_id), false);
  await sessions.stop();
});

test("browser sign-in retains its owner and Connections returns to it", async () => {
  const sessions = setup(); await sessions.open(); const owner = sessions.current;
  owner.change({ pending_connection: "google" });
  assert.equal(sessions.busy, true);
  await assert.rejects(sessions.restart(), /current operation/);
  await sessions.open({ new: true });
  await assert.rejects(sessions.prepareProfileAction({ type: "navigate", text: "/data" }), /other chats/);
  assert.equal(await sessions.resumeConnection({ type: "navigate", text: "/connections" }), true);
  assert.equal(sessions.current, owner);
  assert.deepEqual(owner.frames.at(-1), { type: "navigate", text: "/connections" });
  for (let i = 0; i < 5; i++) await sessions.open({ new: true });
  assert.equal(owner.closed, false);
  await assert.rejects(sessions.prepareUpdate(), /all chats/);
  owner.change({ pending_connection: undefined });
  assert.equal(sessions.busy, false);
  await sessions.stop();
});

test("durable update badges include unopened chats and clear from fresh receipts", async () => {
  const sessions = setup(); await sessions.open();
  sessions.current.change({ update_conversations: [{ id: "unopened-chat", title: "Offsite", count: 2 }] });
  assert.deepEqual(sessions.activity(), [{ id: "unopened-chat", title: "Offsite", status: "2 updates", running: false }]);
  const worker = sessions.current;
  await sessions.open({ conversation_id: "unopened-chat" });
  assert.equal(sessions.current.state.conversation_id, "unopened-chat");
  assert.equal(sessions.current.frames.length, 0);
  assert.deepEqual(sessions.activity(), []);
  worker.change({ update_conversations: [{ id: "unopened-chat", title: "Offsite", count: 2 }] });
  assert.deepEqual(sessions.activity(), []); // A stale background projection cannot restore a read badge.
  await sessions.stop();
});

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

test("reviewed plan starts share the existing three-working-chat admission limit", async () => {
  const sessions = setup();
  for (let i = 0; i < 3; i++) { await sessions.open({ new: true }); sessions.current.change({ running: true }); }
  await sessions.open({ new: true });
  for (const action of ["approve", "start"]) assert.throws(() => sessions.assertSend({ type: "task_plan", task_plan: { action } }), /Three chats/);
  for (const action of ["inspect", "edit", "cancel"]) assert.doesNotThrow(() => sessions.assertSend({ type: "task_plan", task_plan: { action } }));
  [...sessions.workers][0].change({ running: false });
  for (const action of ["approve", "start"]) assert.doesNotThrow(() => sessions.assertSend({ type: "task_plan", task_plan: { action } }));
  assert.equal([...sessions.workers].filter(worker => worker.state.running).length, 2);
  assert.deepEqual(sessions.current.frames, []); // Validation never dispatches.
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


for (const busyFlag of ["receipt_busy", "catalog_busy"]) test(`${busyFlag} retains its runtime and blocks profile maintenance and restart`, async () => {
 const sessions=setup();await sessions.open();const owner=sessions.current;
 owner.change({[busyFlag]:true});assert.equal(sessions.busy,true);
 await assert.rejects(sessions.restart(),/current operation/);
 await assert.rejects(sessions.prepareProfileAction({type:"navigate",text:"/data"}),/current local operation/);
 await sessions.open({new:true});
 await assert.rejects(sessions.prepareProfileAction({type:"navigate",text:"/recovery"}),/other chats/);
 await assert.rejects(sessions.prepareUpdate(),/all chats/);
 for(let i=0;i<5;i++)await sessions.open({new:true});
 assert.equal(owner.closed,false);
 owner.change({[busyFlag]:false});assert.equal(sessions.busy,false);
 await sessions.stop();
});
