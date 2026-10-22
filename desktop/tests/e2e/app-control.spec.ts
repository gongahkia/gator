import { _electron as electron, test, expect, type ElectronApplication, type Page } from "@playwright/test";
import { createServer } from "node:http";
import { mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, temporaryDirectory } from "./lifecycle";

async function fixture() {
  const directory = await temporaryDirectory(join(tmpdir(), "gator-app-control-"));
  const workspace = join(directory, "workspace"), configRoot = join(directory, "config/gator");
  await mkdir(workspace); await mkdir(configRoot, { recursive: true });
  const payloads: { model: string; messages: { role: string; content: string; name?: string; tool_call_id?: string }[] }[] = [];
  let voiceRequests = 0;
  const server = createServer((req, res) => {
    let body = ""; req.on("data", chunk => { body += chunk; });
    req.on("end", () => {
      res.writeHead(200, { "Content-Type": "application/json" });
      if (req.url?.includes(":generateContent")) {
        voiceRequests++;
        const payload = JSON.parse(body);
        expect(payload.contents[0].parts[0].inlineData.data.length).toBeGreaterThan(0);
        res.end(JSON.stringify({ candidates: [{ content: { parts: [{ text: "What is my usage?" }] } }], usageMetadata: { totalTokenCount: 25 } })); return;
      }
      const payload = JSON.parse(body); payloads.push(payload);
      if (payload.messages.some((message: { role: string; content?: string }) => message.role === "system" && message.content?.includes("Classify the user's immediate request"))) {
        res.end(JSON.stringify({ choices: [{ message: { role: "assistant", content: '{"mode":"inspect"}' }, finish_reason: "stop" }] })); return;
      }
      const user = [...payload.messages].reverse().find(message => message.role === "user")?.content ?? "";
      const last = payload.messages.at(-1);
      let setting = "", value: unknown, topic = "usage";
      if (user.includes("rebind")) { topic = "shortcuts"; setting = "shortcuts"; value = { action: "command_search", shortcut: "Mod+P" }; }
      else if (user.includes("model-two")) { topic = "models"; setting = "model"; value = { provider: "control-fixture", model: "model-two" }; }
      else if (user.includes("daily requests")) { setting = "usage_limits"; value = { daily_requests: user.includes("2000") ? 2000 : 1000 }; }
      else if (user.includes("history for 30")) { topic = "data"; setting = "retention"; value = { history_days: 30 }; }
      else if (user.includes("Reports")) { topic = "connections"; setting = "connections"; value = { ids: ["reports"] }; }
      else if (user.includes("delete saved knowledge")) { topic = "data"; setting = "data_action"; value = { action: "delete", scope: "knowledge" }; }
      let message: unknown;
      if (last?.role !== "tool") {
        message = { role: "assistant", tool_calls: [{ id: `inspect-app-${payloads.length}`, type: "function", function: { name: "gator_app_inspect", arguments: JSON.stringify({ topic }) } }] };
      } else if (last.tool_call_id?.startsWith("inspect-app-") && setting) {
        message = { role: "assistant", tool_calls: [{ id: `update-app-${payloads.length}`, type: "function", function: { name: "gator_app_update", arguments: JSON.stringify({ setting, value }) } }] };
      } else {
        const result = JSON.parse(last.content);
        message = { role: "assistant", content: result.status === "declined" ? "The setting change was declined." : setting ? "The requested app change has been handled." : "Your current usage is available in Usage & limits." };
      }
      res.end(JSON.stringify({ id: "fixture-response", choices: [{ message, finish_reason: (message as { tool_calls?: unknown }).tool_calls ? "tool_calls" : "stop" }], usage: { prompt_tokens: 10, completion_tokens: 5, total_tokens: 15 } }));
    });
  });
  await new Promise<void>(done => server.listen(0, "127.0.0.1", done));
  const port = (server.address() as { port: number }).port;
  await writeFile(join(configRoot, "config.json"), JSON.stringify({ version: 4, defaults: { provider: "control-fixture", model: "model-one" }, custom_providers: [{ id: "control-fixture", base_url: `http://127.0.0.1:${port}/v1/chat/completions`, models: ["model-one", "model-two"], default_model: "model-one" }], provider_endpoints: { gemini: `http://127.0.0.1:${port}/v1beta` }, desktop: { voice: { provider: "gemini", model: "voice-fixture", language: "en" } }, connectors: [{ version: 1, id: "reports", name: "Reports", kind: "http_json", resource: "https://example.com/reports.json", authentication: "none" }] }), { mode: 0o600 });
  const env = Object.fromEntries(Object.entries({ ...process.env, GATOR_INTERNAL_COMMANDS: "", GATOR_STATE_DIR: join(directory, "state"), GATOR_CONFIG_DIR: join(directory, "config"), GATOR_DESKTOP_USER_DATA: join(directory, "desktop"), GATOR_DESKTOP_WORKSPACE: workspace, GEMINI_API_KEY: "local-voice-fixture-key" }).filter((entry): entry is [string, string] => typeof entry[1] === "string"));
  let application: ElectronApplication | undefined;
  return {
    directory, payloads, voiceRequests: () => voiceRequests,
    async launch() { const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE; application = await electron.launch({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env }); return application; },
    async close() { try { if (application) await closeDesktop(application); } finally { server.closeAllConnections(); await new Promise<void>(done => server.close(() => done())); await rm(directory, { recursive: true, force: true }); } },
  };
}

const state = (page: Page) => page.evaluate(async () => (await window.gator!.snapshot()).state!);
async function ask(page: Page, question: string, review?: boolean) {
  await expect.poll(async () => Boolean(await state(page))).toBe(true);
  const before = (await state(page)).messages.length;
  const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
  await composer.fill(question); await composer.press("Enter");
  await expect.poll(async () => (await state(page)).messages.length).toBeGreaterThan(before);
  if (review !== undefined) await page.getByRole("button", { name: review ? "Approve this action" : "Deny", exact: true }).click();
  await expect.poll(async () => (await state(page)).running, { timeout: 30000 }).toBe(false);
  expect((await state(page)).messages.filter(message => message.result?.title === "Work failed").map(message => message.result?.summary)).toEqual([]);
}

test("chat changes shortcuts, model selection, limits, retention and selected connections", async () => {
  test.setTimeout(120000);
  const f = await fixture();
  try {
    const app = await f.launch(), page = await app.firstWindow();
    await ask(page, "Can you rebind command search to Mod+P?");
    await expect.poll(async () => (await state(page)).desktop?.shortcuts.command_search).toBe("Mod+P");
    const mod = process.platform === "darwin" ? "Meta" : "Control";
    await page.keyboard.press(`${mod}+k`); await expect(page.getByRole("dialog", { name: "Search commands", exact: true })).toHaveCount(0);
    await page.keyboard.press(`${mod}+p`); await expect(page.getByRole("textbox", { name: "Find a command", exact: true })).toBeFocused(); await page.keyboard.press("Escape");
    await ask(page, "Can you switch to model-two?");
    await expect.poll(async () => (await state(page)).model).toBe("model-two");
    await ask(page, "What is my usage?");
    expect(f.payloads.at(-1)?.model).toBe("model-two");
    await ask(page, "Can you limit me to 1000 daily requests?", true);
    const policy = join(f.directory, "state/gator/usage/policy.json");
    await expect.poll(async () => JSON.parse(await readFile(policy, "utf8")).daily_requests).toBe(1000);
    await ask(page, "Can you limit me to 2000 daily requests?", false);
    expect(JSON.parse(await readFile(policy, "utf8")).daily_requests).toBe(1000);
    await ask(page, "Can you keep completed history for 30 days?", true);
    expect(JSON.parse(await readFile(join(f.directory, "state/gator/data-control/retention.json"), "utf8")).HistoryDays).toBe(30);
    await ask(page, "Can you select Reports for my next messages?");
    await expect.poll(async () => (await state(page)).work_details?.connector_ids).toEqual(["reports"]);
    await page.screenshot({ path: "test-results/chat-app-controls.png" });
    await closeDesktop(app);
    const reopened = await f.launch(), restored = await reopened.firstWindow();
    await expect.poll(async () => (await state(restored)).desktop?.shortcuts.command_search).toBe("Mod+P");
    await expect.poll(async () => (await state(restored)).work_details?.connector_ids).toEqual(["reports"]);
  } finally { await f.close(); }
});

test("chat opens a data deletion review without deleting content", async () => {
  const f = await fixture();
  try {
    const directory = join(f.directory, "state/gator/knowledge"); await mkdir(directory, { recursive: true });
    const kept = join(directory, "kept.md"); await writeFile(kept, "Keep until final review.");
    const app = await f.launch(), page = await app.firstWindow();
    await ask(page, "Can you help me delete saved knowledge?");
    await expect(page.getByRole("heading", { name: "Review private data removal", exact: true })).toBeVisible();
    expect(await readFile(kept, "utf8")).toBe("Keep until final review.");
  } finally { await f.close(); }
});

test("dictation preserves the draft, records voice usage and reads only new replies", async () => {
  test.setTimeout(90000);
  const f = await fixture();
  try {
    const app = await f.launch(), page = await app.firstWindow();
    await app.evaluate(({ systemPreferences }) => { systemPreferences.askForMediaAccess = async () => true; });
    await page.evaluate(() => {
      const view = window as unknown as { voiceSpoken: string[]; testTracks: MediaStreamTrack[] };
      view.voiceSpoken = []; view.testTracks = [];
      Object.defineProperty(navigator.mediaDevices, "getUserMedia", { configurable: true, value: async () => { const audio = new AudioContext(); const oscillator = audio.createOscillator(), destination = audio.createMediaStreamDestination(); oscillator.connect(destination); oscillator.start(); view.testTracks.push(...destination.stream.getTracks()); return destination.stream; } });
      Object.defineProperty(window, "SpeechSynthesisUtterance", { configurable: true, value: class { text: string; constructor(text: string) { this.text = text; } } });
      Object.defineProperty(window, "speechSynthesis", { configurable: true, value: { getVoices: () => [{ name: "Local fixture", localService: true, lang: "en-US" }], cancel: () => {}, speak: (utterance: { text: string; onstart?: () => void }) => { view.voiceSpoken.push(utterance.text); utterance.onstart?.(); } } });
    });
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await composer.fill("Keep my existing draft.");
    await page.getByRole("button", { name: "Voice dictation", exact: true }).click();
    await expect(page.getByText("Audio is sent to Gemini", { exact: false })).toBeVisible();
    await page.screenshot({ path: "test-results/voice-controls-open.png" });
    await page.getByRole("button", { name: "Start recording", exact: true }).click();
    await expect(page.getByRole("button", { name: "Stop & transcribe", exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Send message", exact: true })).toBeDisabled();
    await page.waitForTimeout(350);
    await page.getByRole("button", { name: "Stop & transcribe", exact: true }).click();
    await expect(composer).toHaveValue("Keep my existing draft. What is my usage?", { timeout: 20000 });
    expect(f.voiceRequests()).toBe(1); expect(f.payloads).toHaveLength(0);
    expect(await page.evaluate(() => (window as unknown as { testTracks: MediaStreamTrack[] }).testTracks.every(track => track.readyState === "ended"))).toBe(true);
    await page.getByRole("button", { name: "Enable spoken replies", exact: true }).click();
    await expect(page.getByRole("button", { name: "Disable spoken replies", exact: true })).toBeVisible();
    await ask(page, "What is my usage?");
    await expect.poll(() => page.evaluate(() => (window as unknown as { voiceSpoken: string[] }).voiceSpoken.length)).toBe(1);
    await page.getByRole("button", { name: "Stop speaking", exact: true }).click();
    await page.getByRole("button", { name: "Voice dictation", exact: true }).click();
    await page.getByRole("button", { name: "Start recording", exact: true }).click();
    await expect(page.getByRole("button", { name: "Stop & transcribe", exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Close voice controls", exact: true }).click();
    expect(f.voiceRequests()).toBe(1);
    expect(await page.evaluate(() => (window as unknown as { testTracks: MediaStreamTrack[] }).testTracks.every(track => track.readyState === "ended"))).toBe(true);
    await page.screenshot({ path: "test-results/voice-dictation-and-replies.png" });
  } finally { await f.close(); }
});
