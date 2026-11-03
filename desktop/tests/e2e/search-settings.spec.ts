import { expect, test, type Page } from "@playwright/test";
import { createServer } from "node:http";
import { mkdir, readFile, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, launchDesktop, temporaryDirectory } from "./lifecycle";

const state = (page:Page) => page.evaluate(async () => (await window.gator!.snapshot()).state!);

test("native search setup keeps keys private, changes task tools and persists across restart", async () => {
  test.setTimeout(120000);
  const root = await temporaryDirectory(join(tmpdir(),"gator-search-settings-"));
  const workspace = join(root,"workspace"), config = join(root,"config/gator");
  await mkdir(workspace); await mkdir(config,{recursive:true});
  const requests: {tools?:{function:{name:string}}[];messages:unknown[]}[] = [];
  const server = createServer(async (request,response) => {
    let body = ""; for await (const chunk of request) body += chunk;
    requests.push(JSON.parse(body));
    response.setHeader("Content-Type","application/json");
    response.end(JSON.stringify({choices:[{message:{role:"assistant",content:"The current task tool configuration was inspected."},finish_reason:"stop"}],usage:{prompt_tokens:10,completion_tokens:10}}));
  });
  await new Promise<void>(done => server.listen(0,"127.0.0.1",done));
  const base = `http://127.0.0.1:${(server.address() as {port:number}).port}`;
  const configPath = join(config,"config.json");
  await writeFile(configPath,JSON.stringify({version:4,defaults:{provider:"fixture",model:"fixture"},custom_providers:[{id:"fixture",base_url:base,models:["fixture"],default_model:"fixture"}]}),{mode:0o600});
  const secret = "synthetic-private-search-key";
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const env = {...process.env,GATOR_INTERNAL_COMMANDS:"",GATOR_STATE_DIR:join(root,"state"),GATOR_CONFIG_DIR:join(root,"config"),GATOR_DESKTOP_USER_DATA:join(root,"desktop"),GATOR_DESKTOP_WORKSPACE:workspace,GATOR_WEB_SEARCH_PROVIDER:"",BRAVE_SEARCH_API_KEY:"synthetic-environment-brave",TINYFISH_API_KEY:"synthetic-environment-tinyfish",EXA_API_KEY:"",TAVILY_API_KEY:""};
  const launch = () => launchDesktop({executablePath,args:executablePath?[]:["."],cwd:resolve("."),env});
  let app = await launch();
  try {
    let page = await app.firstWindow();
    const open = async () => {
      await page.getByRole("button",{name:"Settings",exact:true}).click();
      await page.getByRole("button",{name:"Search settings",exact:true}).click();
      await expect(page.getByRole("heading",{name:"Web search",exact:true})).toBeVisible();
    };
    const choose = async (key:string) => {
      await page.getByRole("button",{name:/^Choose search service/}).click();
      await page.getByRole("combobox",{name:"Search service",exact:true}).selectOption("tinyfish");
      const input = page.getByLabel("API key",{exact:true});
      await expect(input).toHaveAttribute("type","password");
      if (key) await input.fill(key);
      expect(JSON.stringify(await state(page))).not.toContain(secret);
      await page.locator(".workflow-footer").getByRole("button",{name:/^Review/}).click();
      await expect(page.getByText("New credential (hidden)",{exact:true})).toBeVisible();
      expect(JSON.stringify(await state(page))).not.toContain(secret);
      await page.locator(".workflow-footer").getByRole("button",{name:/^Choose search service/}).click();
      await expect(page.getByRole("heading",{name:"Web search",exact:true})).toBeVisible();
    };
    const task = async () => {
      await page.getByRole("button",{name:"Back to app",exact:true}).click();
      const composer = page.getByRole("textbox",{name:"Message Gator",exact:true});
      await composer.fill("/mode inspect"); await composer.press("Enter"); await expect(composer).toHaveValue("");
      await composer.fill("Inspect this task configuration."); await page.getByRole("button",{name:"Send message",exact:true}).click();
      await expect(page.getByRole("article",{name:"Gator message",exact:true}).last()).toContainText("current task tool configuration");
      await expect.poll(async () => (await state(page)).running).toBe(false);
    };
    await expect(page.getByRole("textbox",{name:"Message Gator",exact:true})).toBeVisible();
    await open(); await choose(secret);
    await expect(page.getByText("saved on this computer",{exact:true})).toBeVisible();
    expect(JSON.parse(await readFile(configPath,"utf8")).search).toEqual({provider:"tinyfish"});
    expect(await readFile(configPath,"utf8")).not.toContain(secret);
    expect(requests).toHaveLength(0);
    await task();
    expect(requests[0].tools?.some(tool => tool.function.name === "web_search")).toBe(true);
    expect(JSON.stringify(requests)).not.toContain(secret);
    await open();
    await page.getByRole("button",{name:/^Turn off web search/}).click();
    await expect.poll(async () => JSON.parse(await readFile(configPath,"utf8")).search.provider).toBe("off");
    await task();
    expect(requests[1].tools?.some(tool => tool.function.name === "web_search")).toBe(false);
    expect(requests[1].tools?.some(tool => tool.function.name === "http_fetch")).toBe(true);
    await open(); await choose(""); // Reuse the saved key without re-entering it.
    await closeDesktop(app); app = await launch(); page = await app.firstWindow();
    await expect(page.getByRole("button",{name:"Settings",exact:true})).toBeVisible();
    await open();
    await expect(page.getByText("saved on this computer",{exact:true})).toBeVisible();
    expect(JSON.stringify(await state(page))).not.toContain(secret);
    await page.getByRole("button",{name:/^Remove a saved search key/}).click();
    await page.locator(".workflow-footer").getByRole("button",{name:/^Review/}).click();
    await page.locator(".workflow-footer").getByRole("button",{name:/^Remove a saved search key/}).click();
    await expect.poll(async () => JSON.parse(await readFile(configPath,"utf8")).search.provider).toBe("off");
    expect(await readFile(join(root,"state/gator/auth.json"),"utf8")).not.toContain(secret);
    expect((await state(page)).provider).toBe("fixture");
  } finally {
    await closeDesktop(app); server.closeAllConnections(); await new Promise<void>(done => server.close(() => done())); await rm(root,{recursive:true,force:true});
  }
});
