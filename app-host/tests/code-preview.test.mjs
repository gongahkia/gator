import test from "node:test";
import assert from "node:assert/strict";
import { mkdtemp, mkdir, writeFile, rm, symlink } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { createHash } from "node:crypto";
import { codePreview } from "../src/code-preview.mjs";
const hash = value => createHash("sha256").update(value).digest("hex");
async function fixture(run, text = "diff --git a/main.go b/main.go\n--- a/main.go\n+++ b/main.go\n@@ -1 +1 @@\n-old\n+new\n") {
 const root = await mkdtemp(join(tmpdir(), "gator-code-preview-")), path = join(root,"output/code/run-one-candidate-one.patch");
 const candidate = { id:"candidate-one", patch_path:"code/run-one-candidate-one.patch", sha256:hash(text), changed_paths:["main.go"], status:"verified", verification:[{ argv:["go","test","./..."],passed:true,diagnostic:"PRIVATE NOT DISPLAYED" }] };
 const manifest = JSON.stringify({run_id:"run-one",code_candidates:[candidate]});
 await mkdir(join(root,"output/code"),{recursive:true}); await writeFile(path,text);await writeFile(join(root,"manifest.json"),manifest);
 const request = {context:"conversation:chat-one",run:"run-one",candidate:"candidate-one"};
 const state = {conversation_id:"chat-one",messages:[{bundle:{Path:root,RunID:"run-one",ManifestSHA256:hash(manifest),Candidates:[{ID:"candidate-one"}]}}]};
 try {await run({root,path,text,candidate,request,state,read:(change={})=>codePreview({...request,...change},{state})});}finally{await rm(root,{recursive:true,force:true});}
}
test("code preview binds sealed candidate bytes and keeps diagnostics private",()=>fixture(async f=>{
 const view = await f.read();assert.equal(view.patch,f.text);assert.equal(view.sha256,hash(f.text));assert.equal(view.truncated,false);assert.deepEqual(view.changedPaths,["main.go"]);assert.deepEqual(view.verification,[{argv:["go","test","./..."],passed:true}]);assert.ok(!JSON.stringify(view).includes("PRIVATE"));
}));
test("wrong conversation, run, candidate and caller paths cannot select code",()=>fixture(async f=>{
 for (const change of [{context:"conversation:other"},{run:"other"},{candidate:"other"},{candidate:"../secret"},{path:"/etc/passwd"}]) await assert.rejects(f.read(change));
 await assert.rejects(codePreview(f.request,{state:f.state,current:()=>false}));
}));
test("candidate and manifest tampering fail closed",()=>fixture(async f=>{
 await writeFile(f.path,"different bytes");await assert.rejects(f.read(),/changed/);await writeFile(f.path,f.text);await writeFile(join(f.root,"manifest.json"),"{}");await assert.rejects(f.read(),/manifest has changed/);
}));
test("linked code bytes are rejected",{skip:process.platform==="win32"},()=>fixture(async f=>{
 await writeFile(join(f.root,"elsewhere.patch"),f.text);await rm(f.path);await symlink(join(f.root,"elsewhere.patch"),f.path);await assert.rejects(f.read(),/Linked/);
}));
test("authority is rechecked after reading code",()=>fixture(async f=>{
 let checks=0;await assert.rejects(codePreview(f.request,{state:f.state,current:()=>++checks===1}),/conversation changed/);assert.equal(checks,2);
}));
test("large Unicode diffs return an explicitly partial bounded preview",()=>fixture(async f=>{
 const view=await f.read();assert.equal(view.truncated,true);assert.ok(Buffer.byteLength(view.patch)<=128*1024);assert.ok(view.patch.split("\n").length<=2000);assert.ok(!view.patch.includes("�"));
},"+"+"海🦦".repeat(60000)));
