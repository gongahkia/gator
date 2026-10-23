import { _electron as electron, test, expect } from "@playwright/test";
import { createServer } from "node:http";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { closeDesktop, temporaryDirectory } from "./lifecycle";
import { openCommandSearch } from "./navigation";

test("light appearance covers chat, settings, palette and native chrome across restart and cached chats",async()=>{
 test.setTimeout(120000);
 const directory=await temporaryDirectory(join(tmpdir(),"gator-light-"));
 const workspace=join(directory,"workspace"),configRoot=join(directory,"config/gator");
 await mkdir(workspace);await mkdir(configRoot,{recursive:true});
 const server=createServer(async(req,res)=>{
  for await(const _ of req) { /* Drain the local fixture request. */ }
  res.writeHead(200,{"Content-Type":"application/json"});
  res.end(JSON.stringify({choices:[{message:{role:"assistant",content:"## A place to think\n\nYour notes and plans stay together.\n\n- Review the brief\n- Share the next step\n\nUse `project.md` when you are ready."},finish_reason:"stop"}],usage:{prompt_tokens:8,completion_tokens:20}}));
 });
 await new Promise<void>(done=>server.listen(0,"127.0.0.1",done));
 const base=`http://127.0.0.1:${(server.address() as {port:number}).port}`;
 await writeFile(join(configRoot,"config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base,models:["fixture"],default_model:"fixture"}]}),{mode:0o600});
 const env=Object.fromEntries(Object.entries({...process.env,GATOR_INTERNAL_COMMANDS:"",GATOR_STATE_DIR:join(directory,"state"),GATOR_CONFIG_DIR:join(directory,"config"),GATOR_DESKTOP_USER_DATA:join(directory,"desktop"),GATOR_DESKTOP_WORKSPACE:workspace}).filter((entry):entry is [string,string]=>typeof entry[1]==="string"));
 const executablePath=process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
 const launch=()=>electron.launch({executablePath,args:executablePath?[]:["."],cwd:resolve("."),env});
 let app=await launch();
 try{
  let page=await app.firstWindow();
  await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toBeVisible();
  await page.getByRole("button",{name:"Settings",exact:true}).click();
  await page.getByRole("button",{name:"Appearance settings",exact:true}).click();
  await expect(page.getByRole("heading",{name:"Appearance",exact:true})).toBeVisible();
  await page.getByRole("button",{name:/^Light /}).click();
  await expect(page.locator(".app-shell")).toHaveAttribute("data-theme","light");
  await expect.poll(()=>app.evaluate(({nativeTheme})=>nativeTheme.shouldUseDarkColors)).toBe(false);
  expect(JSON.parse(await readFile(join(configRoot,"config.json"),"utf8")).theme).toBe("light");
  await page.screenshot({path:"test-results/appearance-light-settings.png",animations:"disabled"});
  await page.getByRole("button",{name:"Back to app",exact:true}).click();
  const composer=page.getByRole("textbox",{name:"Message Gator",exact:true});
  await composer.fill("Help me plan my week");await composer.press("Enter");
  await expect(page.getByRole("heading",{name:"A place to think",exact:true})).toBeVisible();
  await expect(page.getByRole("button",{name:"Copy response",exact:true})).toBeVisible();
  await composer.fill("Keep this draft");
  await page.getByRole("button",{name:"New chat",exact:true}).click();
  await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toHaveValue("");
  await expect(page.locator(".app-shell")).toHaveAttribute("data-theme","light");
  await page.getByRole("button",{name:"Open conversation: Help me plan my week",exact:true}).click();
  await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toHaveValue("Keep this draft");
  await expect(page.locator(".app-shell")).toHaveAttribute("data-theme","light");
  await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].setSize(760,560));
  await page.screenshot({path:"test-results/appearance-light-chat.png",animations:"disabled"});
  await openCommandSearch(page);
  await expect(page.getByRole("dialog",{name:"Search commands"})).toBeVisible();
  await page.screenshot({path:"test-results/appearance-light-palette.png",animations:"disabled"});
  await page.keyboard.press("Escape");
  expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
  await closeDesktop(app);app=await launch();page=await app.firstWindow();
  await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toHaveValue("Keep this draft");
  await expect(page.locator(".app-shell")).toHaveAttribute("data-theme","light");
  expect(await page.evaluate(()=>getComputedStyle(document.documentElement).colorScheme)).toBe("light");
  await page.getByRole("button",{name:"Settings",exact:true}).click();await page.getByRole("button",{name:"Appearance settings",exact:true}).click();
  await page.getByRole("button",{name:/^Dark /}).click();
  await expect(page.locator(".app-shell")).toHaveAttribute("data-theme","gator");
  await expect.poll(()=>app.evaluate(({nativeTheme})=>nativeTheme.shouldUseDarkColors)).toBe(true);
 }finally{await closeDesktop(app).catch(()=>{});server.closeAllConnections();await new Promise<void>(done=>server.close(()=>done()));await rm(directory,{recursive:true,force:true});}
});
