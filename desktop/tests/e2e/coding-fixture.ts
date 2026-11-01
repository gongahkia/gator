import { createServer } from "node:http";
import { mkdir, writeFile, readFile, rm } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { expect, type Page, type ElectronApplication } from "@playwright/test";
import { closeDesktop, temporaryDirectory, launchDesktop } from "./lifecycle";

export const codingBase = 'module.exports.normalize = value => value;\n';
export const codingFixed = 'module.exports.normalize = value => value.trim().toLowerCase().replace(/\\s+/g, "-");\n';
export const codingFollowup = 'module.exports.normalize = value => value.trim().toLowerCase().replace(/[\\s_]+/g, "-");\n';
export const codingTests = 'const { strict: assert } = require("node:assert");\nconst { normalize } = require("./normalize.cjs");\nassert.equal(normalize(" Ship  NOW "), "ship-now");\nassert.equal(normalize(" λ  NOTE "), "λ-note");\nassert.equal(normalize(""), "");\nconsole.log("3 normalization assertions passed");\n';
export function codingPatch(before: string, after: string) { return `diff --git a/normalize.cjs b/normalize.cjs\n--- a/normalize.cjs\n+++ b/normalize.cjs\n@@ -1 +1 @@\n-${before.trimEnd()}\n+${after.trimEnd()}\n`; }
export type FixtureRequest = { messages: { role: string; content?: string; tool_calls?: { id: string; function: {name: string; arguments: string} }[] }[]; tools?: {function:{name:string}}[] };
export function fixtureTool(name: string, args: unknown, content?: string) { return { role: "assistant", ...(content ? {content} : {}), tool_calls: [{ id: `call-${name}`, type: "function", function: { name, arguments: JSON.stringify(args) } }] }; }
export async function codingFixture(respond?: (body: FixtureRequest) => object) {
 const root = await temporaryDirectory(join(tmpdir(), "gator-coding-ui-")), workspace = join(root,"workspace"), config = join(root,"config/gator");
 await mkdir(workspace); await mkdir(config,{recursive:true});
 await writeFile(join(workspace,"package.json"),JSON.stringify({name:"disposable-gator-coding-fixture",private:true,scripts:{test:"node verify.cjs"}}));
 await writeFile(join(workspace,"normalize.cjs"),codingBase);await writeFile(join(workspace,"verify.cjs"),codingTests);
 let calls = 0;
 const server = createServer((request,response)=>{
  let text="";request.on("data",chunk=>text+=chunk);request.on("end",()=>{
   calls++;const body=JSON.parse(text) as FixtureRequest;
   try {const message=respond!(body);response.writeHead(200,{"Content-Type":"application/json"});response.end(JSON.stringify({choices:[{message,finish_reason:"tool_calls" in message?"tool_calls":"stop"}],usage:{prompt_tokens:20,completion_tokens:20}}));}
   catch (error) {response.writeHead(500);response.end(JSON.stringify({error:{message:String(error).slice(0,4000)}}));}
  });
 });
 let app: ElectronApplication | undefined;
 if (respond) await new Promise<void>(done=>server.listen(0,"127.0.0.1",done));
 const provider=respond ? "coding-fixture" : "openai", model=respond ? "fixture" : "gpt-5.6";
 const settings=respond ? {version:4,defaults:{provider,model},custom_providers:[{id:provider,base_url:`http://127.0.0.1:${(server.address() as {port:number}).port}/v1/chat/completions`,models:[model],default_model:model}]} : {version:4,defaults:{provider,model},provider_endpoints:{openai:process.env.GATOR_LIVE_UI_ENDPOINT}};
 await writeFile(join(config,"config.json"),JSON.stringify(settings));
 const env={...process.env,GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config"),GATOR_DESKTOP_USER_DATA:join(root,"desktop"),GATOR_DESKTOP_WORKSPACE:workspace,...(!respond?{OPENAI_API_KEY:process.env.GATOR_LIVE_UI_TOKEN!,GATOR_PROVIDER:provider,GATOR_MODEL:model}:{} )};
 execFileSync(resolve("runtime",process.platform==="win32"?"gator-runtime.exe":"gator-runtime"),["config","show"],{env:{...env,GATOR_INTERNAL_COMMANDS:"1"},stdio:"ignore"});
 return {root,workspace,calls:()=>calls,
 async launch(){execFileSync(process.execPath,["--check",join(workspace,"verify.cjs")],{stdio:"ignore"});const executablePath=process.env.GATOR_DESKTOP_TEST_EXECUTABLE;app=await launchDesktop({executablePath,args:executablePath?[]:["."],cwd:resolve("."),env});return app;},
 async original(){return readFile(join(workspace,"normalize.cjs"),"utf8");},
 async close(){try{if(app){await app.evaluate(({dialog})=>{dialog.showMessageBox=async()=>({response:1,checkboxChecked:false});}).catch(()=>{});await closeDesktop(app);}}finally{if(respond){server.closeAllConnections();await new Promise<void>(done=>server.close(()=>done()));}await rm(root,{recursive:true,force:true});}}
 };
}

export async function askCoding(page: Page, text: string) {
 const before=(await page.evaluate(()=>window.gator!.snapshot())).state!;
 const beforeUsers=before.messages.filter(message=>message.role==="You").length;
 const composer=page.getByRole("textbox",{name:"Message Gator",exact:true});await expect(composer).toBeEnabled();
 await composer.fill(text);await page.getByRole("button",{name:"Send message",exact:true}).click();
 await expect.poll(async()=>{const state=(await page.evaluate(()=>window.gator!.snapshot())).state!;const user=state.messages.map(message=>message.role).lastIndexOf("You"); return !state.running && state.messages.filter(message=>message.role==="You").length>beforeUsers && state.messages.slice(user+1).some(message=>message.role==="Gator");},{timeout:240000}).toBe(true);
}
