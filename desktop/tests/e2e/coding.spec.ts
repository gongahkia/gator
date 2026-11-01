import { test, expect, type Page } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { codingBase, codingFixed, codingFollowup, codingFixture, codingPatch, fixtureTool, askCoding, type FixtureRequest } from "./coding-fixture";
import { syntaxTokens } from "../../src/syntax";


function toolResults(body: FixtureRequest) { const last=body.messages.map(message=>message.role).lastIndexOf("user");return body.messages.slice(last+1).filter(message=>message.role==="tool"); }
function directStep(body: FixtureRequest, repair=false) {
 const results=toolResults(body), last=results.at(-1)?.content??"", step=results.length;
 // Real tool results determine repair; the fixture doesn't substitute success.
 if(repair && step===0)return fixtureTool("code_run_command",{argv:["npm","test"]},"I’ll run the existing tests before editing.");
 const offset=repair?1:0;
 if(step===offset)return fixtureTool("code_read_file",{path:"normalize.cjs"},repair?"The existing implementation fails its tests. I’ll inspect it before repairing it.":"I’ll inspect the code in an isolated checkout.");
 if(step===offset+1)return fixtureTool("code_apply_patch",{patch:codingPatch(codingBase,codingFixed)},"I’ll implement normalization without changing your test file.");
 if(step===offset+2)return fixtureTool("code_run_command",{argv:["npm","test"]},"I’ll check the implementation against the existing tests.");
 if(step===offset+3)return fixtureTool("code_run_command",{argv:["git","diff","--check"]});
 if(step===offset+4)return fixtureTool("code_git_status",{});
 if(step===offset+5)return fixtureTool("code_git_diff",{});
 if(step===offset+6)return fixtureTool("code_stage",{},"The checks passed; I’ll stage the exact patch for review.");
 if(!last.includes('"verified"'))throw new Error("candidate was not verified: "+JSON.stringify(results).slice(0,3500));
 return {role:"assistant",content:'Staged a verified implementation. Your original files are unchanged.\n\n```javascript\nconst tag = normalize(" Ship  NOW "); // "ship-now"\n```'};
}

for(const repair of [false,true])test(`actual coding tools ${repair?"repair failing tests":"implement files"} through Electron and expose a sealed diff`,async()=>{
 test.setTimeout(180000);
 const f=await codingFixture(body=>directStep(body,repair));try{
 const app=await f.launch(),page=await app.firstWindow();
 await askCoding(page,"Implement normalization in normalize.cjs, run tests, and stage a patch for review.");
 const state=(await page.evaluate(()=>window.gator!.snapshot())).state!,bundle=[...state.messages].reverse().find(message=>message.bundle)?.bundle;
 expect(bundle?.Status,JSON.stringify(state.messages.map(message=>({role:message.role,text:message.text,result:message.result})))).toBe("completed");expect(bundle?.Candidates?.[0].Status).toBe("verified");expect(bundle?.Candidates?.[0].ChangedPaths).toEqual(["normalize.cjs"]);
 expect(await f.original()).toBe(codingBase);
 const history=page.locator(".activity-history").last();await expect(history.getByText("I’ll implement normalization without changing your test file.")).toBeVisible();
 if(repair){await expect(history.getByText("The existing implementation fails its tests.",{exact:false})).toBeVisible();expect(JSON.stringify(state.messages)).toContain('"state":"failed"');}
 await expect(page.locator(".assistant-message .syntax-keyword").filter({hasText:"const"})).toBeVisible();
 await page.getByRole("button",{name:/Change /}).last().click();
 const changes=page.getByRole("region",{name:"Code changes",exact:true});await expect(changes.getByRole("heading",{name:"1 changed file"})).toBeVisible();
 await expect(changes.getByLabel("Retained Git diff")).toContainText('+module.exports.normalize');await expect(changes.getByText("Recorded verification")).toBeVisible();
 await expect(changes.locator(".diff-added")).toHaveCount(1);await expect(changes.locator(".diff-removed")).toHaveCount(1);
 expect(await readFile(join(f.workspace,"verify.cjs"),"utf8")).toContain('3 normalization assertions passed');
 await page.screenshot({path:test.info().outputPath(`coding-${repair?"repair":"implement"}.png`),fullPage:true});
 }finally{await f.close();}
});

test("coding syntax tokens preserve untrusted text exactly and have bounded work",()=>{
 const value='const tag = "<script>alert(1)</script>"; // untrusted\nreturn 42;';
 const tokens=syntaxTokens(value,"javascript");expect(tokens.map(token=>token.text).join("")).toBe(value);expect(tokens.filter(token=>token.kind==="keyword").map(token=>token.text)).toEqual(["const","return"]);
 expect(syntaxTokens(value,"unknown")).toEqual([{text:value}]);const huge="const x = 42;\n".repeat(100000);const limited=syntaxTokens(huge,"js");expect(limited.length).toBeLessThan(16010);expect(limited.map(token=>token.text).join("")).toBe(huge);
});

test("Electron runs an internal coding worker and an independent read-only reviewer",async()=>{
 test.setTimeout(180000);
 const f=await codingFixture(body=>{
 const names=body.tools?.map(tool=>tool.function.name)??[],results=toolResults(body);
 if(names.includes("code_apply_patch")){
 if(!results.length)return fixtureTool("complete_code",{agent:"code",objective:"Implement normalization in normalize.cjs",acceptance:["Existing normalization tests pass","Only normalize.cjs changes"],max_attempts:2},"I’ll delegate implementation and check it independently.");
 if(!results.at(-1)?.content?.includes('"status":"completed"'))throw new Error("completion did not pass");
 return {role:"assistant",content:"Implemented, verified, and independently reviewed the staged patch."};
 }
 if(names.includes("apply_patch")){
 const steps=[fixtureTool("read_file",{path:"normalize.cjs"}),fixtureTool("apply_patch",{patch:codingPatch(codingBase,codingFixed)}),fixtureTool("run_command",{argv:["npm","test"]}),fixtureTool("run_command",{argv:["git","diff","--check"]}),fixtureTool("git_status",{}),fixtureTool("git_diff",{})];
 return steps[results.length]??{role:"assistant",content:"Implemented normalization and ran the required checks."};
 }
 const prompt=[...body.messages].reverse().find(message=>message.role==="user")?.content??"";
 if(!results.length)return fixtureTool("read_file",{path:"candidate/normalize.cjs"});
 const digest=prompt.match(/"candidate_sha256":"([a-f0-9]{64})"/)?.[1];if(!digest)throw new Error("missing review digest");
 return {role:"assistant",content:JSON.stringify({passed:true,candidate_sha256:digest,findings:[]})};
 });try{
 const app=await f.launch(),page=await app.firstWindow();await askCoding(page,"Implement normalization in normalize.cjs using an internal coding worker and independent review.");
 const state=(await page.evaluate(()=>window.gator!.snapshot())).state!,bundle=[...state.messages].reverse().find(message=>message.bundle)?.bundle;
 expect(bundle?.Status,JSON.stringify(state.messages)).toBe("completed");expect(bundle?.Candidates?.at(-1)?.Status).toBe("verified");
 const {readdir}=await import("node:fs/promises"),directory=join(f.root,"state/gator/completions",bundle!.RunID!),files=(await readdir(directory)).filter(path=>path.endsWith(".json"));expect(files).toHaveLength(1);
 const record=JSON.parse(await readFile(join(directory,files[0]),"utf8"));expect(record.status).toBe("completed");expect(record.agent).toBe("code");expect(record.attempts.at(-1).review.passed).toBe(true);expect(record.attempts.at(-1).reviewed_paths).toContain("candidate/normalize.cjs");expect(await f.original()).toBe(codingBase);
 }finally{await f.close();}
});

test("same-conversation follow-up explicitly builds on the verified coding baseline",async()=>{
 test.setTimeout(180000);let phase=0,baseline="";
 const f=await codingFixture(body=>{
 if(!phase)return directStep(body);
 const results=toolResults(body),step=results.length;
 const steps=[fixtureTool("code_open",{baseline_patch:baseline}),fixtureTool("code_read_file",{path:"normalize.cjs"}),fixtureTool("code_apply_patch",{patch:codingPatch(codingFixed,codingFollowup)}),fixtureTool("code_run_command",{argv:["npm","test"]}),fixtureTool("code_run_command",{argv:["git","diff","--check"]}),fixtureTool("code_git_status",{}),fixtureTool("code_git_diff",{}),fixtureTool("code_stage",{})];
 if(step>=steps.length && !results.at(-1)?.content?.includes('"verified"'))throw new Error("follow-up did not verify: "+JSON.stringify(results).slice(0,3500));
 return steps[step]??{role:"assistant",content:"Extended the previous implementation and checked the cumulative patch."};
 });try{
 const app=await f.launch(),page=await app.firstWindow();await askCoding(page,"Implement normalization in normalize.cjs and stage the patch.");
 const first=(await page.evaluate(()=>window.gator!.snapshot())).state!,bundle=[...first.messages].reverse().find(message=>message.bundle)?.bundle;baseline=bundle!.Candidates![0].PatchPath!;
 phase=1;await askCoding(page,"Extend the retained implementation to normalize underscores too; preserve the earlier behavior.");
 const second=(await page.evaluate(()=>window.gator!.snapshot())).state!,next=[...second.messages].reverse().find(message=>message.bundle)?.bundle;expect(second.conversation_id).toBe(first.conversation_id);expect(next?.Status,JSON.stringify(second.messages)).toBe("completed");
 const preview=await page.evaluate(async({conversation,run,candidate})=>window.gator!.codePreview({context:`conversation:${conversation}`,run,candidate}),{conversation:second.conversation_id,run:next!.RunID!,candidate:next!.Candidates![0].ID});
 expect(preview.patch).toContain(codingFollowup.trim());expect(preview.patch).toContain('-'+codingBase.trim());expect(await f.original()).toBe(codingBase);
 }finally{await f.close();}
});

test("explicit inspect assignments cannot acquire coding mutation tools",async()=>{
 let attempt=0;
 const f=await codingFixture(body=>{
 expect(body.tools?.some(tool=>tool.function.name.startsWith("code_"))).toBe(false);
 return attempt++===0?fixtureTool("code_apply_patch",{patch:codingPatch(codingBase,codingFixed)}):{role:"assistant",content:"Inspection cannot modify code."};
 });try{
 const app=await f.launch(),page=await app.firstWindow();await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toBeEnabled();await page.evaluate(()=>window.gator!.send({type:"submit",text:"/mode inspect"}));
 await askCoding(page,"Inspect the current normalization implementation.");expect(await f.original()).toBe(codingBase);
 const state=(await page.evaluate(()=>window.gator!.snapshot())).state!;expect(state.messages.flatMap(message=>message.bundle?.Candidates??[])).toHaveLength(0);
 }finally{await f.close();}
});

test("cancelling while a coding command awaits approval never executes it",async()=>{
 const f=await codingFixture(()=>fixtureTool("code_run_command",{argv:["node","-e",'require("node:fs").writeFileSync("not-authorized.txt","should never run")']},"This exploratory command requires your approval."));try{
 const app=await f.launch(),page=await app.firstWindow(),composer=page.getByRole("textbox",{name:"Message Gator",exact:true});await expect(composer).toBeEnabled();await composer.fill("Implement normalization, but wait for approval before exploratory commands.");await page.getByRole("button",{name:"Send message",exact:true}).click();
 await expect(page.getByRole("button",{name:"Approve this action",exact:true})).toBeVisible();
 await page.getByRole("button",{name:"Stop current work",exact:true}).click();await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state!.running).toBe(false);
 expect(await f.original()).toBe(codingBase);expect(f.calls()).toBe(1);
 const {readdir}=await import("node:fs/promises");const walk=async(root:string):Promise<string[]>=>{const entries=await readdir(root,{withFileTypes:true});return(await Promise.all(entries.map(entry=>entry.isDirectory()?walk(join(root,entry.name)):[entry.name]))).flat();};expect(await walk(join(f.root,"state"))).not.toContain("not-authorized.txt");
 }finally{await f.close();}
});
