import { _electron as electron, test, expect } from "@playwright/test";
import { join, resolve } from "node:path";
import { tmpdir } from "node:os";
import { mkdir, rm, writeFile } from "node:fs/promises";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

test("dated chat search highlights literal matches and opens the intended topic by keyboard", async () => {
  const directory = await temporaryDirectory(join(tmpdir(), "gator-chat-search-e2e-"));
  const workspace = join(directory, "workspace");
  await mkdir(workspace);
  const dates = [new Date(), new Date(), new Date()];
  dates[1].setDate(dates[1].getDate() - 1); dates[2].setDate(dates[2].getDate() - 12);
  const titles = ["Plan [draft]", "Weekend plan", "Older notes with a longer conversation title that wraps naturally in a narrow window"];
  for (const [index, title] of titles.entries()) {
    const id = `work-search-${index}`, root = join(directory, "state/gator/conversations", id);
    await mkdir(root, { recursive: true });
    await writeFile(join(root, "conversation.json"), JSON.stringify({ version: 2, id, title, source_path: workspace, snapshot_id: "snap-search", created_at: dates[index].toISOString(), updated_at: dates[index].toISOString() }));
  }
  const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE;
  const app = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env: { ...process.env, GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace } });
  try {
    const page = await app.firstWindow();
    const recent = page.getByRole("region", { name: "Recent conversations", exact: true });
    await expect(recent.getByRole("heading", { name: "Today", exact: true })).toBeVisible();
    await expect(recent.getByRole("heading", { name: "Yesterday", exact: true })).toBeVisible();
    await expect(recent.getByRole("heading", { name: "Older", exact: true })).toBeVisible();
    const filter = recent.getByRole("searchbox", { name: "Filter recent conversations" });
    await filter.fill("[draft]");
    await expect(recent.locator("mark")).toHaveText("[draft]");
    await filter.press("ArrowDown");
    const draftTopic = recent.getByRole("button", { name: "Open conversation: Plan [draft]", exact: true });
    await expect(draftTopic).toBeFocused();
    await draftTopic.press("Enter");
    await expect.poll(() => page.evaluate(async () => (await window.gator!.snapshot()).state?.conversation_id)).toBe("work-search-0");
    await expect(draftTopic).toHaveAttribute("aria-current", "page");
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await composer.fill("Keep this topic’s unsent message");
    await recent.getByRole("button", { name: "All conversations", exact: true }).click();
    const browser = page.getByRole("region", { name: "Conversations", exact: true });
    await expect(browser.getByRole("heading", { name: "Yesterday", exact: true })).toBeVisible();
    await expect(browser.getByText("Titles and folders · up to 50 recent saved conversations", { exact: true })).toBeVisible();
    const snapshot = await page.evaluate(() => window.gator!.snapshot());
    await app.evaluate(({ BrowserWindow }, value) => BrowserWindow.getAllWindows()[0].webContents.send("gator:update", value), { ...snapshot, state: { ...snapshot.state!, recent_issue: "List unavailable" } });
    await expect(browser.getByText(/The conversation list couldn’t refresh/)).toBeVisible();
    await browser.getByRole("button", { name: "Try again", exact: true }).click();
    await expect(browser.getByText(/The conversation list couldn’t refresh/)).toHaveCount(0);
    for (const width of [1440, 800]) {
      await app.evaluate(({BrowserWindow},width)=>BrowserWindow.getAllWindows()[0].setSize(width,800),width);
      const positions=await browser.locator(".chat-search-row").evaluateAll(rows=>rows.map(row=>{
        const time=row.querySelector("time")!.getBoundingClientRect(),icon=row.querySelector(".chat-search-meta svg")!.getBoundingClientRect(),box=row.getBoundingClientRect(),copy=row.querySelector(".chat-search-copy")!.getBoundingClientRect();
        return {offset:Math.abs(time.y+time.height/2-icon.y-icon.height/2),right:icon.right,left:copy.right,meta:row.querySelector(".chat-search-meta")!.getBoundingClientRect().left,contained:icon.right<=box.right && icon.y>=box.y && icon.bottom<=box.bottom};
      }));
      for (const position of positions) {expect(position.offset).toBeLessThan(1);expect(position.contained).toBe(true);expect(position.left).toBeLessThan(position.meta);expect(position.right).toBeCloseTo(positions[0].right,0);}
      expect(await page.evaluate(()=>document.documentElement.scrollWidth<=innerWidth)).toBe(true);
      await page.screenshot({path:`test-results/conversation-chevron-alignment-${width}.png`});
    }
    const search = browser.getByRole("searchbox", { name: "Search conversations", exact: true });
    await search.fill("no such title");
    await expect(browser.getByRole("heading", { name: "No matching conversations" })).toBeVisible();
    await search.press("Escape");
    await expect(search).toHaveValue("");
    await search.fill("weekend");
    await expect(browser.locator("mark")).toHaveText("Weekend");
    await page.screenshot({ path: "test-results/desktop-chat-search.png" });
    await search.press("ArrowDown");
    const match = browser.locator(".panel-row");
    await expect(match).toBeFocused();
    await match.press("Enter");
    await expect.poll(() => page.evaluate(async () => (await window.gator!.snapshot()).state?.conversation_id)).toBe("work-search-1");
    await filter.fill("[draft]");
    await draftTopic.click();
    await expect(draftTopic).toHaveAttribute("aria-current", "page");
    await expect(composer).toHaveValue("Keep this topic’s unsent message");
    await page.keyboard.press(`${process.platform === "darwin" ? "Meta" : "Control"}+k`);
    await expect(page.getByRole("dialog", { name: "Search commands", exact: true })).toBeVisible();
    await page.keyboard.press("Escape");
  } finally { await closeDesktop(app); await rm(directory, { recursive: true, force: true }); }
});
