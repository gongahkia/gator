import { test } from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { once } from "node:events";
import { Backend } from "../src/backend.mjs";

test("host streams Go state and closes its child on exit", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-desktop-bridge-test-"));
  const executable = join(directory, "fixture.cjs");
  await writeFile(executable, `#!/usr/bin/env node\nconst readline = require('node:readline');\nconst send = (text) => process.stdout.write(JSON.stringify({ version: 1, type: 'state', state: { title: text, messages: [] } }) + '\\n');\nsend('🐊 connected');\nreadline.createInterface({ input: process.stdin }).on('line', line => { const frame = JSON.parse(line); if (frame.type === 'exit') process.exit(0); send(frame.text); });\n`, { mode: 0o700 });
  const backend = new Backend(process.execPath, directory, process.env, [executable]);
  try {
    const [initial] = await once(backend, "state");
    assert.equal(initial.title, "🐊 connected");
    const next = once(backend, "state");
    backend.send({ type: "submit", text: "next turn" });
    assert.equal((await next)[0].title, "next turn");
    assert.throws(() => backend.send({ type: "external_done" }));
    assert.throws(() => backend.send({ type: "key", key: "ctrl+g", text: "No renderer launch" }));
    await backend.stop();
    assert.equal(backend.closed, true);
    assert.throws(() => backend.send({ type: "submit", text: "too late" }));
  } finally { await backend.stop(); await rm(directory, { recursive: true, force: true }); }
});

test("restart preparation waits for the canonical application and never forces a busy child to exit", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-update-bridge-"));
  const executable = join(directory, "fixture.cjs");
  await writeFile(executable, `const readline = require('node:readline'); let busy = true; process.stdout.write(JSON.stringify({version:1,type:'state',state:{running:false,messages:[]}})+'\\n'); readline.createInterface({input:process.stdin}).on('line', line => { const f=JSON.parse(line); if(f.type==='prepare_update'){ process.stdout.write(JSON.stringify({version:1,type:'update_ready',nonce:f.nonce,ready:!busy,reason:'Actual work is busy'})+'\\n'); if(!busy) process.exit(0); } else if(f.type==='submit'){busy=false; process.stdout.write(JSON.stringify({version:1,type:'state',state:{title:'idle',messages:[]}})+'\\n');} else if(f.type==='exit') process.exit(0); });`);
  const backend = new Backend(process.execPath, directory, process.env, [executable]);
  try {
    await once(backend, "state");
    await assert.rejects(backend.prepareUpdate(), /Actual work is busy/);
    assert.equal(backend.closed, false);
    const idle = once(backend, "state"); backend.send({ type: "submit", text: "finish" }); await idle;
    await backend.prepareUpdate(); assert.equal(backend.closed, true);
  } finally { await backend.stop(); await rm(directory, { recursive: true, force: true }); }
});

test("dialogue reaction metadata preserves the legacy answered reply and exact turn", async () => {
  const directory = await mkdtemp(join(tmpdir(), "gator-reaction-protocol-"));
  const executable = join(directory, "fixture.cjs");
  const turn = { id: "turn-fixture", revision: 3, sequence: 1, text: "Thanks!", response: "You're welcome.", state: "answered",
    reaction: { version: 1, conversation_id: "chat-fixture", turn_id: "turn-fixture", revision: 3, kind: "appreciation", emoji: "❤️", level: "subtle" } };
  await writeFile(executable, `const readline=require('node:readline');process.stdout.write(JSON.stringify({version:1,type:'state',state:{conversation_id:'chat-fixture',messages:[],dialogue:[${JSON.stringify(turn)}]}})+'\\n');readline.createInterface({input:process.stdin}).on('line',line=>{if(JSON.parse(line).type==='exit')process.exit(0)});`);
  const backend = new Backend(process.execPath, directory, process.env, [executable]);
  try {
    const [state] = await once(backend, "state");
    assert.deepEqual(state.dialogue[0], turn);
    assert.equal(state.dialogue[0].response, "You're welcome."); // older renderers use this unchanged fallback
    assert.equal(state.dialogue[0].state, "answered");
    assert.equal(state.dialogue[0].reaction.turn_id, state.dialogue[0].id);
    assert.equal(state.dialogue_dispatch, undefined);
    assert.equal(state.pending_approval, undefined);
  } finally { await backend.stop(); await rm(directory, { recursive: true, force: true }); }
});
