import { _electron as electron, test, expect, type ElectronApplication, type Page } from "@playwright/test";
import { createServer } from "node:http";
import { chmod, mkdir, readFile, writeFile, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join, resolve } from "node:path";
import { closeDesktop, temporaryDirectory, launchDesktop } from "./lifecycle";

async function fixture(waitForReply?: Promise<void>, waitForVoice?: () => Promise<void>) {
  const directory = await temporaryDirectory(join(tmpdir(), "gator-app-control-"));
  const workspace = join(directory, "workspace"), configRoot = join(directory, "config/gator");
  await mkdir(workspace); await mkdir(configRoot, { recursive: true });
  const payloads: { model: string; messages: { role: string; content: string; name?: string; tool_call_id?: string }[] }[] = [];
  let voiceRequests = 0;
  const server = createServer((req, res) => {
    let body = ""; req.on("data", chunk => { body += chunk; });
    req.on("end", async () => {
      res.writeHead(200, { "Content-Type": "application/json" });
      if (req.url?.includes(":generateContent")) {
        voiceRequests++;
        if (waitForVoice) await waitForVoice();
        const payload = JSON.parse(body);
        expect(payload.contents[0].parts[0].inlineData.data.length).toBeGreaterThan(0);
        res.end(JSON.stringify({ candidates: [{ content: { parts: [{ text: "What is my usage?" }] } }], usageMetadata: { totalTokenCount: 25 } })); return;
      }
      const payload = JSON.parse(body); payloads.push(payload);
      if (waitForReply) await waitForReply;
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
    async launch(overrides: Record<string, string> = {}) { const executablePath = process.env.GATOR_DESKTOP_TEST_EXECUTABLE; application = await launchDesktop({ executablePath, args: executablePath ? [] : ["."], cwd: resolve("."), env: { ...env, ...overrides } }); return application; },
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
  let releaseVoice = () => {};
  let voiceGate = new Promise<void>(resolve => { releaseVoice = resolve; });
  const f = await fixture(undefined, () => voiceGate);
  try {
    const app = await f.launch(), page = await app.firstWindow();
    await app.evaluate(({ systemPreferences }) => { systemPreferences.askForMediaAccess = async () => true; });
    await page.evaluate(() => {
      const view = window as unknown as { voiceSpoken: string[]; testTracks: MediaStreamTrack[]; testGain: GainNode; rejectMicrophone?: boolean; holdMicrophone?: boolean; releaseMicrophone?: () => void };
      view.voiceSpoken = []; view.testTracks = [];
      Object.defineProperty(navigator.mediaDevices, "getUserMedia", { configurable: true, value: async () => { if (view.rejectMicrophone) throw new Error("Microphone unavailable for this recording."); const audio = new AudioContext(); const oscillator = audio.createOscillator(), destination = audio.createMediaStreamDestination(), gain = audio.createGain(); oscillator.connect(gain); gain.connect(destination); oscillator.start(); view.testGain = gain; view.testTracks.push(...destination.stream.getTracks()); if (view.holdMicrophone) await new Promise<void>(resolve => { view.releaseMicrophone = resolve; }); return destination.stream; } });
      Object.defineProperty(window, "SpeechSynthesisUtterance", { configurable: true, value: class { text: string; constructor(text: string) { this.text = text; } } });
      Object.defineProperty(window, "speechSynthesis", { configurable: true, value: { getVoices: () => [{ name: "Local fixture", localService: true, lang: "en-US" }], cancel: () => {}, speak: (utterance: { text: string; onstart?: () => void }) => { view.voiceSpoken.push(utterance.text); utterance.onstart?.(); } } });
    });
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await composer.fill("Keep my existing draft.");
    await page.getByRole("button", { name: "Voice dictation", exact: true }).click();
    await expect(page.getByText("Audio is sent to Gemini", { exact: false })).toBeVisible();
    await page.screenshot({ path: "test-results/voice-controls-open.png" });
    await page.getByRole("button", { name: "Start recording", exact: true }).click();
    const finish = page.getByRole("button", { name: "Stop & transcribe", exact: true });
    await expect(finish).toBeFocused();
    await expect(page.getByRole("dialog", { name: "Voice dictation", exact: true })).toHaveCount(0);
    await expect(page.getByRole("button", { name: "Send message", exact: true })).toBeDisabled();
    const heights = () => page.locator(".dictation-waveform > span").evaluateAll(bars => bars.map(bar => Number((bar as HTMLElement).style.transform.match(/scaleY\((.+)\)/)?.[1] ?? 0)));
    await expect.poll(async () => Math.max(...await heights())).toBeGreaterThan(.5);
    await page.screenshot({ path: "test-results/voice-recording-inline.png" });
    await page.evaluate(() => { (window as unknown as { testGain: GainNode }).testGain.gain.value = 0; });
    await expect.poll(async () => Math.max(...await heights())).toBeLessThan(.2);
    await finish.click();
    await expect(page.getByText("Transcribing…", { exact: true })).toBeVisible();
    await expect(page.getByRole("button", { name: "Cancel transcription", exact: true })).toBeFocused();
    await expect(composer).toHaveValue("Keep my existing draft.");
    await page.screenshot({ path: "test-results/voice-transcribing-inline.png" });
    releaseVoice();
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
    await page.getByRole("button", { name: "Cancel recording", exact: true }).click();
    await expect(page.getByRole("button", { name: "Voice dictation", exact: true })).toBeFocused();
    expect(f.voiceRequests()).toBe(1);
    expect(await page.evaluate(() => (window as unknown as { testTracks: MediaStreamTrack[] }).testTracks.every(track => track.readyState === "ended"))).toBe(true);
    // Capture failures reopen the settings and leave the draft available to retry.
    await composer.fill("Keep this draft too.");
    await page.evaluate(() => { (window as unknown as { rejectMicrophone: boolean }).rejectMicrophone = true; });
    await page.getByRole("button", { name: "Voice dictation", exact: true }).click();
    await page.getByRole("button", { name: "Start recording", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText("Microphone unavailable for this recording.");
    await expect(page.getByRole("alert")).toBeFocused();
    await expect(composer).toHaveValue("Keep this draft too.");
    await expect(page.locator(".dictation-pill")).toHaveCount(0);
    await page.screenshot({ path: "test-results/voice-capture-error.png" });
    await page.evaluate(() => { (window as unknown as { rejectMicrophone: boolean }).rejectMicrophone = false; });
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 650));
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.getByRole("button", { name: "Start recording", exact: true }).click();
    await expect(finish).toBeVisible();
    await page.screenshot({ path: "test-results/voice-recording-narrow.png" });
    const pill = await page.locator(".dictation-pill").boundingBox();
    expect(pill!.x).toBeGreaterThanOrEqual(0); expect(pill!.x + pill!.width).toBeLessThanOrEqual(800);
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(true);
    await page.keyboard.press("Escape");
    await expect(page.getByRole("button", { name: "Voice dictation", exact: true })).toBeFocused();
    await expect(composer).toHaveValue("Keep this draft too.");
    expect(f.voiceRequests()).toBe(1);
    expect(await page.evaluate(() => (window as unknown as { testTracks: MediaStreamTrack[] }).testTracks.every(track => track.readyState === "ended"))).toBe(true);
    // Cancel while the microphone opens, then discard a late stream grant.
    await page.evaluate(() => { (window as unknown as { holdMicrophone: boolean }).holdMicrophone = true; });
    await page.getByRole("button", { name: "Voice dictation", exact: true }).click();
    await page.getByRole("button", { name: "Start recording", exact: true }).click();
    await expect(page.getByText("Opening mic…", { exact: true })).toBeVisible();
    await expect.poll(() => page.evaluate(() => typeof (window as unknown as { releaseMicrophone?: () => void }).releaseMicrophone)).toBe("function");
    await page.getByRole("button", { name: "Cancel recording", exact: true }).click();
    await page.evaluate(() => { const view = window as unknown as { releaseMicrophone: () => void; holdMicrophone: boolean }; view.holdMicrophone = false; view.releaseMicrophone(); });
    await expect.poll(() => page.evaluate(() => (window as unknown as { testTracks: MediaStreamTrack[] }).testTracks.every(track => track.readyState === "ended"))).toBe(true);
    // Cancel transcription without appending its eventual response or sending chat.
    voiceGate = new Promise<void>(resolve => { releaseVoice = resolve; });
    await page.getByRole("button", { name: "Voice dictation", exact: true }).click();
    await page.getByRole("button", { name: "Start recording", exact: true }).click();
    await expect(page.locator(".dictation-time")).toHaveText("0:01");
    await finish.click();
    await expect(page.getByText("Transcribing…", { exact: true })).toBeVisible();
    await expect.poll(() => f.voiceRequests()).toBe(2);
    await page.getByRole("button", { name: "Cancel transcription", exact: true }).click();
    releaseVoice();
    await expect(page.getByRole("button", { name: "Voice dictation", exact: true })).toBeFocused();
    await expect(composer).toHaveValue("Keep this draft too.");
    await expect(page.locator(".dictation-pill")).toHaveCount(0);
    await page.screenshot({ path: "test-results/voice-dictation-and-replies.png" });
  } finally { releaseVoice(); await f.close(); }
});


test("composer model picker preserves drafts, changes actual defaults and honors active-work locks", async () => {
  let release!: () => void;
  const gate = new Promise<void>(resolve => { release = resolve; });
  const f = await fixture(gate);
  const configFile = join(f.directory, "config/gator/config.json");
  const config = JSON.parse(await readFile(configFile, "utf8"));
  config.model_aliases = { "control-fixture:model-two": "Careful model" };
  config.custom_providers[0].models.push(...Array.from({ length: 8 }, (_, i) => `extra-model-${i}`));
  config.custom_providers.push({ id: "locked-fixture", base_url: "http://127.0.0.1:1/v1/chat/completions", api_key_env: "GATOR_PICKER_UNCONFIGURED_TEST_KEY", models: ["unavailable-model"], default_model: "unavailable-model" });
  await writeFile(configFile, JSON.stringify(config), { mode: 0o600 });
  try {
    const app = await f.launch(), page = await app.firstWindow();
    const errors: string[] = []; page.on("pageerror", error => errors.push(error.message));
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    const trigger = page.getByRole("button", { name: /^Model and work effort:/ });
    await composer.fill("Keep my unsent draft.");
    const before = (await page.evaluate(() => window.gator!.snapshot())).state!;
    await trigger.click();
    const dialog = page.getByRole("dialog", { name: "Model and work effort" });
    await expect(dialog).toBeVisible();
    const slider = dialog.getByRole("slider", { name: "Work effort" });
    await expect(slider).toHaveAttribute("aria-valuetext", "Standard");
    await slider.focus(); await page.keyboard.press("End");
    await expect.poll(async () => (await page.evaluate(() => window.gator!.snapshot())).state!.composer!.effort).toBe("high");
    await dialog.getByRole("button", { name: "Reset work effort to Standard" }).click();
    await expect.poll(async () => (await page.evaluate(() => window.gator!.snapshot())).state!.composer!.effort).toBe("standard");
    await expect.poll(() => page.locator(".effort-fill").evaluate(element => { const fill = element.getBoundingClientRect(), track = element.parentElement!.getBoundingClientRect(); return Math.abs(fill.right - (track.left + track.width / 2)); })).toBeLessThan(1.5);
    await page.screenshot({ path: "test-results/model-picker-effort.png" });
    await dialog.getByRole("button", { name: "model-one", exact: true }).click();
    await expect(dialog.getByRole("option", { name: /unavailable-model/ })).toHaveCount(0);
    await page.screenshot({ path: "test-results/model-picker-models.png" });
    const filter = dialog.getByRole("searchbox", { name: "Find a model" });
    await filter.fill("Careful");
    if (process.platform !== "win32") {
      const configDirectory = join(f.directory, "config/gator");
      await chmod(configDirectory, 0o500);
      try {
        await dialog.getByRole("option", { name: "Careful model" }).click();
        await expect(dialog.getByRole("alert")).toContainText("Could not select model:");
        await expect(filter).toBeFocused();
        await expect(page.locator(".connection-error")).toHaveCount(0);
        expect(JSON.parse(await readFile(configFile, "utf8")).defaults.model).toBe("model-one");
      } finally { await chmod(configDirectory, 0o700); }
    }
    await filter.press("ArrowDown"); await page.keyboard.press("Enter");
    await expect(trigger).toHaveAccessibleName(/Careful model, Standard/);
    expect(JSON.parse(await readFile(configFile, "utf8")).defaults.model).toBe("model-two");
    // The default can arrive before the acknowledgement switches popover views.
    await expect(slider).toBeEnabled();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0); await expect(trigger).toBeFocused();
    await expect(composer).toHaveValue("Keep my unsent draft.");
    const after = (await page.evaluate(() => window.gator!.snapshot())).state!;
    expect(after.source).toBe(before.source); expect(after.conversation_id).toBe(before.conversation_id); expect(after.messages).toEqual(before.messages);
    await trigger.click(); await composer.click({ position: { x: 12, y: 12 } }); await expect(dialog).toHaveCount(0);
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 650));
    await page.emulateMedia({ reducedMotion: "reduce" });
    await trigger.click();
    const box = await dialog.boundingBox(); expect(box!.x).toBeGreaterThanOrEqual(0); expect(box!.y).toBeGreaterThanOrEqual(0); expect(box!.x + box!.width).toBeLessThanOrEqual(800);
    await page.screenshot({ path: "test-results/model-picker-narrow.png" });
    await page.keyboard.press("Escape");
    await composer.fill("What is my usage?"); await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect.poll(() => f.payloads.length).toBeGreaterThan(0);
    expect(f.payloads[0].model).toBe("model-two");
    await trigger.click(); await expect(slider).toBeDisabled();
    await expect(dialog.getByText("Available after current and queued work finishes.")).toBeVisible();
    await page.keyboard.press("Escape");
    expect((await page.evaluate(() => window.gator!.snapshot())).state!.running).toBe(true);
    release(); await expect.poll(async () => (await page.evaluate(() => window.gator!.snapshot())).state!.running).toBe(false);
    expect(errors).toEqual([]);
  } finally { release(); await f.close(); }
});

test("composer hides unconfigured selections and opens Settings while preserving the message", async () => {
  const f = await fixture();
  const configFile = join(f.directory, "config/gator/config.json");
  const config = JSON.parse(await readFile(configFile, "utf8"));
  config.defaults = { provider: "opencode", model: "kimi-k2.7-code" };
  await writeFile(configFile, JSON.stringify(config), { mode: 0o600 });
  const file = join(f.directory, "note.txt");
  await writeFile(file, "Keep this attachment while configuring a model.");
  try {
    const app = await f.launch({ OPENCODE_API_KEY: "" }), page = await app.firstWindow();
    const trigger = page.getByRole("button", { name: /^Model and work effort:/ });
    const configure = page.getByRole("button", { name: "Configure models in Settings", exact: true });
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await expect(configure).toBeVisible();
    await expect(trigger).toHaveCount(0);
    await expect(page.locator(".composer-area")).not.toContainText("kimi-k2.7-code");
    await expect(page.locator(".composer-area")).not.toContainText("OpenCode");
    expect((await state(page)).composer?.models.some(choice => choice.provider === "opencode")).toBe(false);
    expect((await state(page)).composer?.model_issue).toContain("Configure models in Settings");
    await composer.fill("What is my usage?");
    await app.evaluate(({ dialog }, file) => { dialog.showOpenDialog = async () => ({ canceled: false, filePaths: [file] }); }, file);
    await page.getByRole("button", { name: "Attach files", exact: true }).click();
    const attachment = page.getByRole("button", { name: "Remove attachment note.txt", exact: true });
    await expect(attachment).toBeVisible();
    await page.screenshot({ path: "test-results/model-picker-configure-settings.png" });
    await configure.click();
    await page.getByRole("button", { name: /^Cloud API key/ }).click();
    await page.getByRole("searchbox", { name: "Filter models" }).fill("control-fixture");
    await page.getByRole("button", { name: /^control-fixture · model-one/ }).click();
    await page.getByRole("button", { name: "Back to app", exact: true }).click();
    await expect(trigger).toHaveAccessibleName(/model-one, Standard, control fixture$/);
    expect((await state(page)).composer?.model_issue).toBeUndefined();
    await expect(composer).toHaveValue("What is my usage?");
    await expect(attachment).toBeVisible();
    expect(f.payloads).toHaveLength(0);
    expect((await state(page)).messages).toHaveLength(0);
    await page.getByRole("button", { name: "Send message", exact: true }).click();
    await expect.poll(() => f.payloads.length).toBeGreaterThan(0);
    expect(f.payloads[0].model).toBe("model-one");
    await expect.poll(async () => (await state(page)).running).toBe(false);
    expect((await state(page)).messages.some(message => message.result?.title === "Work failed")).toBe(false);
  } finally { await f.close(); }
});

test("local voice setup offers explicit model download and converts browser audio without cloud fallback", async () => {
  test.setTimeout(90000);
  const f = await fixture();
  try {
    const app = await f.launch(), page = await app.firstWindow();
    // UI contract fixture only. Real model download/inference has a separate
    // opt-in native test; no physical microphone or commercial API is used here.
    await app.evaluate(({ ipcMain }) => {
      let model = "base", available = false, provider = "gemini", downloads = 0, recordings = 0;
      let release: ((value: unknown) => void) | undefined;
      const status = () => ({ ready: provider === "whisper-local" && available, provider, model, preferences: { provider, model, spoken_replies: false }, max_seconds: 120, max_audio_bytes: 8388608, local: { supported: true, runtime_ready: true, models: ["tiny", "base", "small"].map(id => ({ id, label: id, bytes: 100000000, available: id === model && available })) } });
      ipcMain.removeHandler("gator:voice");
      ipcMain.handle("gator:voice", async (_event, request) => {
        if (request.action === "configure") { provider = request.preferences.provider ?? provider; model = request.preferences.model || model; return status(); }
        if (request.action === "download") { downloads++; if (downloads === 1) return new Promise(resolve => { release = resolve; }); available = true; return status(); }
        if (request.action === "cancel") { release?.({ ...status(), issue: "Voice model download cancelled." }); return {}; }
        if (request.action === "authorize") return { ...status(), allowed: true };
        if (request.action === "transcribe") {
          recordings++;
          const bytes = new Uint8Array(request.data), view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
          if (request.media_type !== "audio/wav" || new TextDecoder().decode(bytes.slice(0, 4)) !== "RIFF" || view.getUint32(24, true) !== 16000 || view.getUint16(22, true) !== 1 || view.getUint16(34, true) !== 16 || bytes.length <= 44) throw new Error("Invalid local PCM conversion");
          if (recordings > 1) throw new Error("Local runtime failed. No recording was uploaded.");
          return { text: "Local dictation fixture." };
        }
        return status();
      });
    });
    await page.evaluate(() => {
      Object.defineProperty(navigator.mediaDevices, "getUserMedia", { configurable: true, value: async () => { const ctx = new AudioContext(), tone = ctx.createOscillator(), destination = ctx.createMediaStreamDestination(); tone.connect(destination); tone.start(); return destination.stream; } });
    });
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await composer.fill("Keep my draft.");
    await page.getByRole("button", { name: "Voice dictation", exact: true }).click();
    await page.getByLabel("Dictation provider", { exact: true }).selectOption("whisper-local");
    await expect(page.getByText("Audio is transcribed on this device", { exact: false })).toBeVisible();
    await expect(page.getByRole("button", { name: "Start recording", exact: true })).toBeDisabled();
    await page.getByLabel("Local dictation model").selectOption("tiny");
    await page.getByRole("button", { name: "Download local model", exact: true }).click();
    await expect(page.getByText("Downloading and verifying the model…", { exact: false })).toBeVisible();
    await page.getByRole("button", { name: "Cancel download", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText("cancelled");
    await page.getByRole("button", { name: "Download local model", exact: true }).click();
    await expect(page.getByRole("button", { name: "Start recording", exact: true })).toBeEnabled();
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 650));
    await page.emulateMedia({ reducedMotion: "reduce" });
    const panel = await page.getByRole("dialog", { name: "Voice dictation", exact: true }).boundingBox();
    expect(panel!.y).toBeGreaterThanOrEqual(0);
    expect(panel!.x + panel!.width).toBeLessThanOrEqual(800);
    await page.screenshot({ path: "test-results/local-voice-ready.png" });
    await page.getByRole("button", { name: "Start recording", exact: true }).click();
    await expect(page.locator(".dictation-time")).toHaveText("0:01");
    await page.getByRole("button", { name: "Stop & transcribe", exact: true }).click();
    await expect(composer).toHaveValue("Keep my draft. Local dictation fixture.");
    expect(f.voiceRequests()).toBe(0); expect(f.payloads).toHaveLength(0);
    await page.getByRole("button", { name: "Voice dictation", exact: true }).click();
    await page.getByRole("button", { name: "Start recording", exact: true }).click();
    await expect(page.locator(".dictation-time")).toHaveText("0:01");
    await page.getByRole("button", { name: "Stop & transcribe", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText("No recording was uploaded");
    await expect(composer).toHaveValue("Keep my draft. Local dictation fixture.");
    expect(f.voiceRequests()).toBe(0); expect(f.payloads).toHaveLength(0);
  } finally { await f.close(); }
});

test("composer sources use real per-chat preferences without reads or model calls and reject removed accounts", async () => {
  const f = await fixture();
  try {
    const app = await f.launch(), page = await app.firstWindow();
    await expect.poll(async () => !!(await state(page)).sources).toBe(true);
    const original = (await state(page)).conversation_id!;
    const composer = page.getByRole("textbox", { name: "Message Gator", exact: true });
    await composer.fill("Keep my source draft");
    const trigger = page.getByRole("button", { name: "Add sources", exact: true });
    await trigger.click();
    const reports = page.getByRole("checkbox", { name: "Reports source", exact: true });
    await expect(reports).toContainText("access not checked");
    await expect(reports).toBeEnabled();
    await reports.click();
    await expect(reports).toHaveAttribute("aria-checked", "true");
    await expect.poll(async () => (await state(page)).work_details?.connector_ids).toEqual(["reports"]);
    await page.keyboard.press("Escape"); await expect(trigger).toBeFocused();
    await expect(composer).toHaveValue("Keep my source draft");
    expect(f.payloads).toHaveLength(0); expect(f.voiceRequests()).toBe(0);
    await page.getByRole("button", { name: "New chat", exact: true }).click();
    await expect.poll(async () => (await state(page)).conversation_id).not.toBe(original);
    await expect.poll(async () => (await state(page)).work_details?.connector_ids).toEqual([]);
    await page.evaluate(async id => {
      const snapshot = (await window.gator!.snapshot()).state!;
      await window.gator!.send({ type: "conversation", conversation: { id, list_id: snapshot.recent_id! } });
    }, original);
    await expect.poll(async () => (await state(page)).conversation_id).toBe(original);
    await expect.poll(async () => (await state(page)).work_details?.connector_ids).toEqual(["reports"]);
    const settingsPath = join(f.directory, "config/gator/config.json");
    const settings = JSON.parse(await readFile(settingsPath, "utf8")); settings.connectors = [];
    await writeFile(settingsPath, JSON.stringify(settings), { mode: 0o600 });
    // An old displayed source ID cannot reselect a removed descriptor.
    const before = (await state(page)).sources!;
    await page.evaluate(async id => window.gator!.send({ type: "sources", sources: { id, request_id: "removed-source", action: "select", ids: ["reports"] } }), before.id);
    await expect.poll(async () => (await state(page)).sources_result?.error).toContain("Sources changed");
    await trigger.click();
    await expect(page.getByText("No sources configured. Connect an account in Settings.", { exact: true })).toBeVisible();
    await page.getByRole("button", { name: "Clear sources", exact: true }).click();
    await expect.poll(async () => (await state(page)).work_details?.connector_ids).toEqual([]);
    expect(f.payloads).toHaveLength(0);
    await app.evaluate(({ BrowserWindow }) => BrowserWindow.getAllWindows()[0].setSize(800, 650));
    await page.emulateMedia({ reducedMotion: "reduce" });
    await page.screenshot({ path: "test-results/composer-sources.png" });
  } finally { await f.close(); }
});

test("changing composer sources cannot rewrite the durable options of running and queued requests", async () => {
 test.setTimeout(60000);
 let release=()=>{};const gate=new Promise<void>(resolve=>{release=resolve;});
 const f=await fixture(gate);
 try {
  const app=await f.launch(),page=await app.firstWindow();
  const composer=page.getByRole("textbox",{name:"Message Gator",exact:true});
  await composer.fill("What is my usage?");await composer.press("Enter");
  await expect.poll(()=>f.payloads.length).toBeGreaterThan(0);
  const id=(await state(page)).conversation_id!;
  await page.getByRole("button",{name:"Add sources",exact:true}).click();
  await page.getByRole("checkbox",{name:"Reports source",exact:true}).click();
  await expect.poll(async()=>(await state(page)).work_details?.connector_ids).toEqual(["reports"]);
  await page.keyboard.press("Escape");
  await composer.fill("Explain my usage limits next.");await composer.press("Enter");
  await expect.poll(async()=>(await state(page)).queued).toBe(1);
  const journalPath=join(f.directory,"state/gator/conversations",id,"requests.json");
  const journal=()=>readFile(journalPath,"utf8").then(text=>JSON.parse(text).requests as {text:string;options:{ConnectorIDs?:string[]}}[]);
  await expect.poll(async()=>(await journal()).find(item=>item.text==="Explain my usage limits next.")?.options.ConnectorIDs).toEqual(["reports"]);
  await page.getByRole("button",{name:"Add sources",exact:true}).click();
  await page.getByRole("button",{name:"Clear sources",exact:true}).click();
  await expect.poll(async()=>(await state(page)).work_details?.connector_ids).toEqual([]);
  const retained=await journal();
  expect(retained.find(item=>item.text==="Explain my usage limits next.")?.options.ConnectorIDs).toEqual(["reports"]);
  expect(retained.find(item=>item.text==="What is my usage?")?.options.ConnectorIDs??[]).toEqual([]);
  release();await expect.poll(async()=>(await state(page)).running,{timeout:30000}).toBe(false);
 } finally {release();await f.close();}
});
