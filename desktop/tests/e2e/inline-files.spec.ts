import { _electron as electron, test, expect, type ElectronApplication } from "@playwright/test";
import { mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { createServer } from "node:http";
import { join, resolve } from "node:path"; import { tmpdir } from "node:os";
import { unzipSync, strFromU8 } from "fflate";
import { wordFixture, workbookFixture, slidesFixture } from "../office-fixtures.mjs";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

async function fixture(generated = false) {
  const directory=await temporaryDirectory(join(tmpdir(),"gator-inline-files-")), workspace=join(directory,"workspace");
  await mkdir(workspace);await mkdir(join(directory,"config/gator"),{recursive:true});
  const requests: unknown[]=[]; let wroteReport = false;
  const server=createServer((request,response)=>{let body="";request.on("data",chunk=>body+=chunk);request.on("end",()=>{requests.push(JSON.parse(body));response.writeHead(200,{"Content-Type":"application/json"});
    const write = generated && !wroteReport; wroteReport = wroteReport || write;
    const message = write ? { role: "assistant", content: null, tool_calls: [{ id: "write-inline-report", type: "function", function: { name: "write_artifact", arguments: JSON.stringify({ path: "report.md", content: "# A generated report\n\nOriginal sealed output." }) } }] } : { role: "assistant", content: "I received the current file." };
    response.end(JSON.stringify({choices:[{message,finish_reason:write ? "tool_calls" : "stop"}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));});});
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
    const spare=join(f.directory,"spare.txt");await writeFile(spare,"Another attachment");
    await writeFile(path,original);const app=await f.launch(),page=await app.firstWindow();const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));
    const composer=page.getByRole("textbox",{name:"Message Gator",exact:true});await composer.fill("Keep my unsent question.");
    await pick(app,[spare,path]);await page.getByRole("button",{name:"Attach files",exact:true}).click();
    const pending=page.getByRole("region",{name:"Attachments for your next message"});
    const card=pending.locator(".inline-file").filter({has:page.getByRole("button",{name:"Open file notes.md",exact:true})});await card.getByRole("button",{name:"Open file notes.md",exact:true}).click();
    await expect(card.getByText("Original final paragraph.",{exact:true})).toBeVisible();await card.getByRole("button",{name:"Edit",exact:true}).click();
    const editor=card.getByRole("textbox",{name:"Edit notes.md",exact:true}),edited=original.replace("Original final paragraph.","Edited final paragraph: violet kiwi.");
    await editor.fill(edited);
    await pending.getByRole("button",{name:"Remove attachment spare.txt",exact:true}).click();
    await expect(editor).toHaveValue(edited);await expect(page.getByRole("button",{name:"Send message",exact:true})).toBeDisabled();
    await card.getByRole("button",{name:"Save",exact:true}).click();
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
      await card.getByRole("textbox",{name:label,exact:true}).fill(value);
      await expect(card.getByRole("textbox",{name:label,exact:true})).toHaveCSS("outline-style","none");
      if (name === "budget.xlsx") {
        await card.getByRole("textbox",{name:"Budget cell C1",exact:true}).click();
        await card.getByRole("textbox",{name:"Cell value or formula",exact:true}).fill("=B1*3");
        await card.getByRole("button",{name:"Row",exact:true}).click();
        await card.getByRole("textbox",{name:"Budget cell B2",exact:true}).fill("33");
        await card.getByRole("button",{name:"Column",exact:true}).click();
        await card.getByRole("textbox",{name:"Budget cell D1",exact:true}).fill("Notes");
      }
      const send = await page.getByRole("button",{name:"Send message",exact:true}).boundingBox();
      expect(send!.y + send!.height).toBeLessThanOrEqual(await page.evaluate(()=>innerHeight));
      await expect(page.getByRole("button",{name:"Send message",exact:true})).toBeDisabled();
      await page.screenshot({path:`test-results/inline-file-${name}.png`});
      await card.getByRole("button",{name:"Save",exact:true}).click();await expect(card.getByText("Saved · included in your next message",{exact:true})).toBeVisible();
      const exported=join(f.directory,`edited-${name}`);await app.evaluate(({dialog},filePath)=>{dialog.showSaveDialog=async()=>({canceled:false,filePath});},exported);
      await card.getByRole("button",{name:`Export ${name}`,exact:true}).click();await expect.poll(async()=>readFile(exported).then(bytes=>bytes.length).catch(()=>0)).toBeGreaterThan(0);
      expect(await readFile(paths[index])).toEqual(originals[index]);expect(await readFile(exported)).not.toEqual(originals[index]);
      const bytes = await readFile(exported);
      if (name.endsWith("csv")) expect(bytes.toString()).toContain(value);
      else {
        const parts = unzipSync(bytes), part = name.endsWith("docx") ? "word/document.xml" : name.endsWith("xlsx") ? "xl/worksheets/sheet1.xml" : "ppt/slides/slide1.xml";
        expect(strFromU8(parts[part]).replace(/<[^>]+>/g,"" )).toContain(value);
        if (name.endsWith("xlsx")) { expect(strFromU8(parts[part])).not.toContain("<v>8</v>"); expect(strFromU8(parts[part])).toContain("<f>B1*3</f>"); expect(strFromU8(parts[part])).toContain("Notes"); expect(strFromU8(parts[part])).toContain("<v>33</v>"); }
      }
      await card.getByRole("button",{name:`Close file ${name}`,exact:true}).click();
    }
    await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].setSize(800,650));await page.emulateMedia({reducedMotion:"reduce"});
    await pending.getByRole("button",{name:"Open file brief.docx",exact:true}).click();await expect(pending.getByText("Hello from the edited Word document.",{exact:true})).toBeVisible();
    expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:"test-results/inline-file-office-narrow.png"});expect(f.requests).toHaveLength(0);expect(errors).toEqual([]);
  }finally{await f.close();}
});

function pdfFixture() {
  const objects = [
    '<< /Type /Catalog /Pages 2 0 R >>',
    '<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 2 >>',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 180] /Resources << /Font << /F1 7 0 R >> >> /Contents 4 0 R >>',
    '<< /Length 48 >>\nstream\nBT /F1 18 Tf 20 100 Td (A real PDF preview) Tj ET\nendstream',
    '<< /Type /Page /Parent 2 0 R /MediaBox [0 0 300 180] /Resources << /Font << /F1 7 0 R >> >> /Contents 6 0 R >>',
    '<< /Length 48 >>\nstream\nBT /F1 18 Tf 20 100 Td (Second PDF page) Tj ET\nendstream',
    '<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>',
  ];
  let data = '%PDF-1.4\n', offsets = [0];
  for (const [index, object] of objects.entries()) { offsets.push(Buffer.byteLength(data)); data += `${index + 1} 0 obj\n${object}\nendobj\n`; }
  const xref = Buffer.byteLength(data);
  data += `xref\n0 ${objects.length + 1}\n0000000000 65535 f \n${offsets.slice(1).map(offset=>`${String(offset).padStart(10,'0')} 00000 n \n`).join('')}trailer\n<< /Size ${objects.length + 1} /Root 1 0 R >>\nstartxref\n${xref}\n%%EOF\n`;
  return data;
}

test("PDF pages and images render inline without enabling edits or network requests", async()=>{
  const f=await fixture();try{
    const pdf=join(f.directory,"pages.pdf"),image=join(f.directory,"pixel.png");
    await writeFile(pdf,pdfFixture());await writeFile(image,Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jL1sAAAAASUVORK5CYII=","base64"));
    const app=await f.launch(),page=await app.firstWindow(),errors:string[]=[],requests:string[]=[];
    page.on("pageerror",error=>errors.push(error.message));page.on("request",request=>{if(/^https?:/.test(request.url()))requests.push(request.url());});
    await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toBeVisible();await pick(app,[pdf,image]);await page.getByRole("button",{name:"Attach files",exact:true}).click();
    const pending=page.getByRole("region",{name:"Attachments for your next message"});
    const card=pending.locator(".inline-file").filter({has:page.getByRole("button",{name:"Open file pages.pdf",exact:true})});
    await card.getByRole("button",{name:"Open file pages.pdf",exact:true}).click();await expect(card.getByText("Page 1 of 2",{exact:true})).toBeVisible();
    await expect.poll(()=>card.locator("canvas").evaluate((canvas:HTMLCanvasElement)=>{const ctx=canvas.getContext("2d")!;if(canvas.width<100)return false;const pixels=ctx.getImageData(0,0,canvas.width,canvas.height).data;let dark=0;for(let i=0;i<pixels.length;i+=4)if(pixels[i]<100 && pixels[i+3])dark++;return dark>100;})).toBe(true);
    await card.getByRole("button",{name:"Next PDF page",exact:true}).click();await expect(card.getByText("Page 2 of 2",{exact:true})).toBeVisible();
    await expect(card.getByRole("button",{name:"Edit",exact:true})).toHaveCount(0);await page.screenshot({path:"test-results/inline-file-pdf.png"});await card.getByRole("button",{name:"Close file pages.pdf",exact:true}).click();
    await pending.getByRole("button",{name:"Open file pixel.png",exact:true}).click();await expect.poll(()=>pending.getByRole("img",{name:"pixel.png",exact:true}).evaluate((img:HTMLImageElement)=>img.naturalWidth)).toBe(1);
    expect(f.requests).toHaveLength(0);expect(requests).toEqual([]);expect(errors).toEqual([]);
  }finally{await f.close();}
});

test("generated outputs edit in their message and send the working version without changing sealed output",async()=>{
  const f=await fixture(true);try{
    const app=await f.launch(),page=await app.firstWindow(),errors:string[]=[];page.on("pageerror",error=>errors.push(error.message));
    const composer=page.getByRole("textbox",{name:"Message Gator",exact:true});await expect(composer).toBeVisible();
    await page.evaluate(()=>window.gator!.send({type:"submit",text:"/mode draft"}));await page.evaluate(()=>window.gator!.send({type:"submit",text:"/artifact report.md"}));await composer.fill("Create report.md as the inline report fixture.");await page.getByRole("button",{name:"Send message",exact:true}).click();
    const card=page.locator(".conversation-scroll .inline-file").filter({has:page.getByRole("button",{name:"Open file report.md",exact:true})}).first();
    await expect(card).toBeVisible();await expect(page.getByRole("button",{name:"Send message",exact:true})).toBeVisible();
    const bundle=await page.evaluate(async()=>(await window.gator!.snapshot()).state!.messages.flatMap(message=>message.bundle?[message.bundle]:[]).find(bundle=>bundle.Artifacts?.some(file=>file.Path==="report.md")));
    expect(bundle?.Artifacts?.find(file=>file.Path==="report.md")?.SHA256).toMatch(/^[a-f0-9]{64}$/);
    const originalPath=join(bundle!.Path,"output/report.md"),original=await readFile(originalPath,"utf8");
    await card.getByRole("button",{name:"Open file report.md",exact:true}).click();await card.getByRole("button",{name:"Edit",exact:true}).click();await card.getByRole("textbox",{name:"Edit report.md",exact:true}).fill("# A generated report\n\nEdited output: silver otter.");
    await card.getByRole("button",{name:"Save",exact:true}).click();await expect(card.getByText("Saved · included in your next message",{exact:true})).toBeVisible();expect(await readFile(originalPath,"utf8")).toBe(original);
    await card.getByRole("button",{name:"Ask Gator",exact:true}).click();await card.getByRole("textbox",{name:"Request changes to report.md",exact:true}).fill("Please review my edits.");await card.getByRole("button",{name:"Add to message",exact:true}).click();
    await expect(composer).toHaveValue(/Please review my edits/);await page.evaluate(()=>window.gator!.send({type:"submit",text:"/artifact clear"}));await page.evaluate(()=>window.gator!.send({type:"submit",text:"/mode inspect"}));const count=f.requests.length;await page.getByRole("button",{name:"Send message",exact:true}).click();await expect.poll(()=>f.requests.length).toBeGreaterThan(count);expect(JSON.stringify(f.requests.slice(count))).toContain("Edited output: silver otter.");expect(await readFile(originalPath,"utf8")).toBe(original);expect(errors).toEqual([]);
  }finally{await f.close();}
});


test("saved file versions compare inline and export the exact chosen bytes without a model call", async () => {
  const f = await fixture();
  try {
    const path = join(f.directory, "comparison.md"), original = "# Decision\n\nBudget: 100\nKeep this line.\n", edited = "# Decision\n\nBudget: 125\nKeep this line.\nApproved next step.\n";
    await writeFile(path, original);
    const app = await f.launch(), page = await app.firstWindow(), errors: string[] = [];
    page.on("pageerror", error => errors.push(error.message));
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await composer.fill("Keep this unsent question.");
    await pick(app, [path]); await page.getByRole("button", { name: "Attach files", exact: true }).click();
    const card = page.locator(".pending-attachments .inline-file");
    await card.getByRole("button", { name: "Open file comparison.md", exact: true }).click();
    await card.getByRole("button", { name: "Edit", exact: true }).click();
    await card.getByRole("textbox", { name: "Edit comparison.md", exact: true }).fill(edited);
    await expect(card.getByRole("button", { name: "Compare versions of comparison.md", exact: true })).toHaveCount(0);
    await card.getByRole("button", { name: "Save", exact: true }).click();
    await expect(card.getByText("Saved · included in your next message", { exact: true })).toBeVisible();
    await card.getByRole("button", { name: "Compare versions of comparison.md", exact: true }).click();
    let comparison = card.getByRole("region", { name: "Compare versions of comparison.md", exact: true });
    await expect(comparison.getByText("2 changed sections", { exact: true })).toBeVisible();
    await expect(comparison.getByText("Budget: 100", { exact: true })).toBeVisible();
    await expect(comparison.getByText("Budget: 125", { exact: true })).toBeVisible();
    await expect(composer).toHaveValue("Keep this unsent question.");
    for (const [label, text, name] of [["Export before version", original, "before.md"], ["Export after version", edited, "after.md"]]) {
      const filePath = join(f.directory, name);
      await app.evaluate(({ dialog }, filePath) => { dialog.showSaveDialog = async () => ({ canceled: false, filePath }); }, filePath);
      await comparison.getByRole("button", { name: label, exact: true }).click();
      await expect.poll(async () => readFile(filePath, "utf8").catch(() => "")).toBe(text);
    }
    const to = await comparison.getByRole("combobox", { name: "Compare to version" }).inputValue();
    await comparison.getByRole("combobox", { name: "Compare from version" }).selectOption(to);
    await expect(comparison.getByText("The file bytes are identical.", { exact: true })).toBeVisible();
    await page.reload(); await card.getByRole("button", { name: "Open file comparison.md", exact: true }).click();
    await card.getByRole("button", { name: "Compare versions of comparison.md", exact: true }).click();
    comparison = card.getByRole("region", { name: "Compare versions of comparison.md", exact: true });
    await expect(comparison.getByText("2 changed sections", { exact: true })).toBeVisible();
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(760, 560));
    await expect(page.getByRole("button", { name: "Send message", exact: true })).toBeInViewport();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await comparison.getByText("Budget: 125", { exact: true }).scrollIntoViewIfNeeded();
    await page.screenshot({ path: "test-results/file-comparison-dark.png", animations: "disabled" });
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/theme light" }));
    await expect(page.locator(".app-shell")).toHaveAttribute("data-theme", "light");
    await page.screenshot({ path: "test-results/file-comparison-light.png", animations: "disabled" });
    expect(await readFile(path, "utf8")).toBe(original); expect(f.requests).toHaveLength(0); expect(errors).toEqual([]);
  } finally { await f.close(); }
});


test("larger attachment selections survive reload, reject overflow and reach the local model intact", async () => {
  test.setTimeout(90000);
  const f = await fixture();
  try {
    const paths = Array.from({ length: 12 }, (_, i) => join(f.directory, `document-${i}.txt`));
    const large = "Larger attachment marker: amber heron.\n" + "a".repeat(5 * 1024 * 1024);
    await Promise.all(paths.map((path, i) => writeFile(path, i === 0 ? large : `Selected document ${i}`)));
    const app = await f.launch(), page = await app.firstWindow(), composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeVisible(); await composer.fill("Read the selected documents.");
    await pick(app, paths); await page.getByRole("button", { name: "Attach files", exact: true }).click();
    const pending = page.getByRole("region", { name: "Attachments for your next message" });
    await expect(pending.locator(".inline-file-card")).toHaveCount(12);
    await expect(page.getByRole("button", { name: "Attach files", exact: true })).toBeDisabled();
    await composer.evaluate(element => {
      const transfer = new DataTransfer(); transfer.items.add(new File(["extra"], "extra.txt", { type: "text/plain" }));
      element.closest(".composer")!.dispatchEvent(new DragEvent("drop", { bubbles: true, cancelable: true, dataTransfer: transfer }));
    });
    await expect(page.locator(".composer-error")).toContainText("Attach up to 12 files");
    await expect(composer).toHaveValue("Read the selected documents.");
    await page.reload(); await expect(pending.locator(".inline-file-card")).toHaveCount(12);
    await expect(composer).toHaveValue("Read the selected documents.");
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(760, 560));
    await expect(page.getByRole("button", { name: "Send message", exact: true })).toBeInViewport();
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.screenshot({ path: "test-results/larger-attachment-selection.png", animations: "disabled" });
    expect(f.requests).toHaveLength(0);
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/mode inspect" }));
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect(page.locator(".assistant-message").getByText("I received the current file.", { exact: true })).toBeVisible({ timeout: 30000 });
    const payload = JSON.stringify(f.requests);
    expect(payload).toContain("amber heron"); expect(payload).toContain("Selected document 11");
    await expect(page.locator(".user-message .inline-file-card")).toHaveCount(12);
    expect(await readFile(paths[0], "utf8")).toBe(large);
  } finally { await f.close(); }
});
