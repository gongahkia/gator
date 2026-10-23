import { _electron as electron, test, expect, type Page } from "@playwright/test";
import { createServer } from "node:http";
import { mkdir, readFile, writeFile, rm, readdir } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, temporaryDirectory } from "./lifecycle";
const state = (page: Page) => page.evaluate(async () => (await window.gator!.snapshot()).state!);

test("chat reviews, saves, edits and pauses an origin-linked responsibility", async () => {
 test.setTimeout(120000);
 const directory = await temporaryDirectory(join(tmpdir(), "gator-followup-chat-"));
 const workspace = join(directory,"workspace"), configRoot = join(directory,"config/gator");
 await mkdir(workspace); await mkdir(configRoot,{recursive:true});
 const expires = new Date(Date.now()+48*3600_000).toISOString();
 const server = createServer((req,res) => {
  let body=""; req.on("data", chunk => body+=chunk);
  req.on("end", () => {
   const payload=JSON.parse(body), last=payload.messages.at(-1);
   const user=[...payload.messages].reverse().find(message=>message.role==="user")?.content||"";
   let message: unknown;
   const tool=(name:string,args:unknown)=>({role:"assistant",tool_calls:[{id:name,type:"function",function:{name,arguments:JSON.stringify(args)}}]});
   if(payload.messages.some((message: {role:string;content?:string})=>message.role==="system"&&message.content?.includes("Classify the user's immediate request"))) message={role:"assistant",content:'{"mode":"inspect"}'};
   else if(last.role!=="tool") message=tool("gator_app_inspect",{topic:"followups"});
   else if(last.tool_call_id==="gator_app_inspect") {
    const current=JSON.parse(last.content).checks?.[0];
    let value:unknown={action:"create",title:"Venue replies",outcome:"Track venue availability",alert_when:"A price or availability changes",interval_minutes:15,ends_at:expires,mode:"inspect"};
    if(user.includes("Only")) value={action:"edit",id:current.id,expected_digest:current.expected_digest,alert_when:"Only a price changes"};
    if(user.includes("Pause")) value={action:"pause",id:current.id,expected_digest:current.expected_digest};
    message=tool("gator_app_update",{setting:"followup",value});
   } else { const result=JSON.parse(last.content); message={role:"assistant",content:result.status==="declined"?"No follow-up was saved.":"Your agreement is saved. The local background engine is offline."}; }
   res.writeHead(200,{"Content-Type":"application/json"});res.end(JSON.stringify({choices:[{message,finish_reason:(message as {tool_calls?:unknown}).tool_calls?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:5,total_tokens:15}}));
  });
 });
 await new Promise<void>(done=>server.listen(0,"127.0.0.1",done));
 const port=(server.address() as {port:number}).port;
 await writeFile(join(configRoot,"config.json"),JSON.stringify({version:4,defaults:{provider:"followup-fixture",model:"fixture"},custom_providers:[{id:"followup-fixture",base_url:`http://127.0.0.1:${port}/v1/chat/completions`,models:["fixture"],default_model:"fixture"}]}),{mode:0o600});
 const env=Object.fromEntries(Object.entries({...process.env,GATOR_INTERNAL_COMMANDS:"",GATOR_STATE_DIR:join(directory,"state"),GATOR_CONFIG_DIR:join(directory,"config"),GATOR_DESKTOP_USER_DATA:join(directory,"desktop"),GATOR_DESKTOP_WORKSPACE:workspace}).filter((entry):entry is [string,string]=>typeof entry[1]==="string"));
 const executablePath=process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
 const app=await electron.launch({executablePath,args:executablePath?[]:["."],cwd:resolve("."),env});
 const definitions=join(directory,"state/gator/jobs/definitions");
 const saved=async()=>Promise.all((await readdir(definitions)).filter(name=>name.endsWith(".json")).map(async name=>JSON.parse(await readFile(join(definitions,name),"utf8"))));
 try {
  const page=await app.firstWindow();await expect.poll(async()=>Boolean(await state(page))).toBe(true);
  const ask=async(text:string)=>{const box=page.getByRole("textbox",{name:"Message Gator",exact:true});await box.fill(text);await box.press("Enter");await expect(page.getByRole("button",{name:"Approve this action",exact:true})).toBeVisible();};
  await ask("Watch venue replies until the agreed end date.");
  const review=page.getByRole("region",{name:"Review follow-up",exact:true}).last();
  await expect(review).toContainText("Venue replies");await expect(review).toContainText("Offline");await expect(review).toContainText("Ends");
  expect(await saved()).toHaveLength(0);
  await page.getByRole("button",{name:"Deny",exact:true}).click();await expect.poll(async()=>(await state(page)).running).toBe(false);expect(await saved()).toHaveLength(0);
  await ask("Watch venue replies until the agreed end date.");
  await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].setSize(980,780));
  await review.evaluate(el=>{const scroller=el.closest(".conversation-scroll")!;scroller.scrollTop+=el.getBoundingClientRect().top-scroller.getBoundingClientRect().top;});await page.screenshot({path:"test-results/followup-chat-review.png"});
  await page.getByRole("button",{name:"Approve this action",exact:true}).click();await expect.poll(async()=>(await state(page)).running).toBe(false);
  const original=(await saved())[0],chatID=(await state(page)).conversation_id;
  expect(original.origin_conversation_id).toBe(chatID);expect(new Date(original.ends_at).getTime()).toBe(new Date(expires).getTime());expect(original.mode).toBe("inspect");
  await ask("Only bother me if the price changes.");await page.getByRole("button",{name:"Approve this action",exact:true}).click();await expect.poll(async()=>(await state(page)).running).toBe(false);
  expect((await saved())[0].alert_when).toBe("Only a price changes");expect((await saved())[0].origin_conversation_id).toBe(chatID);
  await ask("Pause that follow-up.");await page.getByRole("button",{name:"Approve this action",exact:true}).click();await expect.poll(async()=>(await state(page)).running).toBe(false);
  expect((await saved())[0].enabled).toBe(false);expect(await saved()).toHaveLength(1);
  await page.evaluate(()=>window.gator!.send({type:"navigate",text:"/followup"}));
  await expect(page.getByText("Venue replies",{exact:true}).first()).toBeVisible();expect((await state(page)).panel?.rows?.[0].state).toBe("paused");
  // Retain completed quiet/unchanged/failed checks using this reviewed fixture agreement.
  const checkedAt=new Date(Date.now()-300000).toISOString();
  const attempts=[{id:`attempt-${"a".repeat(24)}`,status:"completed"},{id:`attempt-${"b".repeat(24)}`,status:"unchanged"},{id:`attempt-${"c".repeat(24)}`,status:"failed"}];
  for(const attempt of attempts){
   const path=join(directory,"state/gator/jobs/history",original.id,attempt.id);await mkdir(path,{recursive:true});
   await writeFile(join(path,"intent.json"),JSON.stringify({version:1,execution_id:attempt.id,definition:original}));
   await writeFile(join(path,"result.json"),JSON.stringify({version:1,id:attempt.id,job_id:original.id,status:attempt.status,started_at:checkedAt,finished_at:checkedAt,summary:"No material change",watch_decision:{status:"quiet",summary:"The venue deadline is unchanged",rationale:"The source still says Friday",evidence:["source/project.md"]}}));
  }
  await page.locator(".panel-row").filter({hasText:"Venue replies"}).first().click();
  await page.locator(".panel-more summary").click();await page.getByRole("button",{name:/review quiet checks/i}).click();
  await expect(page.getByRole("heading",{name:"Quiet checks",exact:true})).toBeVisible();
  await expect(page.locator(".panel-row")).toHaveCount(1);await page.locator(".panel-row").click();
  await expect(page.getByText("The venue deadline is unchanged",{exact:true})).toBeVisible();
  await page.getByRole("button",{name:/missed a change/i}).click();
  await page.getByRole("textbox",{name:"Change that should have been reported",exact:true}).fill("The venue moved the deadline to Thursday.");
  await page.getByRole("button",{name:/save feedback/i}).click();
  await expect(page.getByText("Feedback saved. It does not change the agreement or send an alert.",{exact:true})).toBeVisible();
  const reviewPath=join(directory,"state/gator/jobs/history",original.id,attempts[0].id,"quiet-review.json");
  expect(JSON.parse(await readFile(reviewPath,"utf8")).verdict).toBe("missed");
  expect(JSON.parse(await readFile(reviewPath,"utf8")).note).toBe("The venue moved the deadline to Thursday.");
  await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].setSize(900,720));await page.screenshot({path:"test-results/followup-quiet-review.png"});
  await page.getByRole("button",{name:/correctly stayed quiet/i}).click();
  await expect.poll(async()=>JSON.parse(await readFile(reviewPath,"utf8")).verdict).toBe("correct");
  await page.getByRole("button",{name:/missed a change/i}).click();
  const feedback=page.getByRole("textbox",{name:"Change that should have been reported",exact:true});
  const unsaved="A new note that must survive a conflicting assessment.";await feedback.fill(unsaved);
  const concurrent=JSON.parse(await readFile(reviewPath,"utf8"));
  await writeFile(reviewPath,JSON.stringify({...concurrent,version:2,sequence:concurrent.sequence+1,rated_at:new Date().toISOString()}));
  await page.getByRole("button",{name:/save feedback/i}).click();
  await expect(page.getByText("feedback changed; refresh this check before reviewing it again",{exact:true})).toBeVisible();
  await expect(feedback).toHaveValue(unsaved);
  await page.getByRole("button",{name:"New chat",exact:true}).click();await expect(page.getByRole("alertdialog")).toBeVisible();
  await page.getByRole("button",{name:"Keep editing",exact:true}).click();await expect(feedback).toHaveValue(unsaved);
  await page.getByRole("button",{name:/^Cancel/i}).click();
  await page.getByRole("button",{name:"Refresh",exact:true}).click();
  await expect(page.locator(".panel-row")).toHaveCount(1);await expect(page.locator(".panel-row")).toContainText("The venue deadline is unchanged");
  await page.locator(".panel-row").click();await page.locator(".panel-more summary").click();
  await page.getByRole("button",{name:/clear feedback/i}).click();
  await expect.poll(async()=>JSON.parse(await readFile(reviewPath,"utf8")).verdict).toBe("");
  expect(JSON.parse(await readFile(reviewPath,"utf8")).sequence).toBeGreaterThan(concurrent.sequence+1);
  await page.getByRole("button",{name:"Refresh",exact:true}).click();
  await expect(page.locator(".workflow-content")).toContainText("1 quiet checks · 0 reviewed · 0 marked missed");
  expect((await saved())[0].alert_when).toBe("Only a price changes");expect((await saved())[0].enabled).toBe(false);
  await page.evaluate(()=>window.gator!.send({type:"navigate",text:"/followup"}));
  await page.getByRole("button",{name:/new check/i}).click();
  await page.getByLabel("End date (optional)",{exact:true}).fill("2030-10-25T18:30");
  await page.getByRole("button",{name:/^review/i}).click();
  await expect.poll(async()=>(await state(page)).panel?.title).toBe("Review ongoing check");
  expect((await state(page)).panel?.blocks?.[0].fields?.find(field=>field.label==="Ends")?.value).toContain("25 Oct 2030");
 } finally {
  try {
   await app.evaluate(({dialog})=>{dialog.showMessageBox=async()=>({response:0,checkboxChecked:false});}).catch(()=>{});
   await closeDesktop(app);
  } finally {
   server.closeAllConnections();await new Promise<void>(done=>server.close(()=>done()));await rm(directory,{recursive:true,force:true});
  }
 }
});
