import { spawn } from "node:child_process";

export const MAX_AUDIO_BYTES = 8 * 1024 * 1024;
export function validateVoiceRequest(request) {
  if (!request || typeof request !== "object" || Array.isArray(request)) throw new Error("Invalid voice request");
  const allowed = { status: ["action"], authorize: ["action", "context"], release: ["action"], cancel: ["action"], configure: ["action", "preferences"], transcribe: ["action", "context", "media_type", "data"] }[request.action];
  if (!allowed || Object.keys(request).some(key => !allowed.includes(key))) throw new Error("Unsupported voice request");
  if (["authorize", "transcribe"].includes(request.action) && (typeof request.context !== "string" || !request.context || request.context.length > 8192)) throw new Error("Invalid dictation conversation");
  if (request.action === "configure" && (!request.preferences || typeof request.preferences !== "object" || Array.isArray(request.preferences) || Object.keys(request.preferences).some(key => !["spoken_replies", "provider", "model", "language", "rate"].includes(key)) || JSON.stringify(request.preferences).length > 2048)) throw new Error("Invalid voice preferences");
  if (request.action === "transcribe" && (!(request.data instanceof Uint8Array) || !request.data.byteLength || request.data.byteLength > MAX_AUDIO_BYTES || typeof request.media_type !== "string" || !/^audio\/(webm|ogg|mp4|wav)(?:;codecs=[a-z0-9.-]+)?$/i.test(request.media_type))) throw new Error("Invalid audio recording (maximum 8 MiB)");
  return request;
}

export function microphoneAllowed(owner, contents, permission, details, grant, now = Date.now()) {
  const kinds = details.mediaTypes ?? (details.mediaType ? [details.mediaType] : []);
  return !!owner && owner === contents && permission === "media" && details.isMainFrame !== false && !!grant && grant.expires > now && owner.getURL() === "gator://app/index.html" && kinds.length > 0 && kinds.every(kind => kind === "audio");
}

export function runSpeechRequest(binary, cwd, env, request, signal) {
  return new Promise((resolve, reject) => {
    const child = spawn(binary, ["desktop-speech"], { cwd, env: { ...env, GATOR_DESKTOP_SPEECH: "1" }, stdio: ["pipe", "pipe", "pipe"], windowsHide: true });
    let output = "", diagnostic = "", settled = false;
    const finish = (error, value) => { if (settled) return; settled = true; clearTimeout(timer); signal?.removeEventListener("abort", abort); error ? reject(error) : resolve(value); };
    const abort = () => { child.kill("SIGTERM"); const force = setTimeout(() => child.kill("SIGKILL"), 1500); force.unref(); child.once("close", () => clearTimeout(force)); finish(new Error("Dictation cancelled. Your draft is kept.")); };
    const timer = setTimeout(() => { child.kill("SIGKILL"); finish(new Error("Dictation timed out. Try a shorter recording.")); }, 70000);
    signal?.addEventListener("abort", abort, { once: true });
    child.stdout.setEncoding("utf8"); child.stderr.setEncoding("utf8");
    child.stdout.on("data", chunk => { output += chunk; if (Buffer.byteLength(output) > 128 * 1024) { child.kill("SIGKILL"); finish(new Error("Dictation response is too large.")); } });
    child.stderr.on("data", chunk => { diagnostic = (diagnostic + chunk).slice(-2048); });
    child.on("error", error => finish(error)); child.stdin.on("error", () => {});
    child.on("close", code => {
      if (code !== 0) return finish(new Error(diagnostic.trim().replace(/^Gator:\s*/, "") || "Voice transcription could not finish."));
      try { finish(undefined, JSON.parse(output)); } catch { finish(new Error("Invalid voice response.")); }
    });
    if (signal?.aborted) { abort(); return; }
    child.stdin.end(JSON.stringify(request));
  });
}
