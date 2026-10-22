import { openCommandSearch } from "./navigation";
import { _electron as electron, expect, test, type ElectronApplication, type Page } from "@playwright/test";
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises";
import { createServer } from "node:http";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

type Message = { role: string; content: unknown; tool_calls?: {function:{name:string;arguments:string}}[] };
type Request = { messages: Message[]; tools?: {function:{name:string}}[] };
const quote = "I prefer concise answers with bullet points.";
const textOf = (message?: Message) => typeof message?.content === "string" ? message.content : JSON.stringify(message?.content ?? "");
const png = "iVBORw0KGgoAAAANSUhEUgAAABQAAAAUCAYAAACNiR0NAAAAHklEQVR4nGOYdXzpf2pihlEDRw0cNXDUwFEDR6qBAKwvuA3igqjbAAAAAElFTkSuQmCC";
const command = (page:Page,text:string) => page.evaluate(text=>window.gator!.send({type:text==="/chat"||text==="/memory"?"navigate":"submit",text}),text);
const snapshot = (page:Page) => page.evaluate(async()=>(await window.gator!.snapshot()).state!);
const composer = (page:Page) => page.getByRole("textbox",{name:"Message Gator",exact:true});
async function send(page:Page,text:string) { const previous=(await snapshot(page)).messages.filter(m=>m.role==="Gator").length;await composer(page).fill(text);await page.getByRole("button",{name:"Send message",exact:true}).click();await expect.poll(async()=>(await snapshot(page)).messages.filter(m=>m.role==="Gator").length).toBeGreaterThan(previous);await expect(page.getByRole("button",{name:"Stop current work",exact:true})).toHaveCount(0); }
async function fixture() {
  const directory=await temporaryDirectory(join(tmpdir(),"gator-shared-chats-")),workspace=join(directory,"workspace");
  await mkdir(workspace);await mkdir(join(directory,"config/gator"),{recursive:true});
  const requests:Request[]=[];
  const server=createServer((request,response)=>{let body="";request.on("data",chunk=>body+=chunk);request.on("end",()=>{
    const input:Request=JSON.parse(body);requests.push(input);
    const user=textOf(input.messages.filter(m=>m.role==="user").at(-1)),last=input.messages.at(-1);
    const call=(name:string,args:unknown)=>({role:"assistant",content:null,tool_calls:[{id:`call-${requests.length}`,type:"function",function:{name,arguments:JSON.stringify(args)}}]});
    let message:unknown={role:"assistant",content:last?.role==="tool"?"The requested memory or history operation finished.":"A fresh answer from the same Gator."};
    if(last?.role!=="tool" && user.includes(quote)) message=call("memory_remember",{subject:"preference.response_length",kind:"preference",quote,scope:"personal",explicit:user.includes("Remember")});
    if(last?.role!=="tool" && user.includes("Please forget my reply preference")) {
      const memory=input.messages.filter(m=>m.role==="system").map(textOf).join("\n").match(/\[(mem-[a-f0-9]+)\]/);
      message=call("memory_forget",{id:memory?.[1]??"missing",quote:"Please forget my reply preference."});
    }
    if(last?.role!=="tool" && user.includes("Find the earlier Birch Hall discussion")) message=call("chat_search",{query:"Birch Hall"});
    response.writeHead(200,{"Content-Type":"application/json"});response.end(JSON.stringify({choices:[{message,finish_reason:JSON.stringify(message).includes('"tool_calls"')?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10,total_tokens:20}}));
  });});
  await new Promise<void>(done=>server.listen(0,"127.0.0.1",done));const port=(server.address() as {port:number}).port;
  await writeFile(join(directory,"config/gator/config.json"),JSON.stringify({version:4,defaults:{provider:"chat-fixture",model:"fixture-model"},custom_providers:[{id:"chat-fixture",base_url:`http://127.0.0.1:${port}/v1/chat/completions`,models:["fixture-model"],default_model:"fixture-model"}]}),{mode:0o600});
  const env={...process.env,GATOR_STATE_DIR:join(directory,"state"),GATOR_CONFIG_DIR:join(directory,"config"),GATOR_DESKTOP_USER_DATA:join(directory,"desktop"),GATOR_DESKTOP_WORKSPACE:workspace};
  let app:ElectronApplication|undefined;
  const launch=async()=>{const executablePath=process.env.GATOR_DESKTOP_TEST_EXECUTABLE;app=await electron.launch({executablePath,args:executablePath?[]:["."],cwd:resolve("."),env});return app;};
  const memories=async()=>{const path=join(directory,"state/gator/memory");return Promise.all((await readdir(path)).filter(name=>name.endsWith(".json")).map(async name=>JSON.parse(await readFile(join(path,name),"utf8"))));};
  return {directory,requests,launch,memories,async restart(){await closeDesktop(app!);return launch();},async close(){if(app)await closeDesktop(app);server.closeAllConnections();await new Promise<void>(done=>server.close(()=>done()));await rm(directory,{recursive:true,force:true});}};
}

test("new chats share grounded personal memory, keep separate transcripts and reopen the last draft",async()=>{
  test.setTimeout(120_000);const f=await fixture();try {
    let app=await f.launch(),page=await app.firstWindow();const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await expect(composer(page)).toBeVisible();
    const first=(await snapshot(page)).conversation_id;expect(first).toBeTruthy();
    await command(page,"/mode inspect");await send(page,quote);
    await expect.poll(async()=>(await f.memories()).length).toBe(1);const memory=(await f.memories())[0];expect(memory.text).toBe(quote);expect(memory.origin.conversation_id).toBe(first);expect(memory.origin.quote).toBe(quote);
    await page.getByRole("button",{name:"New chat",exact:true}).click();await expect.poll(async()=>(await snapshot(page)).conversation_id).not.toBe(first);const second=(await snapshot(page)).conversation_id;await expect(composer(page)).toBeFocused();
    expect((await snapshot(page)).messages).toEqual([]);await command(page,"/mode inspect");const before=f.requests.length;await send(page,"Suggest a weekend activity.");
    const fresh=f.requests[before];expect(fresh.messages.filter(m=>m.role==="user").map(textOf).join("\n")).not.toContain(quote);expect(fresh.messages.filter(m=>m.role==="system").map(textOf).join("\n")).toContain(quote);
    await send(page,"The venue is Birch Hall.");await page.getByRole("button",{name:"New chat",exact:true}).click();await command(page,"/mode inspect");
    await send(page,"Find the earlier Birch Hall discussion.");expect(JSON.stringify(f.requests.at(-1))).toContain('historical reference only');expect(JSON.stringify(f.requests.at(-1))).toContain(second!);
    await send(page,"Please forget my reply preference.");await expect.poll(async()=>(await f.memories()).length).toBe(0);
    await page.getByRole("button",{name:"New chat",exact:true}).click();await command(page,"/mode inspect");const afterForget=f.requests.length;await send(page,"Suggest a weekend activity.");expect(f.requests[afterForget].messages.filter(m=>m.role==="system").map(textOf).join("\n")).not.toContain(quote);
    await command(page,"/memory");await expect(page.getByRole("button",{name:/turn chat learning off/i})).toBeVisible();await page.getByRole("button",{name:/turn chat learning off/i}).click();await expect(page.getByRole("button",{name:/turn chat learning on/i})).toBeVisible();await command(page,"/chat");
    await send(page,quote);expect(await f.memories()).toHaveLength(0);
    await send(page,`Remember that ${quote}`);await expect.poll(async()=>(await f.memories()).length).toBe(1);
    const previous=(await snapshot(page)).conversation_id;await page.keyboard.press(`${process.platform==="darwin"?"Meta":"Control"}+n`);await expect.poll(async()=>(await snapshot(page)).conversation_id).not.toBe(previous);const last=(await snapshot(page)).conversation_id;expect(last).not.toBe(first);expect((await snapshot(page)).messages).toEqual([]);
    await composer(page).fill("An independent unsent draft.");await page.evaluate(()=>window.gator!.drafts({action:"list"}));const count=f.requests.length;
    app=await f.restart();page=await app.firstWindow();await expect(composer(page)).toHaveValue("An independent unsent draft.");expect((await snapshot(page)).conversation_id).toBe(last);expect(f.requests).toHaveLength(count);
    await command(page,"/memory");await expect(page.getByRole("button",{name:/turn chat learning on/i})).toBeVisible();await command(page,"/chat");
    await page.getByRole("button",{name:`Open conversation: ${quote}`,exact:true}).click();await expect(composer(page)).toHaveValue("");expect((await snapshot(page)).conversation_id).toBe(first);
    expect(errors).toEqual([]);
  } finally {await f.close();}
});

async function syntheticImage(page:Page,type="image/png",bytes?:number[],plain="") {
  await composer(page).evaluate((element,{type,bytes,plain,png})=>{const transfer=new DataTransfer();transfer.items.add(new File([bytes?new Uint8Array(bytes):Uint8Array.from(atob(png),c=>c.charCodeAt(0))],"clipboard.png",{type}));if(plain)transfer.setData("text/plain",plain);element.dispatchEvent(new ClipboardEvent("paste",{bubbles:true,cancelable:true,clipboardData:transfer}));},{type,bytes,plain,png});
}

test("native image paste stages a preview, retains drafts per chat, and sends only after review",async()=>{
  test.setTimeout(120_000);const f=await fixture();let clipboardOwned=false,clipboardApp:ElectronApplication|undefined;try{
    let app=await f.launch(),page=await app.firstWindow();const errors:string[]=[];page.on("pageerror",e=>errors.push(e.message));await expect(composer(page)).toBeVisible();await composer(page).fill("Review this screenshot.");
    // The previous clipboard remains only in the test main process, is never
    // logged or serialized to the runner, and is restored as soon as paste ends.
    clipboardApp=app;clipboardOwned=true;await app.evaluate(async({clipboard,ClipboardItem,nativeImage},png)=>{const items=await clipboard.read();(globalThis as any).__gatorClipboard=await Promise.all(items.map(async item=>new ClipboardItem(Object.fromEntries(await Promise.all(item.types.map(async type=>[type,await item.getType(type)]))))));await clipboard.write([new ClipboardItem({"image/png":new Blob([new Uint8Array(Buffer.from(png,"base64"))],{type:"image/png"})})]);const written=(await clipboard.read())[0];const data=await written.getType("image/png");if(nativeImage.createFromBuffer(Buffer.from(await data.arrayBuffer())).getSize().width!==20)throw new Error("Native clipboard did not accept the test image");},png);
    await composer(page).focus();await page.keyboard.press(`${process.platform==="darwin"?"Meta":"Control"}+v`);
    const pending=page.getByRole("region",{name:"Attachments for your next message",exact:true});await expect(pending.locator(".inline-file")).toHaveCount(1);
    await app.evaluate(async({clipboard})=>{const saved=(globalThis as any).__gatorClipboard;if(saved.length)await clipboard.write(saved);else clipboard.clear();delete (globalThis as any).__gatorClipboard;});clipboardOwned=false;
    await expect(composer(page)).toHaveValue("Review this screenshot.");expect(f.requests).toHaveLength(0);
    const name=(await page.evaluate(()=>window.gator!.drafts({action:"list"}))).drafts.find(d=>d.uploads?.length)?.uploads![0].name!;
    await pending.getByRole("button",{name:`Open file ${name}`,exact:true}).click();await expect.poll(()=>pending.getByRole("img").evaluate((img:HTMLImageElement)=>img.naturalWidth)).toBe(20);
    await page.screenshot({path:"test-results/chat-pasted-image.png"});
    const first=(await snapshot(page)).conversation_id;await page.getByRole("button",{name:"New chat",exact:true}).click();await expect(pending).toHaveCount(0);await expect(composer(page)).toHaveValue("");
    await page.getByRole("button",{name:"Open conversation: New chat",exact:true}).and(page.locator('button:not([aria-current="page"])')).first().click();await expect.poll(async()=>(await snapshot(page)).conversation_id).toBe(first);await expect(pending.locator(".inline-file")).toHaveCount(1);await expect(composer(page)).toHaveValue("Review this screenshot.");
    await page.evaluate(()=>window.gator!.drafts({action:"list"}));app=await f.restart();page=await app.firstWindow();await expect(composer(page)).toHaveValue("Review this screenshot.");await expect(page.getByRole("region",{name:"Attachments for your next message"}).locator(".inline-file")).toHaveCount(1);
    await command(page,"/mode inspect");await page.getByRole("button",{name:"Send message",exact:true}).click();await expect.poll(()=>f.requests.length).toBeGreaterThan(0);expect(JSON.stringify(f.requests[0])).toContain("data:image/png;base64,");await expect(page.getByRole("button",{name:"Stop current work",exact:true})).toHaveCount(0);
    await composer(page).fill("Keep this text.");await syntheticImage(page,"image/svg+xml",[60,115,118,103,62]);await expect(page.locator(".composer-error")).toContainText("PNG, JPEG or WebP");await expect(composer(page)).toHaveValue("Keep this text.");
    await syntheticImage(page,"image/png",undefined," appended text");await expect(composer(page)).toHaveValue("Keep this text. appended text");
    await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].setSize(800,650));await page.emulateMedia({reducedMotion:"reduce"});expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:"test-results/chat-pasted-image-narrow.png"});expect(errors).toEqual([]);
  } finally {if(clipboardOwned){await clipboardApp!.evaluate(async({clipboard})=>{const saved=(globalThis as any).__gatorClipboard;if(saved){if(saved.length)await clipboard.write(saved);else clipboard.clear();delete (globalThis as any).__gatorClipboard;}}).catch(()=>{});}await f.close();}
});


test("new chat and pasted attachments keep focus and layout across themes and compact windows",async()=>{
  test.setTimeout(90_000);const f=await fixture();try {
    const app=await f.launch(),page=await app.firstWindow();await expect(composer(page)).toBeVisible();
    const newChat=page.getByRole("button",{name:"New chat",exact:true});
    for(const theme of ["gator","contrast","mono"]){
      await command(page,"/theme "+theme);await expect(page.locator(".app-shell")).toHaveAttribute("data-theme",theme);
      await newChat.hover();await newChat.focus();await expect(newChat).toBeFocused();expect((await newChat.boundingBox())!.height).toBeGreaterThanOrEqual(40);
      await newChat.press("Enter");await expect(composer(page)).toBeFocused();await composer(page).fill("Keep my draft while attaching.");await syntheticImage(page);
      const pending=page.getByRole("region",{name:"Attachments for your next message"});await expect(pending.locator(".inline-file")).toHaveCount(1);
      for(const width of [1440,800]){await app.evaluate(({BrowserWindow},width)=>BrowserWindow.getAllWindows()[0].setSize(width,700),width);expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);await page.screenshot({path:`test-results/shared-chat-${theme}-${width}.png`});}
    }
    await page.emulateMedia({reducedMotion:"reduce"});await page.getByRole("button",{name:"Collapse sidebar",exact:true}).click();await expect(page.getByRole("button",{name:"Expand sidebar",exact:true})).toBeVisible();await newChat.press("Enter");await expect(composer(page)).toBeFocused();await expect(page.getByRole("region",{name:"Attachments for your next message"})).toHaveCount(0);
    await openCommandSearch(page);const search=page.getByRole("textbox",{name:"Find a command",exact:true});await search.fill("/new");await expect(page.locator(".command-list button")).toHaveCount(1);await search.fill("new chat");await expect(page.locator(".command-list button")).toHaveCount(1);await page.keyboard.press("Escape");expect(f.requests).toHaveLength(0);
  }finally{await f.close();}
});

test("earlier-chat privacy is independent of learning and persists across restart",async()=>{
  test.setTimeout(120_000);const f=await fixture();try{
    let app=await f.launch(),page=await app.firstWindow();await expect(composer(page)).toBeVisible();
    await command(page,"/mode inspect");await send(page,quote);
    await page.getByRole("button",{name:"Settings",exact:true}).click();
    await page.getByRole("button",{name:"Memory and privacy settings",exact:true}).click();
    await page.getByRole("button",{name:/turn earlier-chat retrieval off/i}).click();
    await expect(page.getByRole("button",{name:/turn earlier-chat retrieval on/i})).toBeVisible();
    await expect(page.getByRole("button",{name:/turn chat learning off/i})).toBeVisible();
    await page.screenshot({path:"test-results/memory-privacy.png"});
    await page.getByRole("button",{name:"Back to app",exact:true}).click();
    const first=(await snapshot(page)).conversation_id;await page.getByRole("button",{name:"New chat",exact:true}).click();await expect.poll(async()=>(await snapshot(page)).conversation_id).not.toBe(first);await expect(composer(page)).toBeEnabled();await command(page,"/mode inspect");
    const before=f.requests.length;await send(page,"What do you know about my reply preferences?");
    const request=f.requests[before];const names=request.tools?.map(tool=>tool.function.name)??[];
    expect(names).not.toContain("chat_search");expect(names).not.toContain("chat_read");expect(names).toContain("memory_remember");
    expect(request.messages.filter(m=>m.role==="system").map(textOf).join("\n")).toContain(quote);
    await expect.poll(async()=>(await page.evaluate(()=>window.gator!.snapshot())).allChatsBusy).toBe(false);
    app=await f.restart();page=await app.firstWindow();await expect(composer(page)).toBeVisible();
    await command(page,"/memory");await expect(page.getByRole("button",{name:/turn earlier-chat retrieval on/i})).toBeVisible();
    await page.getByRole("button",{name:/turn earlier-chat retrieval on/i}).click();
    await page.getByRole("button",{name:"Back to app",exact:true}).click();
    const restored=f.requests.length;await send(page,"Find the earlier Birch Hall discussion.");
    expect(f.requests[restored].tools?.map(tool=>tool.function.name)).toContain("chat_search");
    expect((await f.memories())[0].text).toBe(quote);
  }finally{await f.close();}
});
