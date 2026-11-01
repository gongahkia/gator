import { _electron as electron, test, expect, type ElectronApplication } from "@playwright/test";
import { createServer } from "node:http";
import { execFileSync } from "node:child_process";
import { mkdir, writeFile, rm } from "node:fs/promises";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { closeDesktop, temporaryDirectory } from "./lifecycle";
import { draftLinks, markdownLink } from "../../src/draftLinks";

test("draft links preserve punctuation and restrict edits to safe web links", () => {
  expect(draftLinks("Read https://example.com/a. Then https://example.com/a_(b). ")[0].url).toBe("https://example.com/a");
  expect(draftLinks("Read https://example.com/a. Then https://example.com/a_(b). ")[1].url).toBe("https://example.com/a_(b)");
  expect(draftLinks("javascript:alert(1) https://user:secret@example.com/")).toHaveLength(0);
  expect(markdownLink("Repository", "https://github.com/pingdotgg/t3code")).toBe("[Repository](https://github.com/pingdotgg/t3code)");
  expect(() => markdownLink("unsafe", "javascript:alert(1)")).toThrow();
});

async function fixture() {
  const root = await temporaryDirectory(join(tmpdir(), "gator-source-links-")), workspace = join(root, "workspace");
  await mkdir(workspace); await mkdir(join(root, "config/gator"), { recursive: true });
  await writeFile(join(workspace, "project.md"), "Project: Willow\nOwner: Mina\nDeadline: Friday\nBudget: unapproved\n");
  let calls = 0;
  const server = createServer((request, response) => {
    let body = ""; request.on("data", chunk => body += chunk); request.on("end", () => {
      calls++; const payload = JSON.parse(body), tool = payload.messages.at(-1)?.role === "tool";
      response.writeHead(200, { "Content-Type": "application/json" });
      const message = tool ? { role: "assistant", content: "**Owner:** Mina. **Deadline:** Friday.\n\n**Source:** `source/project.md`, lines 2–3.\n\n[Project site](https://example.com/project)" } : { role: "assistant", tool_calls: [{ id: "read-selected", type: "function", function: { name: "read_file", arguments: JSON.stringify({ path: "source/project.md", start_line: 2, end_line: 3 }) } }] };
      response.end(JSON.stringify({ choices: [{ message, finish_reason: tool ? "stop" : "tool_calls" }], usage: { prompt_tokens: 20, completion_tokens: 20 } }));
    });
  });
  await new Promise<void>(done => server.listen(0, "127.0.0.1", done));
  await writeFile(join(root, "config/gator/config.json"), JSON.stringify({ version: 4, defaults: { provider: "source-fixture", model: "fixture" }, custom_providers: [{ id: "source-fixture", base_url: `http://127.0.0.1:${(server.address() as { port: number }).port}/v1/chat/completions`, models: ["fixture"], default_model: "fixture" }] }));
  const env = { ...process.env, GATOR_STATE_DIR: join(root, "state"), GATOR_CONFIG_DIR: join(root, "config"), GATOR_DESKTOP_USER_DATA: join(root, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace };
  // Validate the disposable profile before launching a GUI. A fixture error
  // should fail this test in the log, not repeatedly show native startup alerts.
  execFileSync(resolve("runtime", process.platform === "win32" ? "gator-runtime.exe" : "gator-runtime"), ["config", "show"], { env: { ...env, GATOR_INTERNAL_COMMANDS: "1" }, stdio: "ignore" });
  let app: ElectronApplication;
  return { workspace, calls: () => calls, async launch() { const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE; app = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env }); return app; }, async close() { try { if (app!) { await app.evaluate(({ dialog }) => { dialog.showMessageBox = async () => ({ response: 1, checkboxChecked: false }); }).catch(() => {}); await closeDesktop(app); } } finally { server.closeAllConnections(); await new Promise<void>(done => server.close(() => done())); await rm(root, { recursive: true, force: true }); } } };
}

test("source citations open the right sidebar with original evidence and the actual read range", async () => {
  const f = await fixture(); try {
    const app = await f.launch(), page = await app.firstWindow(), composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeEnabled(); await page.evaluate(() => window.gator!.send({ type: "submit", text: "/mode inspect" }));
    await composer.fill("Read the selected project and cite its owner and deadline."); await page.getByRole("button", { name: "Send message", exact: true }).click();
    const reference = page.locator(".assistant-message").getByRole("button", { name: "source/project.md", exact: true }); await expect(reference).toBeVisible();
    await expect.poll(async () => (await page.evaluate(() => window.gator!.snapshot())).state!.running).toBe(false);
    await writeFile(join(f.workspace, "project.md"), "Changed current file: owner Eve, deadline Tuesday.");
    await composer.fill("Keep this draft"); const before = f.calls(); await reference.click();
    const sidebar = page.getByRole("complementary", { name: "Source preview" }); await expect(sidebar).toBeVisible();
    await expect(sidebar).toContainText("Read by Gator: 2–3"); await expect(sidebar).toContainText("Cited: lines 2–3");
    await expect(sidebar).toContainText("Owner: Mina"); await expect(sidebar).not.toContainText("owner Eve");
    await expect(sidebar.locator('[data-cited="true"]')).toHaveCount(2); await expect(sidebar.locator('[data-read="true"]')).toHaveCount(2);
    await page.screenshot({ path: "test-results/source-preview-retained.png" });
    await page.getByRole("button", { name: "Close source preview" }).click(); await expect(composer).toHaveValue("Keep this draft"); expect(f.calls()).toBe(before);
    const context = (await page.evaluate(() => window.gator!.snapshot())).state!;
    const run = context.messages.find(message => message.bundle)?.bundle?.RunID;
    expect(await page.evaluate(async ({ run }) => { try { await window.gator!.sourcePreview({ context: "conversation:other", run: run!, path: "source/project.md" }); return "opened"; } catch { return "denied"; } }, { run })).toBe("denied");
  } finally { await f.close(); }
});

test("effort level transitions reverse cleanly and reduced motion removes interpolation", async () => {
  const f = await fixture(); try {
    const app = await f.launch(), page = await app.firstWindow(); await page.emulateMedia({ reducedMotion: "no-preference" });
    const trigger = page.getByRole("button", { name: /^Model and work effort:/ }); await trigger.click();
    const slider = page.getByRole("slider", { name: "Work effort" }); await slider.focus(); await slider.press("End");
    await expect(slider).toHaveAttribute("aria-valuetext", "High");
    expect(await page.locator(".effort-fill").evaluate(element => getComputedStyle(element).transitionProperty)).toContain("transform");
    await page.locator(".effort-slider").evaluate(element => { for (const animation of element.getAnimations({ subtree: true })) animation.updatePlaybackRate(.1); });
    await expect(slider).toBeEnabled(); await slider.press("Home"); await expect(slider).toHaveAttribute("aria-valuetext", "Low");
    await expect.poll(() => page.locator(".effort-fill").evaluate(element => element.getBoundingClientRect().width)).toBeLessThan(1);
    await expect(slider).toBeEnabled(); await page.emulateMedia({ reducedMotion: "reduce" }); await slider.press("End");
    await expect(slider).toHaveAttribute("aria-valuetext", "High");
    expect(await page.locator(".effort-thumb").evaluate(element => getComputedStyle(element).transitionDuration)).toBe("0s");
    await page.screenshot({ path: "test-results/effort-level-high.png" }); expect(f.calls()).toBe(0);
  } finally { await f.close(); }
});

test("pasted links have reviewed edit controls, survive drafts and render sent prompts as Markdown", async () => {
  const f = await fixture(); try {
    const app = await f.launch(), page = await app.firstWindow(), composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(composer).toBeEnabled();
    await app.evaluate(({ clipboard }) => clipboard.writeText("https://github.com/pingdotgg/t3code"));
    await composer.focus(); await composer.press(process.platform === "darwin" ? "Meta+V" : "Control+V");
    await expect(composer).toHaveValue("https://github.com/pingdotgg/t3code");
    const link = () => page.locator(".composer-link-text").getByRole("link"); await link().click();
    const menu = () => page.getByRole("dialog", { name: "Link options" }); await expect(menu().getByRole("button", { name: "Open link", exact: true })).toBeVisible();
    await menu().getByRole("button", { name: "Edit text", exact: true }).click(); await menu().getByRole("textbox", { name: "Link text", exact: true }).fill("Project repository"); await menu().getByRole("button", { name: "Save link" }).click();
    await expect(composer).toHaveValue("[Project repository](https://github.com/pingdotgg/t3code)");
    await link().click(); await menu().getByRole("button", { name: "Edit link", exact: true }).click(); await menu().getByRole("textbox", { name: "Link address" }).fill("javascript:alert(1)"); await menu().getByRole("button", { name: "Save link" }).click(); await expect(menu().getByRole("alert")).toBeVisible();
    await menu().getByRole("textbox", { name: "Link address" }).fill("https://example.com/review"); await menu().getByRole("button", { name: "Save link" }).click();
    await expect(composer).toHaveValue("[Project repository](https://example.com/review)"); expect(f.calls()).toBe(0);
    await page.screenshot({ path: "test-results/composer-pasted-link.png" });
    await link().click(); await menu().getByRole("button", { name: "Edit text", exact: true }).click();
    await page.getByRole("button", { name: "New chat", exact: true }).click(); await expect(menu()).toHaveCount(0); await expect(composer).toHaveValue("");
    await page.evaluate(() => window.gator!.send({ type: "submit", text: "/mode inspect" }));
    await composer.fill("**Review this** [Project repository](https://example.com/review)"); await page.getByRole("button", { name: "Send message", exact: true }).click();
    const sent = page.getByRole("article", { name: "Your message" }); await expect(sent.locator("strong")).toHaveText("Review this"); await expect(sent.getByRole("link", { name: "Project repository" })).toBeVisible();
    await expect.poll(async () => (await page.evaluate(() => window.gator!.snapshot())).state!.running).toBe(false);
  } finally { await f.close(); }
});
