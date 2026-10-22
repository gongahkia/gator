import { _electron as electron, test, expect, type ElectronApplication } from "@playwright/test";
import { mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { createServer } from "node:http";
import { join, resolve } from "node:path"; import { tmpdir } from "node:os";
import { wordFixture, workbookFixture, slidesFixture } from "../office-fixtures.mjs";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

async function fixture() {
  const directory=await temporaryDirectory(join(tmpdir(),"gator-inline-files-")), workspace=join(directory,"workspace");
  await mkdir(workspace);await mkdir(join(directory,"config/gator"),{recursive:true});
  const requests: unknown[]=[];
  const server=createServer((request,response)=>{let body="";request.on("data",chunk=>body+=chunk);request.on("end",()=>{requests.push(JSON.parse(body));response.writeHead(200,{"Content-Type":"application/json"});response.end(JSON.stringify({choices:[{message:{role:"assistant",content:"I received the current file."},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));});});
  await new Promise<void>(resolve=>server.listen(0,"127.0.0.1",resolve));const port=(server.address() as {port:number}).port;
  await writeFile(join(directory,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"file-editor-fixture",model:"fixture-model"},custom_providers:[{id:"file-editor-fixture",base_url:`http://127.0.0.1:${port}/v1/chat/completions`,models:["fixture-model"],default_model:"fixture-model"}]}),{mode:0o600});
  const env={...process.env,GATOR_STATE_DIR:join(directory,"state"),GATOR_CONFIG_DIR:join(directory,"config"),GATOR_DESKTOP_USER_DATA:join(directory,"desktop"),GATOR_DESKTOP_WORKSPACE:workspace};
  let app: ElectronApplication;
  const launch=async()=>{const executablePath=process.env.GATOR_DESKTOP_TEST_EXECUTABLE;app=await electron.launch({executablePath,args:executablePath?[]:["."],cwd:resolve("."),env});return app;};
  return {directory,requests,launch,async close(){if(app!)await closeDesktop(app);server.closeAllConnections();await new Promise<void>(resolve=>server.close(()=>resolve()));await rm(directory,{recursive:true,force:true});}};
}
const pick=(app:ElectronApplication,paths:string[])=>app.evaluate(({dialog},paths)=>{dialog.showOpenDialog=async()=>({canceled:false,filePaths:paths});},paths);

test("inline text editing saves full versions, recovers drafts and sends the edited bytes",async()=>{
  const f=await fixture();try{
    const path=join(f.directory,"notes.md"), original="# A useful document\n\n"+"Context to preserve.\n".repeat(300)+"\nOriginal final paragraph.";
    await writeFile(path,original);const app=await f.launch(),page=await app.firstWindow();const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
    const composer=page.getByRole("textbox",{name:"Message Gator",exact:true});await composer.fill("Keep my unsent question.");
    await pick(app,[path]);await page.getByRole("button",{name:"Attach files",exact:true}).click();
    const pending=page.getByRole("region",{name:"Attachments for your next message"});
    const card=pending.locator(".inline-file");await card.getByRole("button",{name:"Open file notes.md",exact:true}).click();
    await expect(card.getByText("Original final paragraph.",{exact:true})).toBeVisible();await card.getByRole("button",{name:"Edit",exact:true}).click();
    const editor=card.getByRole("textbox",{name:"Edit notes.md",exact:true}),edited=original.replace("Original final paragraph.","Edited final paragraph: violet kiwi.");
    await editor.fill(edited);await card.getByRole("button",{name:"Save",exact:true}).click();
    await expect(card.getByText("Saved · included in your next message",{exact:true})).toBeVisible();await expect(composer).toHaveValue("Keep my unsent question.");
    expect(await readFile(path,"utf8")).toBe(original);expect(f.requests).toHaveLength(0);
    await card.getByRole("button",{name:"Version history for notes.md",exact:true}).click();await expect(card.getByText("Version 1 · Original",{exact:true})).toBeVisible();
    await card.getByRole("button",{name:/Version 1 · Original/}).click();await expect(card.getByText("Original final paragraph.",{exact:true})).toBeVisible();
    await card.getByRole("button",{name:"Version history for notes.md",exact:true}).click();await card.getByRole("button",{name:/Version 2 · Edited by you/}).click();
    await card.getByRole("button",{name:"Edit",exact:true}).click();await editor.fill(edited+"\nAn unsaved edit that must survive.");
    await expect.poll(async()=>{const drafts=await page.evaluate(()=>window.gator!.drafts({action:"list"}));const id=drafts.drafts[0]?.uploads?.[0]?.id;if(!id)return false;return page.evaluate(async({id,context})=>(await window.gator!.files({action:"open",context,ref:{kind:"upload",id}})).draft?.text?.endsWith("must survive.")??false,{id,context:drafts.drafts[0].context});}).toBe(true);
    await page.reload();await pending.getByRole("button",{name:"Open file notes.md",exact:true}).click();await expect(editor).toHaveValue(edited+"\nAn unsaved edit that must survive.");
    await card.getByRole("button",{name:"Discard",exact:true}).click();await expect(card.getByText("Edited final paragraph: violet kiwi.",{exact:true})).toBeVisible();
    await card.getByRole("button",{name:"Ask Gator",exact:true}).click();await card.getByRole("textbox",{name:"Request changes to notes.md",exact:true}).fill("Make the final paragraph clearer.");await card.getByRole("button",{name:"Add to message",exact:true}).click();
    await expect(composer).toHaveValue(/Keep my unsent question\.[\s\S]*Make the final paragraph clearer\./);expect(f.requests).toHaveLength(0);
    await page.screenshot({path:"test-results/inline-file-markdown.png"});
    await page.evaluate(()=>window.gator!.send({type:"submit",text:"/mode inspect"}));await page.getByRole("button",{name:"Send message",exact:true}).click();
    await expect.poll(()=>f.requests.length).toBeGreaterThan(0);expect(JSON.stringify(f.requests)).toContain("Edited final paragraph: violet kiwi.");expect(JSON.stringify(f.requests)).not.toContain("An unsaved edit that must survive.");
    await expect(page.locator(".assistant-message").getByText("I received the current file.",{exact:true})).toBeVisible();
    await page.locator(".user-message").getByRole("button",{name:"Open file notes.md",exact:true}).click();await expect(page.locator(".user-message").getByText("Edited final paragraph: violet kiwi.",{exact:true})).toBeVisible();
    expect(errors).toEqual([]);
  }finally{await f.close();}
});

test("Word Excel PowerPoint and CSV edit inline and export native files with unchanged originals",async()=>{
  const f=await fixture();try{
    const paths=[join(f.directory,"brief.docx"),join(f.directory,"budget.xlsx"),join(f.directory,"review.pptx"),join(f.directory,"people.csv")];
    const originals=[wordFixture(),workbookFixture(),slidesFixture(),Buffer.from("name,role\nAlex,Designer\n")];await Promise.all(paths.map((path,i)=>writeFile(path,originals[i])));
    const app=await f.launch(),page=await app.firstWindow(),errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toBeVisible();
    await pick(app,paths);await page.getByRole("button",{name:"Attach files",exact:true}).click();
    const pending=page.getByRole("region",{name:"Attachments for your next message"});
    for(const [index,name,label,value] of [[0,"brief.docx","Document paragraph 0","Hello from the edited Word document."],[1,"budget.xlsx","Budget cell B1","125"],[2,"review.pptx","Slide 1 text 0","An edited PowerPoint title"],[3,"people.csv","Table cell B2","Product designer"]] as const){
      const card=pending.locator(".inline-file").filter({has:page.getByRole("button",{name:`Open file ${name}`,exact:true})});
      await card.getByRole("button",{name:`Open file ${name}`,exact:true}).click();await card.getByRole("button",{name:"Edit",exact:true}).click();
      await card.getByRole("textbox",{name:label,exact:true}).fill(value);await page.screenshot({path:`test-results/inline-file-${name}.png`});
      await card.getByRole("button",{name:"Save",exact:true}).click();await expect(card.getByText("Saved · included in your next message",{exact:true})).toBeVisible();
      const exported=join(f.directory,`edited-${name}`);await app.evaluate(({dialog},filePath)=>{dialog.showSaveDialog=async()=>({canceled:false,filePath});},exported);
      await card.getByRole("button",{name:`Export ${name}`,exact:true}).click();await expect.poll(async()=>readFile(exported).then(bytes=>bytes.length).catch(()=>0)).toBeGreaterThan(0);
      expect(await readFile(paths[index])).toEqual(originals[index]);expect(await readFile(exported)).not.toEqual(originals[index]);
      await card.getByRole("button",{name:`Close file ${name}`,exact:true}).click();
    }
    await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].setSize(800,650));await page.emulateMedia({reducedMotion:"reduce"});
    await pending.getByRole("button",{name:"Open file brief.docx",exact:true}).click();await expect(pending.getByText("Hello from the edited Word document.",{exact:true})).toBeVisible();
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:"test-results/inline-file-office-narrow.png"});expect(f.requests).toHaveLength(0);expect(errors).toEqual([]);
  }finally{await f.close();}
});
