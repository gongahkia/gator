import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, rm, symlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createHash } from "node:crypto";
import { sourcePreview, sourceCatalog } from "../electron/source-preview.mjs";
const hash = text => createHash("sha256").update(text).digest("hex");
async function fixture(run) {
  const root = await mkdtemp(join(tmpdir(), "gator-source-preview-")), path = join(root, "bundle"), text = "Title\nOwner: Mina\nDeadline: Friday\nBudget: unapproved\n";
  const digest = hash(text), file = `evidence/${digest}.txt`, manifest = JSON.stringify({ run_id: "run-one", evidence: [{ locator: "source/project.md", sha256: digest, snapshot_path: file }] });
  const revision = { conversation_id: "chat-one", bundle_path: path, replay: { messages: [{ role: "user", content: "Read the project" }, { role: "tool", tool_name: "read_file", content: JSON.stringify({ ok: true, result: { path: "source/project.md", start_line: 2, end_line: 3, content: "Owner: Mina\nDeadline: Friday", truncated: false } }) }] } };
  const revisions = join(root, "gator/conversations/chat-one/revisions"), history = join(root, "gator/work-history");
  await mkdir(join(path, "evidence"), { recursive: true }); await mkdir(revisions, { recursive: true }); await mkdir(history, { recursive: true });
  await writeFile(join(path, file), text); await writeFile(join(path, "manifest.json"), manifest);
  await writeFile(join(history, "run-one.json"), JSON.stringify({ id: "run-one", conversation_id: "chat-one", revision_id: "rev-one" }));
  await writeFile(join(revisions, "rev-one.json"), JSON.stringify(revision));
  const state = { conversation_id: "chat-one", messages: [{ bundle: { Path: path, RunID: "run-one", ManifestSHA256: hash(manifest) } }] };
  const request = { context: "conversation:chat-one", run: "run-one", path: "source/project.md", start: 2, end: 3 };
  try { await run({ root, path, file, text, revision, revisions, state, request, read: value => sourcePreview(value ?? request, { stateRoot: root, state }) }); } finally { await rm(root, { recursive: true, force: true }); }
}
test("source preview shows verified retained bytes, cited lines and independently recorded read ranges", () => fixture(async f => {
  const result = await f.read(); assert.equal(result.text, f.text); assert.equal(result.sha256, hash(f.text)); assert.deepEqual(result.cited, { start: 2, end: 3 }); assert.deepEqual(result.reads, [{ start: 2, end: 3, truncated: false }]); assert.equal(result.notice, undefined);
}));
test("source references cannot open another run, conversation, unselected source or arbitrary path", () => fixture(async f => {
  for (const change of [{ context: "conversation:other" }, { run: "run-other" }, { path: "source/secret.md" }, { path: "source/../secret" }, { path: "/etc/passwd" }, { path: "source/\\secret" }, { start: 0 }, { end: 999 }, { executable: "/bin/sh" }]) await assert.rejects(f.read({ ...f.request, ...change }));
  await assert.rejects(sourcePreview(f.request, { stateRoot: f.root, state: f.state, current: () => false }));
}));
test("source tampering fails closed; no fallback to the current workspace", () => fixture(async f => {
  await writeFile(join(f.path, f.file), "Changed private data"); await assert.rejects(f.read(), /changed/);
}));
test("linked source bytes cannot bypass the retained-evidence boundary", { skip: process.platform === "win32" }, () => fixture(async f => {
  // Windows file symlink creation requires additional OS privileges; do not
  // require elevation or weaken the ordinary cross-platform tamper check.
  await rm(join(f.path, f.file)); await writeFile(join(f.root, "elsewhere.txt"), f.text); await symlink(join(f.root, "elsewhere.txt"), join(f.path, f.file)); await assert.rejects(f.read(), /Linked/);
}));
test("unverified or missing replay is explicit and never invented from a model's cited range", () => fixture(async f => {
  f.revision.replay.messages[1].content = JSON.stringify({ ok: true, result: { path: "source/project.md", start_line: 1, end_line: 3, content: "Not the selected file" } });
  await writeFile(join(f.revisions, "rev-one.json"), JSON.stringify(f.revision));
  const result = await f.read(); assert.deepEqual(result.reads, []); assert.match(result.notice, /exact read range is unavailable/);
}));
test("source authority is checked again after reading", () => fixture(async f => {
  let checks = 0; await assert.rejects(sourcePreview(f.request, { stateRoot: f.root, state: f.state, current: () => ++checks === 1 }), /conversation changed/); assert.equal(checks, 2);
}));
test("bounded tool reads retain the exact partial final line instead of claiming it was fully read", () => fixture(async f => {
  f.revision.replay.messages[1].content = JSON.stringify({ ok: true, result: { path: "source/project.md", start_line: 2, end_line: 3, content: "Owner: Mina\nDead", truncated: true } });
  await writeFile(join(f.revisions, "rev-one.json"), JSON.stringify(f.revision));
  assert.deepEqual((await f.read()).reads, [{ start: 2, end: 3, truncated: true, endColumn: 4 }]);
}));
test("large retained text is bounded without splitting UTF-8", () => fixture(async f => {
  const text = "λ🦦".repeat(12000), digest = hash(text), file = `evidence/${digest}.txt`;
  const manifest = JSON.stringify({ run_id: "run-one", evidence: [{ locator: "source/project.md", sha256: digest, snapshot_path: file }] });
  await writeFile(join(f.path, file), text); await writeFile(join(f.path, "manifest.json"), manifest); f.state.messages[0].bundle.ManifestSHA256 = hash(manifest);
  const result = await f.read({ ...f.request, start: 1, end: 1 }); assert.ok(Buffer.byteLength(result.text) <= 65536); assert.ok(!result.text.includes("�")); assert.equal(result.truncated, true);
}));

async function replaceEvidence(f, entries) {
 const manifest=JSON.stringify({run_id:"run-one",evidence:entries});
 await writeFile(join(f.path,"manifest.json"),manifest);f.state.messages[0].bundle.ManifestSHA256=hash(manifest);
}
test("citation metadata supports retained web and connected evidence without native configuration",()=>fixture(async f=>{
 const entries=[{id:"web-one",locator:"https://example.com/report",sha256:hash(f.text),snapshot_path:f.file,retrieved_at:"2026-11-01T01:00:00Z"},{id:"connected-one",locator:"connector:reports/fetch",sha256:hash(f.text),snapshot_path:f.file}];
 await replaceEvidence(f,entries);
 const result=await sourceCatalog({context:f.request.context,run:f.request.run},{state:f.state});
 assert.deepEqual(result.references.map(reference=>reference.kind),["web","connected"]);
 assert.equal(result.references[0].original_url,"https://example.com/report");assert.equal(result.references[0].captured_at,"2026-11-01T01:00:00Z");
 for(const entry of entries){const preview=await f.read({...f.request,path:`evidence:${entry.id}`});assert.equal(preview.text,f.text);assert.equal(preview.locator,entry.locator);assert.equal(preview.kind,entry.id==="web-one"?"web":"connected");}
 assert.equal(result.references[1].original_url,undefined);
 await assert.rejects(sourceCatalog({context:"conversation:other",run:"run-one"},{state:f.state}));
 await assert.rejects(f.read({...f.request,path:"evidence:invented"}));
 await assert.rejects(f.read({...f.request,path:"https://unretrieved.example/report"}));
 for(const path of ["javascript:alert(1)","https://user:password@example.com/","evidence:../../secret"]){await assert.rejects(f.read({...f.request,path}));}
}));
test("ambiguous locator never substitutes a capture; exact evidence IDs disambiguate",()=>fixture(async f=>{
 await replaceEvidence(f,[{id:"first",locator:"https://example.com/page",sha256:hash(f.text),snapshot_path:f.file},{id:"second",locator:"https://example.com/page",sha256:hash(f.text),snapshot_path:f.file}]);
 await assert.rejects(f.read({...f.request,path:"https://example.com/page"}),/multiple captures/);
 assert.equal((await f.read({...f.request,path:"evidence:second"})).id,"second");
}));
test("evidence byte-page read ranges preserve partial first and final columns",()=>fixture(async f=>{
 const text="one\nprefix λ🐊 tail\nthree\n", digest=hash(text),file=`evidence/${digest}.txt`;
 await writeFile(join(f.path,file),text);await replaceEvidence(f,[{id:"paged",locator:"https://example.com/page",sha256:digest,snapshot_path:file}]);
 f.revision.replay.messages[1]={role:"tool",tool_name:"read_evidence",content:JSON.stringify({evidence:{id:"paged",sha256:digest},text:"λ🐊",offset_bytes:Buffer.byteLength("one\nprefix "),truncated:true})};
 await writeFile(join(f.revisions,"rev-one.json"),JSON.stringify(f.revision));
 const result=await f.read({...f.request,path:"evidence:paged",start:2,end:2});
 assert.deepEqual(result.reads,[{start:2,end:2,truncated:true,startColumn:7,endColumn:10}]);
 f.revision.replay.messages[1].content=JSON.stringify({evidence:{id:"paged",sha256:digest},text:"invented read",offset_bytes:0});
 await writeFile(join(f.revisions,"rev-one.json"),JSON.stringify(f.revision));
 assert.deepEqual((await f.read({...f.request,path:"evidence:paged"})).reads,[]);
}));
test("catalogue size, disappearing source, unsupported binary/PDF capture and late revocation stay explicit",()=>fixture(async f=>{
 const entries=Array.from({length:600},(_,i)=>({id:`source-${i}`,locator:`https://example.com/${"a".repeat(600)}/${i}`,sha256:hash(f.text),snapshot_path:f.file}));
 await replaceEvidence(f,entries);
 const request={context:f.request.context,run:f.request.run},result=await sourceCatalog(request,{state:f.state});
 assert.ok(Buffer.byteLength(JSON.stringify(result))<=65536);assert.ok(result.references.length<=256);assert.equal(result.truncated,true);
 let checks=0;await assert.rejects(sourceCatalog(request,{state:f.state,current:()=>++checks===1}),/conversation changed/);
 await replaceEvidence(f,[{id:"pdf-one",locator:"attachment/report.pdf",sha256:hash(f.text)}]);
 await assert.rejects(f.read({...f.request,path:"evidence:pdf-one"}),/binary\/PDF/);
 await replaceEvidence(f,[{id:"missing",locator:"https://example.com/report",sha256:hash(f.text),snapshot_path:f.file}]);
 await rm(join(f.path,f.file));await assert.rejects(f.read({...f.request,path:"evidence:missing"}));
}));

test("normalized fetched preview is identified separately from exact raw-byte read ranges",()=>fixture(async f=>{
 const text="<html><body><p>Actual fetched content.</p></body></html>",digest=hash(text),file=`evidence/${digest}.txt`;
 await writeFile(join(f.path,file),text);await replaceEvidence(f,[{id:"html-one",locator:"https://example.com/page",sha256:digest,snapshot_path:file}]);
 f.revision.replay.messages[1]={role:"tool",tool_name:"web_fetch",content:JSON.stringify({evidence:{id:"html-one",sha256:digest},text:"Actual fetched content.",preview_format:"readable_text",truncated:false})};
 await writeFile(join(f.revisions,"rev-one.json"),JSON.stringify(f.revision));
 const result=await f.read({...f.request,path:"evidence:html-one",start:1,end:1});
 assert.equal(result.observed_preview,"Actual fetched content.");assert.deepEqual(result.reads,[]);assert.match(result.notice,/normalized preview/);
}));

test("inherited sources use only host-bound original provenance and cannot invent read ranges",()=>fixture(async f=>{
 const bundle=f.state.messages[0].bundle;
 bundle.InheritedOrigin={conversation_id:"chat-one",source_revision_id:"rev-one",revision_id:"rev-child",run_id:"run-one",sha256:hash(JSON.stringify(f.revision))};
 f.state.conversation_id="chat-child";
 const request={...f.request,context:"conversation:chat-child"};
 const read=()=>sourcePreview(request,{stateRoot:f.root,state:f.state});
 assert.deepEqual((await read()).reads,[{start:2,end:3,truncated:false}]);
 bundle.InheritedOrigin.source_revision_id="rev-other";
 assert.deepEqual((await read()).reads,[]);
 bundle.InheritedOrigin.source_revision_id="rev-one";
 await writeFile(join(f.revisions,"rev-one.json"),JSON.stringify({...f.revision,replay:{messages:[]}}));
 assert.deepEqual((await read()).reads,[]);
 assert.equal((await read()).text,f.text);
}));
