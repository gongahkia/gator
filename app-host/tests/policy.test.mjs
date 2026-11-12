import { test } from "node:test";
import assert from "node:assert/strict";
import { assetPath, externalURL, shellQuote, validateFrame } from "../src/policy.mjs";
import { resolve, join } from "node:path";
import { frontendDefaults } from "../src/preferences.mjs";

test("terminal preferences accept bounded display changes and complete imports without authority fields", () => {
  const identity = { id: "a".repeat(64), request_id: "preference-1" };
  const change = { ...identity, action: "configure", patch: { theme: "terminal", sidebarCollapsed: false } };
  assert.equal(validateFrame({ type: "terminal_preferences", terminal_preferences: change }).version, 1);
  assert.equal(validateFrame({ type: "terminal_preferences", terminal_preferences: { ...identity, action: "migrate", legacy: frontendDefaults() } }).version, 1);
  for (const patch of [{ theme: "bad" }, { sidebarCollapsed: "yes" }, { api_key: "secret" }, { approved: true }, { workDetailsSections: { command: "run" } }]) assert.throws(() => validateFrame({ type: "terminal_preferences", terminal_preferences: { ...change, patch } }));
  for (const extra of [{ id: "old" }, { request_id: "" }, { legacy: frontendDefaults() }, { action: "run" }, { path: "/other" }]) assert.throws(() => validateFrame({ type: "terminal_preferences", terminal_preferences: { ...change, ...extra } }));
  assert.throws(() => validateFrame({ type: "terminal_preferences", terminal_preferences: { ...identity, action: "migrate", legacy: { theme: "terminal" } } }));
});

test("commitment controls accept only current identities, never replacement policy or execution", () => {
  const check = {request_id: "review-1", id: "job-one", digest: "a".repeat(64), action: "pause"};
  for (const action of ["inspect", "edit", "pause", "resume"]) assert.equal(validateFrame({type: "check", check: {...check, action}}).version, 1);
  assert.equal(validateFrame({type: "check", check: {request_id: "refresh-1", action: "refresh"}}).version, 1);
  const setup = {request_id: "setup-1", action: "setup", id: "goal-one"};
  assert.equal(validateFrame({type: "check", check: setup}).version, 1);
  for (const change of [{id: "../goal"}, {digest: "a".repeat(64)}, {source: "/other"}, {goal_id: "other"}, {approved: true}]) assert.throws(() => validateFrame({type: "check", check: {...setup, ...change}}));
  for (const change of [{id: "../job"}, {id: ""}, {digest: "old"}, {digest: "A".repeat(64)}, {request_id: ""}, {action: "run"}, {action: "refresh"}, {source: "/other"}, {enabled: true}, {connector_ids: ["other"]}, {approved: true}]) assert.throws(() => validateFrame({type: "check", check: {...check, ...change}}));
});

test("goal controls carry only the original goal and reviewed status identity", () => {
  const goal = { request_id: "goal-review-1", id: "goal-" + "a".repeat(24), action: "pause", digest: "b".repeat(64) };
  for (const action of ["pause", "resume"]) assert.equal(validateFrame({ type: "goal", goal: { ...goal, action } }).version, 1);
  assert.equal(validateFrame({ type: "goal", goal: { ...goal, id: "goal-" + "A".repeat(24) } }).version, 1);
  const inspect = { request_id: goal.request_id, id: goal.id, action: "inspect" };
  assert.equal(validateFrame({ type: "goal", goal: inspect }).version, 1);
  assert.throws(() => validateFrame({ type: "goal", goal: { ...inspect, digest: goal.digest } }));
  for (const change of [{ action: "complete" }, { action: "run" }, { digest: "" }, { digest: "B".repeat(64) }, { id: "../other" }, { id: "goal-other" }, { request_id: "" }, { status: "active" }, { source: "/another" }, { account: "other" }, { approved: true }]) assert.throws(() => validateFrame({ type: "goal", goal: { ...goal, ...change } }));
});

test("specialist controls accept bounded actions without renderer-supplied authority", () => {
  const specialist = { id: "subagent-001", token: "a".repeat(64), request_id: "guide-one", action: "guide", text: "Use primary sources" };
  assert.equal(validateFrame({ type: "specialist", specialist }).version, 1);
  assert.equal(validateFrame({ type: "specialist", specialist: { ...specialist, action: "stop", text: undefined } }).version, 1);
  for (const change of [{ id: "../subagent-001" }, { token: "old" }, { request_id: "" }, { action: "start" }, { text: " " }, { text: "x".repeat(8193) }, { text: "a\0b" }, { command: "sh" }, { conversation_id: "other" }, { action: "stop" }]) {
    assert.throws(() => validateFrame({ type: "specialist", specialist: { ...specialist, ...change } }));
  }
});

test("update receipts and replies accept identities but never renderer-supplied authority", () => {
  const update = { conversation_id: "work-one", id: "inbox-one", action: "read", version: 0 };
  assert.equal(validateFrame({ type: "update", update }).version, 1);
  for (const change of [{ conversation_id: "../other" }, { action: "approve" }, { version: -1 }, { version: 0.5 }, { summary: "replacement" }, { job_id: "different-job" }]) assert.throws(() => validateFrame({ type: "update", update: { ...update, ...change } }));
  const upload = { request_id: "reply-one", context: "conversation:work-one", text: "Use option two", files: [], reply_to_update_id: "inbox-one" };
  assert.equal(validateFrame({ type: "upload_submit", upload }).version, 1);
  assert.throws(() => validateFrame({ type: "upload_submit", upload: { ...upload, reply_to_update_id: "../other" } }));
  assert.throws(() => validateFrame({ type: "upload_submit", upload: { ...upload, goal_id: "other" } }));
});

test("update feedback requires a bounded nonce and independent reviewed version", () => {
  const update = { conversation_id: "work-one", id: "inbox-one", action: "rate", version: 0, rating: "useful", rating_version: 2, request_id: "feedback-1" };
  assert.equal(validateFrame({ type: "update", update }).version, 1);
  for (const change of [{ rating: "good" }, { rating_version: undefined }, { rating_version: -1 }, { rating_version: 1.5 }, { request_id: "" }, { request_id: "a".repeat(65) }, { action: "read" }]) assert.throws(() => validateFrame({ type: "update", update: { ...update, ...change } }));
});

test("update item actions accept a bounded outcome nonce without feedback or replacement authority", () => {
  const update = { conversation_id: "work-one", id: "inbox-one", action: "resolve", version: 3, request_id: "done-1" };
  for (const action of ["read", "resolve", "reopen", "delivered", "older", "newer"]) assert.equal(validateFrame({ type: "update", update: { ...update, action } }).version, 1);
  for (const change of [{ request_id: "" }, { request_id: "a".repeat(65) }, { request_id: "bad id" }, { request_id: 7 }, { rating: "useful" }, { rating_version: 2 }, { origin_conversation_id: "other" }, { account: "other" }]) assert.throws(() => validateFrame({ type: "update", update: { ...update, ...change } }));
});

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

test("answer context navigation admits exact run IDs without generalizing navigation arguments", () => {
  const text = "/context work-20261112T030608-603cbcf614c5";
  assert.equal(validateFrame({ type: "navigate", text }).text, text);
  assert.equal(validateFrame({ type: "navigate", text: "/context Run_1" }).version, 1);
  for (const text of ["/context ../other", "/context run;send", "/context Run_1 extra", "/context " + "A".repeat(97), "/settings Run_1"]) assert.throws(() => validateFrame({ type: "navigate", text }));
});

test("panel actions require a view identity and bounded typed values", () => {
  const frame = { type: "panel", key: "ctrl+r", panel: { id: "a".repeat(64), action: "key", fields: { 0: "review me" } } };
  assert.equal(validateFrame(frame).version, 1);
  assert.equal(validateFrame({ ...frame, key: "ctrl+o" }).version, 1);
  for (const key of ["ctrl+e", "ctrl+x", "ctrl+a"]) assert.equal(validateFrame({ ...frame, key }).version, 1);
  assert.throws(() => validateFrame({ ...frame, key: "f1" }));
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

test("conversation organization carries only a reviewed row and bounded title", () => {
  const panel = { id: "a".repeat(64), action: "rename", row: 3, text: "A chosen title" };
  assert.equal(validateFrame({ type: "panel", panel }).version, 1);
  for (const change of [{ row: undefined }, { text: " " }, { text: "🐊".repeat(65) }, { text: "first\nsecond" }, { conversation_id: "other-chat" }, { version: "forged" }, { action: "delete" }]) assert.throws(() => validateFrame({ type: "panel", panel: { ...panel, ...change } }));
  for (const action of ["pin", "unpin", "archive", "restore"]) {
    assert.equal(validateFrame({ type: "panel", panel: { id: panel.id, action, row: 1 } }).version, 1);
    assert.throws(() => validateFrame({ type: "panel", panel: { id: panel.id, action } }));
  }
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

test("memory review accepts bounded exact revisions and rejects supplied scope", () => {
  const memory = {conversation_id:"work-chat",id:"mem-"+"a".repeat(24),updated:"2026-10-23T00:00:00.123456789Z",action:"correct",text:"Detailed answers",request_id:"edit-1"};
  assert.equal(validateFrame({type:"memory",memory}).version,1);
  for(const change of [{id:"../memory"},{updated:"unknown"},{action:"create"},{goal_id:"other"},{text:"x".repeat(4097)},{action:"undo"},{request_id:""}]) assert.throws(()=>validateFrame({type:"memory",memory:{...memory,...change}}));
  const {text,...undo}=memory;assert.equal(validateFrame({type:"memory",memory:{...undo,action:"undo"}}).version,1);
});


test("receipt inspection only accepts exact retained identities and read-only actions", () => {
  const receipt = {conversation_id:"work_chat",run_id:"work-one",action_id:"action-"+"a".repeat(16),correlation_id:"b".repeat(32),manifest_sha256:"c".repeat(64),action:"check_sent",request_id:"check-1"};
  assert.equal(validateFrame({type:"receipt",receipt}).version,1);
  assert.equal(validateFrame({type:"receipt",receipt:{...receipt,action:"inspect"}}).version,1);
  for(const change of [{conversation_id:"../chat"},{run_id:"../../run"},{action_id:"action-other"},{correlation_id:"x".repeat(32)},{manifest_sha256:"c".repeat(63)},{action:"resend"},{request_id:""},{request_id:"x".repeat(65)},{payload:{to:"other@example.com"}},{bundle_path:"/private/run"},{credentials:"token"},{query:"in:anywhere"}]) assert.throws(()=>validateFrame({type:"receipt",receipt:{...receipt,...change}}));
});

test("source selection accepts only finite source IDs and no renderer authority", () => {
 const value={id:"a".repeat(64),request_id:"request",action:"select",ids:["reports"]};
 assert.equal(validateFrame({type:"sources",sources:value}).version,1);
 for(const change of [{ids:["../private"]},{ids:["reports","reports"]},{ids:Array.from({length:17},(_,i)=>`source-${i}`)},{grant:{operation:"write"}},{endpoint:"https://example.com"},{action:"enroll"},{id:"stale"}]) assert.throws(()=>validateFrame({type:"sources",sources:{...value,...change}}));
 assert.doesNotThrow(()=>validateFrame({type:"sources",sources:{...value,ids:[]}}));
 assert.throws(()=>validateFrame({type:"sources",sources:{...value,action:"refresh"}}));
});

test("branch creation accepts exact durable references but no replay or execution authority",()=>{
 const branch={conversation_id:"work-parent",revision_id:"rev-saved",run_id:"run-saved",request_id:"branch-request-123456"};
 assert.equal(validateFrame({type:"branch",branch}).version,1);
 for(const change of [{conversation_id:"../private"},{revision_id:""},{request_id:"short"},{worker:"other"},{replay:[]},{approved:true},{grants:["inspect"]},{bundle_path:"/private"}]) assert.throws(()=>validateFrame({type:"branch",branch:{...branch,...change}}));
});

test("task plan actions bind exact reviews and cannot supply approval or native configuration",()=>{
 const task_plan={conversation_id:"work-one",task_id:"a".repeat(32),revision:1,digest:"b".repeat(64),action:"approve",request_id:"review-1"};
 assert.equal(validateFrame({type:"task_plan",task_plan}).version,1);
 for(const change of [{task_id:"../private"},{revision:-1},{digest:""},{approved:true},{grant:{operation:"input"}},{budget:{max_steps:10000}},{action:"execute"},{content:{}}]) assert.throws(()=>validateFrame({type:"task_plan",task_plan:{...task_plan,...change}}));
 const content={outcome:"A checked parser",steps:[{id:"step-1",text:"Implement and verify"}],sources:["docs"],effects:["read","code","checks"]};
 assert.equal(validateFrame({type:"task_plan",task_plan:{...task_plan,action:"edit",content}}).version,1);
 for(const change of [{approved:true},{sources:["../private"]},{effects:["shell"]},{steps:[{id:"step-1",text:"one"},{id:"step-1",text:"two"}]},{outcome:"🐊".repeat(1025)}]) assert.throws(()=>validateFrame({type:"task_plan",task_plan:{...task_plan,action:"edit",content:{...content,...change}}}));
 assert.equal(validateFrame({type:"task_plan",task_plan:{...task_plan,action:"inspect",revision:0,digest:""}}).version,1);
});
test("plan-first message admission is a boolean opt-in with no renderer plan grant",()=>{
 const upload={request_id:"message-1",context:"conversation:work-one",text:"Prepare a plan",files:[],plan_first:true};
 assert.equal(validateFrame({type:"upload_submit",upload}).version,1);
 for(const change of [{plan_first:"yes"},{plan:{approved:true}},{TaskPlanRequestID:"a".repeat(32)}]) assert.throws(()=>validateFrame({type:"upload_submit",upload:{...upload,...change}}));
});
