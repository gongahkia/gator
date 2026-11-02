import { test, expect, type Page } from "@playwright/test";
import { join } from "node:path";
import { codingFixture, fixtureTool, type FixtureRequest } from "./coding-fixture";
import { closeDesktop } from "./lifecycle";

const proposal = {outcome:"Explain the retained normalization source",steps:[{id:"read-source",text:"Read the selected source file"},{id:"explain",text:"Explain the observed implementation"}],sources:[],effects:["read"]};
function respond(body:FixtureRequest){
 if(!body.tools?.length)return {role:"assistant",content:JSON.stringify(proposal)};
 const last=body.messages.map(message=>message.role).lastIndexOf("user");
 if(!body.messages.slice(last+1).some(message=>message.role==="tool"))return fixtureTool("read_file",{path:"source/normalize.cjs"});
 return {role:"assistant",content:"The retained implementation returns the input unchanged. Source: `source/normalize.cjs`, line 1."};
}
async function inspectMode(page:Page){
 await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toBeEnabled();
 await page.evaluate(()=>window.gator!.send({type:"submit",text:"/mode inspect"}));
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.work_details?.mode).toBe("inspect");
}
async function prepare(page:Page,text="Read normalize.cjs and explain what it currently does."){
 await page.getByRole("button",{name:"Plan first",exact:true}).click();
 await expect(page.getByRole("button",{name:"Plan first",exact:true})).toHaveAttribute("aria-pressed","true");
 await page.getByRole("textbox",{name:"Message Gator",exact:true}).fill(text);
 await page.getByRole("button",{name:"Send message",exact:true}).click();
 await expect(page.getByRole("button",{name:"Review plan",exact:true})).toBeEnabled();
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.running).toBe(false);
}

test("plan-first pauses, edits exact reviews, rejects stale approval, and links actual activity without replay",async()=>{
 const fixture=await codingFixture(respond);try{
 const app=await fixture.launch(),page=await app.firstWindow();await inspectMode(page);await prepare(page);
 expect(fixture.calls()).toBe(1);
 const before=(await page.evaluate(()=>window.gator!.snapshot())).state!,plan=before.task_plans![0];
 expect(plan.state).toBe("queued");expect(plan.phase).toBe("review");expect(before.pending_approval).toBeUndefined();
 await page.getByRole("button",{name:"Review plan",exact:true}).click();
 let review=page.getByRole("dialog",{name:"Review task plan",exact:true});
 await expect(review).toContainText("Read normalize.cjs and explain what it currently does.");
 await expect(review).toContainText("Total cost and specialist count are not estimated");
 const steps=review.getByRole("list",{name:"Proposed steps",exact:true});
 await expect(steps.getByRole("listitem")).toHaveCount(2);
 expect(await steps.evaluate(element=>getComputedStyle(element).listStyleType)).toBe("decimal");
 const cardSteps=page.getByRole("region",{name:"Reviewed task plans",exact:true}).getByRole("list",{name:"Proposed steps",exact:true});
 expect(await cardSteps.evaluate(element=>getComputedStyle(element).listStyleType)).toBe("decimal");
 await review.getByRole("button",{name:"Edit plan",exact:true}).click();
 await page.screenshot({path:test.info().outputPath("plan-edit-dark.png"),fullPage:true});
 await review.getByLabel("Intended outcome",{exact:true}).fill("Explain the observed implementation with exact source evidence");
 await review.getByRole("textbox",{name:"Step 2",exact:true}).fill("Explain only the implementation actually read");
 await review.getByRole("button",{name:"Save revision for review",exact:true}).click();await expect(review).toHaveCount(0);
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.task_plans?.[0].revision).toBe(2);
 await page.evaluate(async old=>window.gator!.send({type:"task_plan",task_plan:{conversation_id:old.conversation!,task_id:old.plan.id,revision:old.plan.revision,digest:old.plan.digest,action:"approve",request_id:"stale-ui-review"}}),{conversation:before.conversation_id,plan});
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.task_plan_result?.error).toBeTruthy();expect(fixture.calls()).toBe(1);
 await page.getByRole("button",{name:"Review plan",exact:true}).click();review=page.getByRole("dialog",{name:"Review task plan",exact:true});
 await expect(review).toContainText("Explain only the implementation actually read");
 await expect(review.getByRole("button",{name:"Approve plan & start",exact:true})).toBeEnabled();
 await review.getByRole("button",{name:"Approve plan & start",exact:true}).click();await expect(review).toHaveCount(0);
 await expect.poll(async()=>{const state=(await page.evaluate(()=>window.gator!.snapshot())).state;return !state?.running&&state?.task_plans?.[0].state==="completed";}).toBe(true);
 expect(fixture.calls()).toBe(3);
 await expect(page.getByText("Work ended · inspect results · revision 2",{exact:true})).toBeVisible();
 // Completed plan cards collapse; expand the retained record before inspecting its run.
 await page.locator(".task-plan-card > summary").click();
 await page.getByRole("button",{name:"Inspect run activity & results",exact:true}).click();
 await expect(page.getByRole("article",{name:"Gator message",exact:true}).filter({hasText:"returns the input unchanged"})).toBeVisible();
 const after=(await page.evaluate(()=>window.gator!.snapshot())).state!,completed=after.task_plans![0];
 await page.evaluate(async value=>window.gator!.send({type:"task_plan",task_plan:{conversation_id:value.conversation!,task_id:value.plan.id,revision:value.plan.revision,digest:value.plan.digest,action:"approve",request_id:"duplicate-completed-plan"}}),{conversation:after.conversation_id,plan:completed});
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.task_plan_result?.request_id).toBe("duplicate-completed-plan");
 expect(fixture.calls()).toBe(3);
 }finally{await fixture.close();}
});

test("an undispatched plan survives restart for explicit review and can be cancelled without model execution",async()=>{
 const fixture=await codingFixture(respond);try{
 let app=await fixture.launch(),page=await app.firstWindow();await inspectMode(page);await prepare(page);
 const before=(await page.evaluate(()=>window.gator!.snapshot())).state!;
 await closeDesktop(app);app=await fixture.launch();page=await app.firstWindow();
 await expect(page.getByRole("button",{name:"Review plan",exact:true})).toBeVisible();
 expect((await page.evaluate(()=>window.gator!.snapshot())).state?.conversation_id).toBe(before.conversation_id);expect(fixture.calls()).toBe(1);
 await page.evaluate(()=>window.gator!.send({type:"submit",text:"/theme light"}));
 await expect(page.locator(".app-shell")).toHaveAttribute("data-theme","light");
 await page.getByRole("button",{name:"Review plan",exact:true}).click();
 let review=page.getByRole("dialog",{name:"Review task plan",exact:true});
 await expect(review.getByRole("button",{name:"Approve plan & start",exact:true})).toBeEnabled();
 await page.setViewportSize({width:820,height:740});await page.emulateMedia({reducedMotion:"reduce",colorScheme:"light"});
 const box=await review.boundingBox();expect(box!.x).toBeGreaterThanOrEqual(0);expect(box!.x+box!.width).toBeLessThanOrEqual(820);
 await page.screenshot({path:test.info().outputPath("plan-review-light-narrow.png"),fullPage:true});
 await page.keyboard.press("Escape");await expect(review).toHaveCount(0);
 await expect(page.getByRole("button",{name:"Review plan",exact:true})).toBeFocused();
 await page.getByRole("button",{name:"Review plan",exact:true}).click();review=page.getByRole("dialog",{name:"Review task plan",exact:true});
 await review.getByRole("button",{name:"Cancel plan",exact:true}).click();await expect(review).toHaveCount(0);
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.task_plans?.[0].state).toBe("discarded");
 expect(fixture.calls()).toBe(1);
 await page.getByRole("button",{name:"New chat",exact:true}).click();
 await expect.poll(async()=>{const snapshot=await page.evaluate(()=>window.gator!.snapshot());return snapshot.state?.conversation_id!==before.conversation_id&&!snapshot.sessionSwitching;}).toBe(true);
 await expect(page.getByRole("region",{name:"Reviewed task plans",exact:true})).toHaveCount(0);
 await expect(page.getByRole("button",{name:"Plan first",exact:true})).toHaveAttribute("aria-pressed","false");
 }finally{await fixture.close();}
});

test("out-of-scope model effects stop before mutation and are never shown as completed plan steps",async()=>{
 let implementationCalls=0;
 const fixture=await codingFixture(body=>{
  if(!body.tools?.length)return {role:"assistant",content:JSON.stringify(proposal)};
  implementationCalls++;
  return fixtureTool("write_artifact",{path:"outside.md",content:"This must not be written"});
 });try{
 const app=await fixture.launch(),page=await app.firstWindow();await inspectMode(page);
 await page.evaluate(()=>window.gator!.send({type:"submit",text:"/mode draft"}));
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.work_details?.mode).toBe("draft");
 await prepare(page);
 await page.getByRole("button",{name:"Review plan",exact:true}).click();
 await page.getByRole("dialog",{name:"Review task plan",exact:true}).getByRole("button",{name:"Approve plan & start",exact:true}).click();
 await expect.poll(async()=>{const state=(await page.evaluate(()=>window.gator!.snapshot())).state;return !state?.running&&state?.task_plans?.[0].state==="failed";}).toBe(true);
 expect(implementationCalls).toBe(1);expect(fixture.calls()).toBe(2);
 await expect(page.getByText("Work ended · results may be partial · revision 1",{exact:true})).toBeVisible();
 await expect(page.getByLabel("Reviewed task plans")).toContainText("Proposed steps are intentions, not verified completion");
 await expect(page.getByRole("article",{name:"Gator message",exact:true}).filter({hasText:"reviewed plan does not include draft"})).toBeVisible();
 }finally{await fixture.close();}
});


test("plan approval leaves the exact coding-command review pending and Stop prevents dispatch",async()=>{
 const fixture=await codingFixture(body=>{
  if(!body.tools?.length)return {role:"assistant",content:JSON.stringify({outcome:"Review an exploratory coding command",steps:[{id:"review-command",text:"Request the exact command review before proceeding"}],sources:[],effects:["read","code","checks"]})};
  return fixtureTool("code_run_command",{argv:["node","-e",'require("node:fs").writeFileSync("not-authorized.txt","must remain absent")']},"This exact exploratory command still requires your decision.");
 });try{
 const app=await fixture.launch(),page=await app.firstWindow();
 await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toBeEnabled();
 await prepare(page,"Implement normalization in the isolated code workspace, but wait for the exact exploratory command decision.");
 expect(fixture.calls()).toBe(1);
 await page.getByRole("button",{name:"Review plan",exact:true}).click();
 await page.getByRole("dialog",{name:"Review task plan",exact:true}).getByRole("button",{name:"Approve plan & start",exact:true}).click();
 await expect(page.getByRole("button",{name:"Approve this action",exact:true})).toBeVisible();
 const pending=(await page.evaluate(()=>window.gator!.snapshot())).state!;
 expect(pending.pending_approval?.token).toBeTruthy();expect(pending.task_plans?.[0].state).toBe("running");
 expect(pending.guidance_id).toBeUndefined();
 expect(JSON.stringify(pending.messages.find(message=>message.result?.kind==="approval")?.result)).toContain("not-authorized.txt");
 expect(fixture.calls()).toBe(2);
 await page.getByRole("button",{name:"Stop current work",exact:true}).click();
 await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).state?.running).toBe(false);
 await expect(page.getByRole("button",{name:"Approve this action",exact:true})).toHaveCount(0);
 expect(fixture.calls()).toBe(2);
 const {readdir}=await import("node:fs/promises");
 const walk=async(root:string):Promise<string[]>=>{const entries=await readdir(root,{withFileTypes:true});return(await Promise.all(entries.map(entry=>entry.isDirectory()?walk(join(root,entry.name)):[entry.name]))).flat();};
 expect(await walk(fixture.root)).not.toContain("not-authorized.txt");
 }finally{await fixture.close();}
});
