import { test, expect } from "@playwright/test";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { codingFixture, fixtureTool, askCoding, type FixtureRequest } from "./coding-fixture";
import { closeDesktop } from "./lifecycle";

function turn(body:FixtureRequest){const last=body.messages.map(message=>message.role).lastIndexOf("user");return {question:body.messages[last]?.content??"",results:body.messages.slice(last+1).filter(message=>message.role==="tool")};}
const answerOne="First retained answer. Source: `source/normalize.cjs`, line 1.";
const answerTwo="Later parent answer must stay in the parent.";
function respond(body:FixtureRequest){const current=turn(body);if(!current.results.length)return fixtureTool("read_file",{path:"source/normalize.cjs"});return {role:"assistant",content:current.question.includes("First")?answerOne:answerTwo};}
async function inspectMode(page:import("@playwright/test").Page){await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toBeEnabled();await page.evaluate(()=>window.gator!.send({type:"submit",text:"/mode inspect"}));await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.work_details?.mode).toBe("inspect");}

test("new-chat branch preserves the exact saved boundary, sources, ancestry and independent drafts",async()=>{
 test.setTimeout(120000);
 const f=await codingFixture(respond);try{
 let app=await f.launch(),page=await app.firstWindow();await inspectMode(page);
 await askCoding(page,"First: read the source file and summarize it.");await askCoding(page,"Second: read the source again.");
 const before=(await page.evaluate(()=>window.gator!.snapshot())).state!,parent=before.conversation_id!;
 const metadata=join(f.root,"state/gator/conversations",parent,"conversation.json"),parentRaw=await readFile(metadata,"utf8");
 const composer=page.getByRole("textbox",{name:"Message Gator",exact:true});await composer.fill("Parent draft stays here");
 const first=page.getByRole("article",{name:"Gator message",exact:true}).filter({hasText:"First retained answer."});
 await first.getByRole("button",{name:"Response actions",exact:true}).click();
 await page.getByRole("menuitem",{name:"Branch in new chat",exact:true}).click();
 const review=page.getByRole("dialog",{name:"Branch in new chat",exact:true});await expect(review).toContainText("pending approvals, queues, workers, grants and schedules are not copied");
 expect(f.calls()).toBe(4);
 await review.getByRole("button",{name:"Create branch",exact:true}).click();
 await expect.poll(async()=>{const snapshot=await page.evaluate(()=>window.gator!.snapshot());return !!snapshot.state?.conversation_id&&snapshot.state.conversation_id!==parent&&!snapshot.sessionSwitching;}).toBe(true);
 const child=(await page.evaluate(()=>window.gator!.snapshot())).state!;
 expect(child.running).toBe(false);expect(child.pending_approval).toBeUndefined();expect(child.queued).toBe(0);
 expect(child.messages.filter(message=>message.role==="Gator").map(message=>message.text)).toEqual([answerOne]);
 expect(child.sources?.selected).toEqual([]);expect(f.calls()).toBe(4);
 await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toHaveValue("");
 expect(await readFile(metadata,"utf8")).toBe(parentRaw);
 await page.getByRole("button",{name:"source/normalize.cjs",exact:true}).first().click();
 await expect(page.getByLabel("Retained file contents",{exact:true})).toContainText("module.exports.normalize");
 await expect(page.locator('.source-lines > div').first()).toHaveAttribute("data-read","true");
 await page.getByRole("button",{name:"Close source preview",exact:true}).click();
 await page.getByRole("textbox",{name:"Message Gator",exact:true}).fill("Child draft only");
 await page.getByRole("button",{name:"Open original chat",exact:true}).click();
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.conversation_id).toBe(parent);
 await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toHaveValue("Parent draft stays here");
 await expect(page.getByText(answerTwo,{exact:true})).toBeVisible();
 await page.evaluate(id=>window.gator!.openSession(id),child.conversation_id!);
 await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toHaveValue("Child draft only");
 await closeDesktop(app);app=await f.launch();page=await app.firstWindow();
 await expect(page.getByRole("button",{name:"Open original chat",exact:true})).toBeVisible();
 expect((await page.evaluate(()=>window.gator!.snapshot())).state?.conversation_id).toBe(child.conversation_id);
 await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toHaveValue("Child draft only");
 expect(f.calls()).toBe(4);
 await page.setViewportSize({width:820,height:740});await page.emulateMedia({reducedMotion:"reduce",colorScheme:"light"});
 await page.getByRole("article",{name:"Gator message",exact:true}).getByRole("button",{name:"Response actions",exact:true}).click();
 await page.getByRole("menuitem",{name:"Branch in new chat",exact:true}).click();
 const box=await page.getByRole("dialog",{name:"Branch in new chat",exact:true}).boundingBox();expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.x+box!.width).toBeLessThanOrEqual(820);
 await page.screenshot({path:test.info().outputPath("branch-review-narrow.png"),fullPage:true});
 await page.keyboard.press("Escape");await expect(page.getByRole("dialog",{name:"Branch in new chat",exact:true})).toHaveCount(0);
 }finally{await f.close();}
});

test("per-message local speech reads only the chosen reply, stops, and reports unavailable voices",async()=>{
 const f=await codingFixture(respond);try{
 const app=await f.launch(),page=await app.firstWindow();await inspectMode(page);
 await askCoding(page,"First: read the source.");await askCoding(page,"Second: read the source.");
 const parent=(await page.evaluate(()=>window.gator!.snapshot())).state!.conversation_id!;
 await page.evaluate(()=>{
  const view=window as unknown as {spoken:string[];cancelled:number;voicesAvailable:boolean};view.spoken=[];view.cancelled=0;view.voicesAvailable=true;
  Object.defineProperty(window,"SpeechSynthesisUtterance",{configurable:true,value:class{constructor(public text:string){}}});
  Object.defineProperty(window,"speechSynthesis",{configurable:true,value:{getVoices:()=>view.voicesAvailable?[{name:"Local fixture",voiceURI:"local-fixture",lang:"en-US",localService:true}]:[],cancel:()=>view.cancelled++,speak:(utterance:{text:string})=>view.spoken.push(utterance.text)}});
 });
 const first=page.getByRole("article",{name:"Gator message",exact:true}).filter({hasText:"First retained answer."});
 await first.getByRole("button",{name:"Response actions",exact:true}).click();await page.getByRole("menuitem",{name:"Read aloud",exact:true}).click();
 await expect(first.getByRole("button",{name:"Stop reading",exact:true})).toBeVisible();
 expect(await page.evaluate(()=>(window as unknown as {spoken:string[]}).spoken)).toEqual(["First retained answer. Source:  source/normalize.cjs , line 1."]);
 expect(f.calls()).toBe(4);
 await first.getByRole("button",{name:"Stop reading",exact:true}).click();await expect(first.getByRole("button",{name:"Stop reading",exact:true})).toHaveCount(0);
 await first.getByRole("button",{name:"Response actions",exact:true}).click();await page.getByRole("menuitem",{name:"Read aloud",exact:true}).click();
 await page.getByRole("button",{name:"New chat",exact:true}).click();
 await expect.poll(async()=>{const snapshot=await page.evaluate(()=>window.gator!.snapshot());return !!snapshot.state?.conversation_id&&snapshot.state.conversation_id!==parent&&!snapshot.sessionSwitching;}).toBe(true);
 await expect(page.getByRole("button",{name:"Stop speaking",exact:true})).toHaveCount(0);
 expect(await page.evaluate(()=>(window as unknown as {cancelled:number}).cancelled)).toBeGreaterThanOrEqual(3);
 // Return through existing navigation; the old response is never auto-spoken.
 await page.evaluate(id=>window.gator!.openSession(id),parent);
 await page.evaluate(()=>{(window as unknown as {voicesAvailable:boolean}).voicesAvailable=false;});
 await first.getByRole("button",{name:"Response actions",exact:true}).click();
 await expect(page.getByRole("menuitem",{name:"Read aloud",exact:true})).toBeDisabled();
 await expect(page.getByText("No local system voice is available.",{exact:true})).toBeVisible();
 expect(f.calls()).toBe(4);
 }finally{await f.close();}
});

test("branching from an older answer leaves the actual parent worker running",async()=>{
 test.setTimeout(120000);
 let release=()=>{};const gate=new Promise<void>(done=>release=done);
 const f=await codingFixture(async body=>{if(turn(body).question.startsWith("Hold"))await gate;return respond(body);});try{
 const app=await f.launch(),page=await app.firstWindow();await inspectMode(page);await askCoding(page,"First: read the source.");
 const before=(await page.evaluate(()=>window.gator!.snapshot())).state!,parent=before.conversation_id!;
 const composer=page.getByRole("textbox",{name:"Message Gator",exact:true});await composer.fill("Hold the second parent request until released.");await page.getByRole("button",{name:"Send message",exact:true}).click();
 await expect.poll(()=>f.calls()).toBe(3);
 const original=page.getByRole("article",{name:"Gator message",exact:true}).filter({hasText:"First retained answer."});
 await original.getByRole("button",{name:"Response actions",exact:true}).click();await page.getByRole("menuitem",{name:"Branch in new chat",exact:true}).click();
 await page.getByRole("dialog",{name:"Branch in new chat",exact:true}).getByRole("button",{name:"Create branch",exact:true}).click();
 await expect.poll(async()=>{const snapshot=await page.evaluate(()=>window.gator!.snapshot());return !!snapshot.state?.conversation_id&&snapshot.state.conversation_id!==parent&&!snapshot.sessionSwitching;}).toBe(true);
 const child=(await page.evaluate(()=>window.gator!.snapshot())).state!;expect(child.running).toBe(false);expect(f.calls()).toBe(3);
 await page.getByRole("button",{name:"Open original chat",exact:true}).click();
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.conversation_id).toBe(parent);
 expect((await page.evaluate(()=>window.gator!.snapshot())).state!.running).toBe(true);expect(f.calls()).toBe(3);
 await page.evaluate(id=>window.gator!.openSession(id),child.conversation_id!);
 release();await expect.poll(()=>f.calls()).toBe(4);
 await page.getByRole("button",{name:"Open original chat",exact:true}).click();
 await expect(page.getByText(answerTwo,{exact:true})).toBeVisible();
 expect((await page.evaluate(()=>window.gator!.snapshot())).state!.running).toBe(false);
 expect(f.calls()).toBe(4);
 }finally{release();await f.close();}
});
