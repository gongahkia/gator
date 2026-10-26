import { EventEmitter } from "node:events";
import { runSpeechRequest } from "./voice.mjs";
import { recordAudio, speak } from "./native.mjs";

// Binds explicit microphone grants and transient audio to one live chat.
export class VoiceSession extends EventEmitter {
  phase = "idle";
  #recording;
  #abort;
  #generation = 0;
  #seen = new Map();
  #playback;
  constructor(application, { capture = recordAudio, call = runSpeechRequest, playback = speak } = {}) {
    super(); this.application = application; this.capture = capture; this.call = call; this.playback = playback;
    this.changed = () => {
      const context = application.context, state = application.snapshot().state;
      if (this.context && this.context !== context) this.cancel();
      if (!state || !context) return;
      const count = this.#seen.get(context), messages = state.messages ?? [];
      this.#seen.set(context, messages.length);
      if (count === undefined || count >= messages.length || !state.desktop?.voice?.spoken_replies) return;
      const replies = messages.slice(count).filter(message => message.role === "Gator" && message.text && message.result?.kind !== "approval");
      if (!replies.length) return;
      this.#playback?.abort(); const controller = new AbortController(); this.#playback = controller;
      void playback(replies.map(message => message.text).join("\n"), state.desktop.voice, controller.signal).catch(error => { if (!controller.signal.aborted) { this.issue = error.message; this.emit("change"); } });
    };
    application.on("change", this.changed);
  }
  async request(request, signal) { return this.call(this.application.binary, this.application.workspace, this.application.env, request, signal); }
  status() { return this.request({ action: "status" }); }
  async configure(preferences) { const status = await this.request({ action: "configure", preferences }); this.application.backend?.refreshPreferences(); return status; }
  async start() {
    this.application.mutable();
    if (this.phase !== "idle" || this.application.snapshot().state?.panel) throw new Error("Return to the conversation and finish the current dictation first.");
    const generation = ++this.#generation, context = this.application.context;
    const status = await this.status();
    if (!status.ready) throw new Error(status.issue || "Configure Gemini or OpenAI for dictation first.");
    if (generation !== this.#generation || context !== this.application.context) throw new Error("The conversation changed before microphone capture started.");
    this.context = context; this.route = status.route; this.issue = "";
    this.#recording = this.capture(); this.phase = "recording"; this.emit("change");
    void this.#recording.audio.catch(error => { if (generation === this.#generation && this.phase === "recording") { this.issue = error.message; this.phase = "idle"; this.#recording = undefined; this.emit("change"); } });
  }
  async stop() {
    const recording = this.#recording, generation = this.#generation;
    if (!recording || this.phase !== "recording") throw new Error("Start dictation explicitly before stopping it.");
    this.phase = "transcribing"; this.emit("change"); recording.stop();
    const controller = new AbortController(); this.#abort = controller;
    try {
      const bytes = await recording.audio;
      if (generation !== this.#generation || this.context !== this.application.context) throw new Error("Dictation cancelled or chat changed.");
      const result = await this.request({ action: "transcribe", route: this.route, recording: { media_type: "audio/wav", data: Buffer.from(bytes).toString("base64") } }, controller.signal);
      if (generation !== this.#generation || this.context !== this.application.context) throw new Error("The chat changed. Transcription was not added to another draft.");
      return { context: this.context, text: result.text };
    } finally { if (generation === this.#generation) { this.phase = "idle"; this.#recording = undefined; this.#abort = undefined; this.emit("change"); } }
  }
  cancel() { this.#generation++; this.#recording?.cancel(); this.#abort?.abort(); this.#recording = undefined; this.#abort = undefined; this.context = undefined; this.phase = "idle"; this.emit("change"); }
  close() { this.cancel(); this.#playback?.abort(); this.application.off("change", this.changed); }
}
