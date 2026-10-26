import { test } from "node:test";
import assert from "node:assert/strict";
import { EventEmitter } from "node:events";
import { helper, openLink, speechPlayer } from "../src/native.mjs";
import { VoiceSession } from "../src/voice-session.mjs";
import { renderPDF } from "../src/pdf.mjs";
import { pdfFixture } from "./pdf-fixture.mjs";

test("native helpers preserve literal argv, bound output and support cancellation", async () => {
  const literal = 'x; $(echo never) `echo never` " 🐊';
  assert.equal((await helper(process.execPath, ["-e", "process.stdout.write(process.argv[1])", literal])).toString(), literal);
  await assert.rejects(helper(process.execPath, ["-e", "process.stdout.write('x'.repeat(10000))"], { maxBytes: 100 }), /output limit/);
  const abort = new AbortController();
  const pending = helper(process.execPath, ["-e", "setTimeout(()=>{},10000)"], { signal: abort.signal });
  abort.abort(); await assert.rejects(pending, /abort/i);
  await assert.rejects(openLink("file:///private/credentials"), /Only http/);
});

test("native playback carries language and rate without exposing reply text in arguments", async () => {
  const text = 'Literal reply: $(never) `never` " 中文 🐊';
  for (const platform of ["darwin", "win32", "linux"]) {
    const calls = [], signal = new AbortController().signal;
    const speak = speechPlayer({ platform, powershell: () => "powershell-fixture", run: async (program, args, options) => {
      calls.push({ program, args, options });
      return Buffer.from(JSON.stringify([{ name: "English", language: "en_US" }, { name: "French Canada", language: "fr_CA" }, { name: "French France", language: "fr-FR" }]));
    } });
    await speak(text, { language: "fr-FR", rate: 1.5 }, signal);
    const playback = calls.at(-1); assert.equal(playback.options.signal, signal);
    assert.ok(calls.every(call => !call.args.join(" ").includes(text)));
    if (platform === "darwin") { assert.deepEqual(playback.args, ["-r", "263", "-v", "French France", "-f", "-"]); assert.equal(playback.options.input, text); }
    if (platform === "win32") { assert.deepEqual(JSON.parse(playback.options.input), { text, voice: "French France", rate: 6 }); assert.ok(playback.args.at(-1).includes("$s.Rate=[int]$p.rate")); }
    if (platform === "linux") { assert.deepEqual(playback.args, ["--stdin", "-s", "263", "-v", "fr-FR"]); assert.equal(playback.options.input, text); }
    calls.length = 0; await speak(text, { language: "fr-BE", rate: 0.5 });
    if (platform !== "linux") assert.ok(JSON.stringify(calls.at(-1)).includes("French Canada"));
    if (platform === "win32") assert.equal(JSON.parse(calls.at(-1).options.input).rate, -10);
    calls.length = 0; await speak(text, { rate: 2 }); assert.equal(calls.length, 1);
    if (platform === "win32") assert.equal(JSON.parse(calls[0].options.input).rate, 10);
    calls.length = 0;
    if (platform !== "linux") { await assert.rejects(speak(text, { language: "ja" }), /No installed speech voice/); assert.equal(calls.length, 1); }
    calls.length = 0;
    await assert.rejects(speak(text, { language: "en; malicious" }), /Invalid/);
    await assert.rejects(speak(text, { rate: 3 }), /Invalid/);
    const aborted = new AbortController(); aborted.abort(); await assert.rejects(speak(text, {}, aborted.signal), /abort/i);
    assert.equal(calls.length, 0);
  }
});

test("cancelling voice enumeration prevents native playback", async () => {
  const abort = new AbortController(); let calls = 0;
  const speak = speechPlayer({ platform: "darwin", run: async () => { calls++; abort.abort(); return Buffer.from('[{"name":"English","language":"en"}]'); } });
  await assert.rejects(speak("Do not play", { language: "en" }, abort.signal), /abort/i);
  assert.equal(calls, 1);
});

class App extends EventEmitter {
  context = "conversation:a";
  state = { messages: [], desktop: { voice: { spoken_replies: true } } };
  mutable() {}
  snapshot() { return { state: this.state }; }
}
test("dictation captures the provider route and cannot deliver into another conversation", async () => {
  const application = new App(); let resolveAudio, cancelled = false, transcription = 0;
  const status = { ready: true, route: "captured-provider-route" };
  const session = new VoiceSession(application, {
    capture: () => ({ audio: new Promise(resolve => { resolveAudio = resolve; }), stop() { resolveAudio(Buffer.from("test-audio")); }, cancel() { cancelled = true; resolveAudio(Buffer.from("cancelled")); } }),
    call: async (_binary, _cwd, _env, request) => { if (request.action === "status") return status; assert.equal(request.route, status.route); transcription++; return { text: "Review this transcript" }; }, playback: async () => {},
  });
  try {
    await session.start(); assert.equal(session.phase, "recording"); assert.equal(transcription, 0);
    assert.deepEqual(await session.stop(), { context: application.context, text: "Review this transcript" });
    assert.equal(transcription, 1);
    await session.start(); application.context = "conversation:b"; application.emit("change");
    assert.equal(cancelled, true); assert.equal(session.phase, "idle");
    await assert.rejects(session.stop(), /Start dictation/); assert.equal(transcription, 1);
  } finally { session.close(); assert.equal(application.listenerCount("change"), 0); }
});
test("spoken output ignores retained history, speaks only new replies and aborts on close", async () => {
  const application = new App(); const spoken = []; let aborted = false;
  const session = new VoiceSession(application, { playback: async (text, _preferences, signal) => { spoken.push(text); signal.addEventListener("abort", () => { aborted = true; }); } });
  application.state.messages = [{ role: "Gator", text: "Historical reply" }]; application.emit("change");
  assert.deepEqual(spoken, []);
  application.state.messages.push({ role: "You", text: "A new request" }, { role: "Gator", text: "New reply" }); application.emit("change");
  assert.deepEqual(spoken, ["New reply"]);
  session.close(); assert.equal(aborted, true);
});

test("spoken output stops on chat switch, preference disable and explicit playback cancellation", async () => {
  const application = new App(), signals = [];
  const session = new VoiceSession(application, { playback: async (_text, _preferences, signal) => { signals.push(signal); await new Promise(resolve => signal.addEventListener("abort", resolve, { once: true })); } });
  try {
    application.emit("change"); application.state.messages.push({ role: "Gator", text: "New spoken reply" }); application.emit("change");
    assert.equal(session.playing, true);
    application.context = "conversation:b"; application.state.messages = [{ role: "Gator", text: "Other retained history" }]; application.emit("change");
    assert.equal(signals[0].aborted, true); assert.equal(session.playing, false); assert.equal(signals.length, 1);
    application.state.messages.push({ role: "Gator", text: "Other new reply" }); application.emit("change");
    application.state.desktop.voice.spoken_replies = false; application.emit("change");
    assert.equal(signals[1].aborted, true); assert.equal(session.playing, false);
    application.state.desktop.voice.spoken_replies = true; application.state.messages.push({ role: "Gator", text: "Explicitly cancellable reply" }); application.emit("change");
    session.stopPlayback(); assert.equal(signals[2].aborted, true); assert.equal(session.playing, false);
  } finally { session.close(); }
});

test("PDF preview renders actual distinct page images and rejects nonexistent pages", async () => {
  const bytes = pdfFixture();
  const first = await renderPDF(bytes, 1, 400), second = await renderPDF(bytes, 2, 400);
  assert.equal(first.pages, 2); assert.equal(second.page, 2);
  assert.equal(Buffer.from(first.image.subarray(1, 4)).toString(), "PNG");
  assert.notDeepEqual(first.image, second.image);
  await assert.rejects(renderPDF(bytes, 3), /does not exist/);
});
