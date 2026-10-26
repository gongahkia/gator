import { spawn } from "node:child_process";
import { externalURL } from "./policy.mjs";
import { powershellPath } from "./platform.mjs";
import { MAX_AUDIO_BYTES } from "./voice.mjs";

export function helper(program, args, { input, maxBytes = 16 * 1024 * 1024, signal } = {}) {
  return new Promise((resolve, reject) => {
    const child = spawn(program, args, { windowsHide: true, stdio: ["pipe", "pipe", "pipe"], signal });
    const chunks = []; let size = 0, errorText = "", failure;
    child.stdout.on("data", chunk => { size += chunk.length; if (size > maxBytes) { failure = new Error("The native helper exceeded its output limit."); child.kill("SIGKILL"); } else chunks.push(chunk); });
    child.stderr.on("data", chunk => { errorText = (errorText + chunk).slice(-4096); });
    child.on("error", reject); child.stdin.on("error", () => {});
    child.on("close", code => failure || code !== 0 ? reject(failure || new Error(errorText.trim() || `${program} exited with ${code}. Install/configure the native helper and try again.`)) : resolve(Buffer.concat(chunks)));
    child.stdin.end(input);
  });
}
export async function openLink(value) {
  const url = externalURL(value);
  if (!url) throw new Error("Only http and https links can be opened.");
  if (process.platform === "darwin") await helper("/usr/bin/open", [url]);
  else if (process.platform === "win32") await helper(powershellPath(), ["-NoProfile", "-NonInteractive", "-Command", "$u=[Console]::In.ReadToEnd(); Start-Process -FilePath $u"], { input: url });
  else await helper("xdg-open", [url]);
}
export async function clipboardRead() {
  if (process.platform === "darwin") return (await helper("/usr/bin/pbpaste", [])).toString("utf8");
  if (process.platform === "win32") return (await helper(powershellPath(), ["-NoProfile", "-NonInteractive", "-Command", "[Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); [Console]::Out.Write((Get-Clipboard -Raw))"])).toString("utf8");
  return (await helper(process.env.WAYLAND_DISPLAY ? "wl-paste" : "xclip", process.env.WAYLAND_DISPLAY ? ["--no-newline", "--type", "text/plain"] : ["-selection", "clipboard", "-out"])).toString("utf8");
}
export async function clipboardWrite(text) {
  if (process.platform === "darwin") await helper("/usr/bin/pbcopy", [], { input: text });
  else if (process.platform === "win32") await helper(powershellPath(), ["-NoProfile", "-NonInteractive", "-Command", "[Console]::InputEncoding=[Text.UTF8Encoding]::new($false); Set-Clipboard -Value ([Console]::In.ReadToEnd())"], { input: text });
  else await helper(process.env.WAYLAND_DISPLAY ? "wl-copy" : "xclip", process.env.WAYLAND_DISPLAY ? ["--type", "text/plain"] : ["-selection", "clipboard", "-in"], { input: text });
}
export async function clipboardImage() {
  if (process.platform === "darwin") {
    const script = "ObjC.import('AppKit');var p=$.NSPasteboard.generalPasteboard;var d=p.dataForType('public.png');if(!d){var t=p.dataForType('public.tiff');if(t){d=$.NSBitmapImageRep.imageRepWithData(t).representationUsingTypeProperties($.NSBitmapImageFileTypePNG,$({}));}}if(!d)throw Error('No image in clipboard');ObjC.unwrap(d.base64EncodedStringWithOptions(0));";
    return Buffer.from((await helper("/usr/bin/osascript", ["-l", "JavaScript", "-e", script], { maxBytes: MAX_AUDIO_BYTES })).toString().trim(), "base64");
  }
  if (process.platform === "win32") {
    const script = "Add-Type -AssemblyName System.Windows.Forms; Add-Type -AssemblyName System.Drawing; $i=[Windows.Forms.Clipboard]::GetImage(); if($null -eq $i){throw 'No image in clipboard'}; $m=[IO.MemoryStream]::new(); try{$i.Save($m,[Drawing.Imaging.ImageFormat]::Png); [Console]::Out.Write([Convert]::ToBase64String($m.ToArray()))}finally{$m.Dispose();$i.Dispose()}";
    return Buffer.from((await helper(powershellPath(), ["-NoProfile", "-NonInteractive", "-STA", "-Command", script], { maxBytes: MAX_AUDIO_BYTES })).toString(), "base64");
  }
  return helper(process.env.WAYLAND_DISPLAY ? "wl-paste" : "xclip", process.env.WAYLAND_DISPLAY ? ["--type", "image/png"] : ["-selection", "clipboard", "-out", "-target", "image/png"], { maxBytes: 4 * 1024 * 1024 });
}
export function recordingArguments(platform, device) {
  const input = platform === "darwin" ? ["-f", "avfoundation", "-i", device || ":0"] : platform === "win32" ? ["-f", "dshow", "-i", `audio=${device || "default"}`] : ["-f", "pulse", "-i", device || "default"];
  return ["-hide_banner", "-loglevel", "error", ...input, "-t", "120", "-ar", "16000", "-ac", "1", "-f", "s16le", "pipe:1"];
}
function wave(pcm) {
  const header = Buffer.alloc(44); header.write("RIFF"); header.writeUInt32LE(36 + pcm.length, 4); header.write("WAVEfmt ", 8); header.writeUInt32LE(16, 16); header.writeUInt16LE(1, 20); header.writeUInt16LE(1, 22); header.writeUInt32LE(16000, 24); header.writeUInt32LE(32000, 28); header.writeUInt16LE(2, 32); header.writeUInt16LE(16, 34); header.write("data", 36); header.writeUInt32LE(pcm.length, 40); return Buffer.concat([header, pcm]);
}
export function recordAudio(device = process.env.GATOR_MICROPHONE_DEVICE) {
  const child = spawn("ffmpeg", recordingArguments(process.platform, device), { windowsHide: true, stdio: ["pipe", "pipe", "pipe"] });
  const chunks = []; let size = 0, failure, diagnostic = "", cancelled = false;
  let force;
  const audio = new Promise((resolve, reject) => {
    child.stdout.on("data", chunk => { size += chunk.length; if (size + 44 > MAX_AUDIO_BYTES) { failure = new Error("Recording exceeded the 8 MiB limit."); child.kill("SIGKILL"); } else chunks.push(chunk); });
    child.stderr.on("data", chunk => { diagnostic = (diagnostic + chunk).slice(-2048); });
    child.on("error", reject); child.stdin.on("error", () => {});
    child.on("close", code => { clearTimeout(force); if (cancelled) reject(new Error("Dictation cancelled. Your draft is kept.")); else if (failure || code !== 0 || !size) reject(failure || new Error(diagnostic || "No microphone audio was captured. Check OS microphone permission and the configured device.")); else resolve(wave(Buffer.concat(chunks))); });
  });
  // Avoid an unhandled rejection while the user is deciding when to stop.
  void audio.catch(() => {});
  return { audio, stop() { child.stdin.write("q\n"); force = setTimeout(() => child.kill("SIGKILL"), 3000); }, cancel() { cancelled = true; child.kill("SIGTERM"); force = setTimeout(() => child.kill("SIGKILL"), 1500); } };
}
// Keep the native process boundary injectable for contract checks on all hosts.
// Reply content always goes through stdin, never script interpolation or argv.
export function speechPlayer({ platform = process.platform, run = helper, powershell = powershellPath } = {}) {
  return async (text, { language = "", rate = 1 } = {}, signal) => {
    if (typeof text !== "string" || text.length > 128 * 1024 || !Number.isFinite(rate) || rate < 0.5 || rate > 2 || typeof language !== "string" || language.length > 64 || (language && !/^[a-z]{2,8}(?:[-_][a-z0-9]{1,8})*$/i.test(language))) throw new Error("Invalid spoken reply or voice language.");
    signal?.throwIfAborted();
    const normalize = value => value.replaceAll("_", "-").toLowerCase();
    let voice;
    if (language && ["darwin", "win32"].includes(platform)) {
      const enumeration = platform === "darwin"
        ? await run("/usr/bin/osascript", ["-l", "JavaScript", "-e", "ObjC.import('AppKit');var v=$.NSSpeechSynthesizer.availableVoices;var out=[];for(var i=0;i<v.count;i++){var a=$.NSSpeechSynthesizer.attributesForVoice(v.objectAtIndex(i));out.push({name:ObjC.unwrap(a.objectForKey($.NSVoiceName)),language:ObjC.unwrap(a.objectForKey($.NSVoiceLocaleIdentifier))});}JSON.stringify(out);"], { signal, maxBytes: 1024 * 1024 })
        : await run(powershell(), ["-NoProfile", "-NonInteractive", "-Command", "Add-Type -AssemblyName System.Speech; [Console]::OutputEncoding=[Text.UTF8Encoding]::new($false); $s=[System.Speech.Synthesis.SpeechSynthesizer]::new(); try{ConvertTo-Json -Compress -InputObject @($s.GetInstalledVoices() | Where-Object {$_.Enabled} | ForEach-Object {@{name=$_.VoiceInfo.Name;language=$_.VoiceInfo.Culture.Name}})}finally{$s.Dispose()}"], { signal, maxBytes: 1024 * 1024 });
      const voices = JSON.parse(enumeration.toString());
      if (!Array.isArray(voices)) throw new Error("The system could not list installed speech voices.");
      const valid = voices.filter(item => typeof item?.name === "string" && item.name && typeof item.language === "string");
      const requested = normalize(language), primary = requested.split("-")[0];
      voice = (valid.find(item => normalize(item.language) === requested) ?? valid.find(item => normalize(item.language).split("-")[0] === primary))?.name;
      if (!voice) throw new Error(`No installed speech voice supports ${language}. Install a matching system voice or choose another language in Voice settings.`);
    }
    signal?.throwIfAborted();
    if (platform === "darwin") await run("/usr/bin/say", ["-r", String(Math.round(175 * rate)), ...(voice ? ["-v", voice] : []), "-f", "-"], { input: text, signal });
    else if (platform === "win32") {
      // System.Speech uses an engine-specific integer scale (-10..10), with 0
      // normal. Preserve monotonic slower/faster preferences around rate 1.
      const input = JSON.stringify({ text, voice, rate: Math.round(10 * Math.log2(rate)) });
      await run(powershell(), ["-NoProfile", "-NonInteractive", "-Command", "$ErrorActionPreference='Stop'; Add-Type -AssemblyName System.Speech; [Console]::InputEncoding=[Text.UTF8Encoding]::new($false); $p=[Console]::In.ReadToEnd() | ConvertFrom-Json; $s=[System.Speech.Synthesis.SpeechSynthesizer]::new(); try{if($p.voice){$s.SelectVoice([string]$p.voice)}; $s.Rate=[int]$p.rate; $s.Speak([string]$p.text)}finally{$s.Dispose()}"], { input, signal });
    } else await run("espeak-ng", ["--stdin", "-s", String(Math.round(175 * rate)), ...(language ? ["-v", language] : [])], { input: text, signal });
  };
}
export const speak = speechPlayer();
