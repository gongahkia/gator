import test from "node:test";
import assert from "node:assert/strict";
import { microphoneAllowed, validateVoiceRequest } from "../electron/voice.mjs";

test("dictation requests accept audio bytes and reject file paths and extra authority", () => {
  const recording = { action: "transcribe", context: "conversation-one", media_type: "audio/webm;codecs=opus", data: new Uint8Array([1, 2]) };
  assert.equal(validateVoiceRequest(recording), recording);
  for (const request of [{ ...recording, path: "/private/file" }, { ...recording, data: "base64" }, { ...recording, media_type: "video/webm" }, { ...recording, data: new Uint8Array() }, { action: "configure", preferences: { api_key: "secret" } }, { action: "exec", program: "sh" }]) assert.throws(() => validateVoiceRequest(request));
});

test("microphone permission belongs only to the current app window and explicit audio grant", () => {
  const owner = { getURL: () => "gator://app/index.html" }, grant = { expires: 200 };
  assert.equal(microphoneAllowed(owner, owner, "media", { mediaTypes: ["audio"] }, grant, 100), true);
  assert.equal(microphoneAllowed(owner, owner, "media", { mediaType: "audio" }, grant, 100), true);
  for (const [contents, permission, details, token, now] of [[owner, "media", { mediaTypes: ["video"] }, grant, 100], [owner, "media", { mediaTypes: ["audio"], isMainFrame: false }, grant, 100], [owner, "media", { mediaTypes: ["audio"] }, undefined, 100], [owner, "media", { mediaTypes: ["audio"] }, grant, 201], [{ getURL: owner.getURL }, "media", { mediaTypes: ["audio"] }, grant, 100], [owner, "clipboard-read", {}, grant, 100]]) assert.equal(microphoneAllowed(owner, contents, permission, details, token, now), false);
});
