import { _electron as electron, test, expect, type Page } from "@playwright/test";
import { createServer } from "node:http";
import { mkdir, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, temporaryDirectory } from "./lifecycle";
const state=(page:Page)=>page.evaluate(async()=>(await window.gator!.snapshot()).state!);

test("first task keeps a file and draft through model setup and sends only on request",async()=>{
 test.setTimeout(120000);
 const directory=await temporaryDirectory(join(tmpdir(),"gator-first-task-")),workspace=join(directory,"workspace");
 await mkdir(workspace);await mkdir(join(directory,"config/gator"),{recursive:true});
 const file=join(directory,"meeting.txt");await writeFile(file,"Meeting note: confirm the venue on Friday.");
 let requests=0;
 const server=createServer((req,res)=>{let body="";req.on("data",chunk=>body+=chunk);req.on("end",()=>{requests++;const payload=JSON.parse(body);const classify=payload.messages.some((m:{role:string;content?:string})=>m.role==="system"&&m.content?.includes("Classify the user's immediate request"));res.writeHead(200,{"Content-Type":"application/json"});res.end(JSON.stringify({choices:[{message:{role:"assistant",content:classify?'{"mode":"inspect"}':"The meeting note asks you to confirm the venue on Friday."},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));});});
 await new Promise<void>(done=>server.listen(0,"127.0.0.1",done));
 const base=`http://127.0.0.1:${(server.address() as {port:number}).port}`;
 await writeFile(join(directory,"config/gator/config.json"),JSON.stringify({version:4,defaults:{},custom_providers:[{id:"starter",base_url:base+"/v1/chat/completions",models:["fixture"],default_model:"fixture"}]}),{mode:0o600});
 const env=Object.fromEntries(Object.entries({...process.env,GATOR_INTERNAL_COMMANDS:"",GATOR_STATE_DIR:join(directory,"state"),GATOR_CONFIG_DIR:join(directory,"config"),GATOR_DESKTOP_USER_DATA:join(directory,"desktop"),GATOR_DESKTOP_WORKSPACE:workspace}).filter((entry):entry is [string,string]=>typeof entry[1]==="string"));
 const executablePath=process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
 const app=await electron.launch({executablePath,args:executablePath?[]:["."],cwd:resolve("."),env});
 try{
  const page=await app.firstWindow();await expect(page.getByRole("region",{name:"Get started with Gator"})).toBeVisible();
  await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].setSize(760,560));
  await page.screenshot({path:"test-results/first-task-minimum.png"});
  await app.evaluate(({dialog},file)=>{dialog.showOpenDialog=async()=>({canceled:false,filePaths:[file]});},file);
  await page.getByRole("button",{name:"Understand a file",exact:true}).click();
  const composer=page.getByRole("textbox",{name:"Message Gator",exact:true});
  const text="Help me understand this file. Summarize the main points and anything I should pay attention to.";
  await expect(composer).toHaveValue(text);await expect(page.getByRole("button",{name:"Remove attachment meeting.txt",exact:true})).toBeVisible();
  expect(requests).toBe(0);
  await page.getByRole("button",{name:"Choose a model",exact:true}).click();
  await page.getByRole("button",{name:/^Cloud API key/}).click();
  await page.getByRole("searchbox",{name:"Filter models"}).fill("starter");
  await page.getByRole("button",{name:/^starter · fixture/}).click();
  await page.getByRole("button",{name:"Back to app",exact:true}).click();
  await expect(composer).toHaveValue(text);await expect(page.getByRole("button",{name:"Remove attachment meeting.txt",exact:true})).toBeVisible();
  await expect(page.getByRole("region",{name:"Get started with Gator"})).toHaveCount(0);expect(requests).toBe(0);
  await page.reload();await expect(composer).toHaveValue(text);await expect(page.getByRole("button",{name:"Remove attachment meeting.txt",exact:true})).toBeVisible();expect(requests).toBe(0);
  await page.getByRole("button",{name:"Send message",exact:true}).click();
  await expect.poll(async()=>(await state(page)).messages.some(m=>m.role==="Gator"&&m.text.includes("confirm the venue on Friday"))).toBe(true);
  await expect.poll(async()=>(await state(page)).running).toBe(false);expect(requests).toBeGreaterThan(0);
  await closeDesktop(app);
 }finally{await app.evaluate(({dialog})=>{dialog.showMessageBox=async()=>({response:0,checkboxChecked:false});}).catch(()=>{});await closeDesktop(app).catch(()=>{});server.closeAllConnections();await new Promise<void>(done=>server.close(()=>done()));await rm(directory,{recursive:true,force:true});}
});
