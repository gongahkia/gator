import { _electron as electron, test, expect, type Page } from "@playwright/test";
import { createServer } from "node:http";
import { mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, temporaryDirectory } from "./lifecycle";
const state = (page: Page) => page.evaluate(async () => (await window.gator!.snapshot()).state!);

test("native account setup reviews credentials, verifies access and returns to browser sign-in", async () => {
 test.setTimeout(120000);
 const directory = await temporaryDirectory(join(tmpdir(),"gator-connections-"));
 const workspace = join(directory,"workspace"), configRoot = join(directory,"config/gator");
 await mkdir(workspace); await mkdir(configRoot,{recursive:true});
 let reads=0,tokenRequests=0;
 const server=createServer((req,res)=>{
  if(req.url==="/resource") {reads++;expect(req.headers.authorization).toBe("Bearer private-service-token");res.writeHead(200,{"Content-Type":"application/json"});res.end('{"value":1}');return;}
  if(req.url==="/token") {tokenRequests++;res.writeHead(200,{"Content-Type":"application/json"});res.end('{"access_token":"private-oauth-access","refresh_token":"private-refresh","token_type":"Bearer","expires_in":3600}');return;}
  res.writeHead(404);res.end();
 });
 await new Promise<void>(done=>server.listen(0,"127.0.0.1",done));
 const base=`http://127.0.0.1:${(server.address() as {port:number}).port}`;
 await writeFile(join(configRoot,"config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base+"/chat",models:["fixture"],default_model:"fixture"}],connectors:[{version:1,id:"google",name:"Personal Google",kind:"google_workspace",resource:base,authentication:"oauth",oauth_client_id:"desktop-client",oauth_authorize_url:base+"/authorize",oauth_token_url:base+"/token",oauth_redirect_url:"http://127.0.0.1:1457/oauth/callback",oauth_scopes:"openid https://www.googleapis.com/auth/userinfo.email https://www.googleapis.com/auth/gmail.readonly"}]}),{mode:0o600});
 const env=Object.fromEntries(Object.entries({...process.env,GATOR_INTERNAL_COMMANDS:"",GATOR_STATE_DIR:join(directory,"state"),GATOR_CONFIG_DIR:join(directory,"config"),GATOR_DESKTOP_USER_DATA:join(directory,"desktop"),GATOR_DESKTOP_WORKSPACE:workspace}).filter((entry):entry is [string,string]=>typeof entry[1]==="string"));
 const executablePath=process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
 const app=await electron.launch({executablePath,args:executablePath?[]:["."],cwd:resolve("."),env});
 try {
  const page=await app.firstWindow();await expect.poll(async()=>Boolean(await state(page))).toBe(true);
  await page.getByRole("button",{name:"Settings",exact:true}).click();
  await page.getByRole("button",{name:"Connection settings",exact:true}).click();
  await expect(page.getByRole("heading",{name:"Connections",exact:true})).toBeVisible();
  await page.getByRole("button",{name:"Add a service token connection",exact:true}).click();
  await page.getByLabel("Connection ID",{exact:true}).fill("metrics");await page.getByLabel("Display name",{exact:true}).fill("Team metrics");await page.getByLabel("Service",{exact:true}).selectOption("http_json");await page.getByLabel("Resource URL",{exact:true}).fill(base+"/resource");
  const review=()=>page.getByRole("button",{name:/^Review/}).click();
  await review();expect(JSON.parse(await readFile(join(configRoot,"config.json"),"utf8")).connectors).toHaveLength(1);
  await page.locator(".workflow-footer").getByRole("button",{name:/^Add a service token connection/}).click();
  await page.getByRole("button",{name:"Set access token",exact:true}).click();await page.getByLabel("Access token",{exact:true}).fill("private-service-token");
  await review();expect(JSON.stringify(await state(page))).not.toContain("private-service-token");expect(reads).toBe(0);
  await page.locator(".workflow-footer").getByRole("button",{name:/^Replace credentials/}).click();
  await page.getByRole("button",{name:"Check account & read access",exact:true}).click();await expect(page.getByRole("status").filter({hasText:"Read access verified just now"})).toBeVisible();expect(reads).toBe(1);
  await page.getByRole("button",{name:"Disconnect account",exact:true}).click();await page.locator(".workflow-footer").getByRole("button",{name:/^Disconnect account/}).click();
  await expect(page.getByText("Not connected",{exact:true})).toBeVisible();expect(JSON.stringify(await state(page))).not.toContain("private-service-token");
  await page.getByRole("button",{name:"Back to connections",exact:true}).click();await page.getByRole("button",{name:/^Personal Google/}).click();
  await page.getByRole("button",{name:"Sign in",exact:true}).click();await review();await page.locator(".workflow-footer").getByRole("button",{name:/^Sign in/}).click();
  await expect(page.getByRole("button",{name:"Continue in browser",exact:true})).toBeVisible();
  const authURL=(await state(page)).panel!.links![0].url;
  await app.evaluate(({shell},expected)=>{shell.openExternal=async(url)=>{if(url!==expected)throw new Error("Wrong authorization URL");process.env.GATOR_TEST_BROWSER_OPENED="yes";};},authURL);
  await page.getByRole("button",{name:"Continue in browser",exact:true}).click();
  await expect.poll(()=>app.evaluate(()=>process.env.GATOR_TEST_BROWSER_OPENED)).toBe("yes");
  await page.getByRole("button",{name:"Back to app",exact:true}).click();await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toBeVisible();
  await page.getByRole("button",{name:"Settings",exact:true}).click();await page.getByRole("button",{name:"Connection settings",exact:true}).click();
  await page.getByRole("button",{name:/^Personal Google/}).click();await expect(page.getByRole("button",{name:"Cancel sign-in",exact:true})).toBeVisible();
  const authorize=new URL(authURL),callback=new URL(authorize.searchParams.get("redirect_uri")!);callback.searchParams.set("state",authorize.searchParams.get("state")!);callback.searchParams.set("code","authorized-fixture");await fetch(callback).catch(()=>{});
  await expect(page.getByRole("button",{name:"Reconnect account",exact:true})).toBeVisible();expect(tokenRequests).toBe(1);expect(JSON.stringify(await state(page))).not.toContain("private-oauth-access");
  await app.evaluate(({BrowserWindow})=>BrowserWindow.getAllWindows()[0].setSize(980,780));
  await page.screenshot({path:"test-results/connections-account.png"});
  await closeDesktop(app);
 } finally {
  await app.evaluate(({dialog})=>{dialog.showMessageBox=async()=>({response:0,checkboxChecked:false});}).catch(()=>{});await closeDesktop(app).catch(()=>{});server.closeAllConnections();await new Promise<void>(done=>server.close(()=>done()));await rm(directory,{recursive:true,force:true});
 }
});
