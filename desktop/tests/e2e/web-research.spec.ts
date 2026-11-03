import { expect, test } from "@playwright/test";
import { createServer } from "node:http";
import { mkdir, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, launchDesktop, temporaryDirectory } from "./lifecycle";

// Real native approval/operation journey, with no external web/model traffic.
// Native search/fetch and evidence behavior is separately exercised by Work tests.
test("public web scope is reviewed with limits and expires between requests", async () => {
  test.setTimeout(90000);
  const root = await temporaryDirectory(join(tmpdir(), "gator-web-review-"));
  const workspace = join(root, "workspace"), config = join(root, "config/gator");
  await mkdir(workspace); await mkdir(config, { recursive: true });
  const observations: {public_web:boolean; requests_used:number; max_requests:number; max_searches:number}[] = [];
  const server = createServer(async (request, response) => {
    let body = ""; for await (const chunk of request) body += chunk;
    const payload = JSON.parse(body);
    type Message = {role:string;content:string;tool_call_id?:string};
    const messages = payload.messages as Message[];
    let lastUser = messages.length - 1;
    while (lastUser >= 0 && messages[lastUser].role !== "user") lastUser--;
    const current = messages.slice(lastUser + 1);
    const result = (id:string) => current.find(m => m.role === "tool" && m.tool_call_id === id);
    const call = (id:string,args:unknown) => ({role:"assistant",tool_calls:[{id,type:"function",function:{name:"web_access",arguments:JSON.stringify(args)}}]});
    let message;
    if (!result("grant")) message = call("grant",{action:"request",scope:"public",reason:"Find relevant public sources beyond manually selected websites."});
    else if (!result("status")) message = call("status",{action:"status"});
    else {
      const status = JSON.parse(result("status")!.content);
      observations.push(status);
      message = {role:"assistant",content: status.public_web ? "Public research is available for this task, within the reviewed shared limits." : "Public web access was declined. I can continue with local evidence."};
    }
    response.setHeader("Content-Type","application/json");
    response.end(JSON.stringify({choices:[{message,finish_reason:"tool_calls" in message?"tool_calls":"stop"}],usage:{prompt_tokens:10,completion_tokens:10}}));
  });
  await new Promise<void>(done => server.listen(0,"127.0.0.1",done));
  const base = `http://127.0.0.1:${(server.address() as {port:number}).port}`;
  await writeFile(join(config,"config.json"),JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base,models:["fixture"],default_model:"fixture"}]}),{mode:0o600});
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await launchDesktop({executablePath,args:executablePath?[]:["."],cwd:resolve("."),env:{...process.env,GATOR_INTERNAL_COMMANDS:"",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config"),GATOR_DESKTOP_USER_DATA:join(root,"desktop"),GATOR_DESKTOP_WORKSPACE:workspace,GATOR_WEB_SEARCH_PROVIDER:"",BRAVE_SEARCH_API_KEY:"",EXA_API_KEY:"",TAVILY_API_KEY:"",TINYFISH_API_KEY:""}});
  try {
    const page = await app.firstWindow(), composer = page.getByRole("textbox",{name:"Message Gator",exact:true});
    await expect(composer).toBeVisible();
    await composer.fill("/mode inspect"); await composer.press("Enter"); await expect(composer).toHaveValue("");
    const send = async (text:string) => { await composer.fill(text); await page.getByRole("button",{name:"Send message",exact:true}).click(); };
    await send("Research the public web for this project.");
    const review = page.getByRole("region",{name:"Review web research access",exact:true});
    await expect(review).toBeVisible();
    await expect(review).toContainText("Public HTTPS websites");
    await expect(review).toContainText("8 search requests within 32 total web requests");
    await expect(review).toContainText("no search provider configured");
    await expect(review).toContainText("provider charges");
    expect(observations).toHaveLength(0);
    await page.getByRole("button",{name:"Approve this action",exact:true}).click();
    await expect(page.getByRole("article",{name:"Gator message",exact:true}).last()).toContainText("Public research is available");
    await expect.poll(async () => (await windowState()).running).toBe(false);
    expect(observations).toEqual([expect.objectContaining({public_web:true,requests_used:0,max_requests:32,max_searches:8})]);
    await send("Research the public web again as a new task.");
    await expect(page.getByRole("button",{name:"Approve this action",exact:true})).toBeVisible();
    expect(observations).toHaveLength(1);
    await page.getByRole("button",{name:"Deny",exact:true}).click();
    await expect(page.getByRole("article",{name:"Gator message",exact:true}).last()).toContainText("Public web access was declined");
    await expect.poll(async () => (await windowState()).running).toBe(false);
    expect(observations[1]).toMatchObject({public_web:false,requests_used:0});
    async function windowState() { return page.evaluate(async () => (await window.gator!.snapshot()).state!); }
  } finally {
    await closeDesktop(app); server.closeAllConnections(); await new Promise<void>(done => server.close(() => done())); await rm(root,{recursive:true,force:true});
  }
});
