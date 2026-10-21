import { EventEmitter } from "node:events";
import { spawn } from "node:child_process";
import { validateFrame } from "./policy.mjs";

// The host owns one Go session. No HTTP listener, browser credential storage,
// alternate agent loop, or renderer-side subprocess execution is involved.
export class Backend extends EventEmitter {
  state;
  failure = "";
  closed = false;
  #child;
  #buffer = "";
  #diagnostics = "";

  constructor(binary, cwd, env = process.env) {
    super();
    this.#child = spawn(binary, ["ui-backend", "", ""], { cwd, env: { ...env, GATOR_UI_BACKEND: "1" }, stdio: ["pipe", "pipe", "pipe"] });
    this.#child.stdout.setEncoding("utf8");
    this.#child.stdout.on("data", (chunk) => this.#consume(chunk));
    this.#child.stderr.on("data", (chunk) => { this.#diagnostics = (this.#diagnostics + chunk.toString()).slice(-4000); });
    this.#child.stdin.on("error", () => {});
    this.#child.on("error", (error) => { this.failure = error.message; });
    this.#child.on("close", (code) => {
      this.closed = true;
      this.failure ||= this.#diagnostics.trim() || (code === 0 ? "Gator session closed." : `Gator exited (${code ?? "signal"}).`);
      this.emit("closed", this.failure);
    });
  }

  #consume(chunk) {
    this.#buffer += chunk;
    if (Buffer.byteLength(this.#buffer) > 16 * 1024 * 1024) { this.failure = "Gator sent an oversized state frame."; this.stop(); return; }
    let newline;
    while ((newline = this.#buffer.indexOf("\n")) >= 0) {
      const line = this.#buffer.slice(0, newline);
      this.#buffer = this.#buffer.slice(newline + 1);
      try {
        const frame = JSON.parse(line);
        if (frame.version !== 1 || frame.type !== "state" || !frame.state || typeof frame.state !== "object") throw new Error("Invalid state frame");
        this.state = frame.state;
        this.emit("state", this.state);
      } catch {
        this.failure = "Gator's UI connection failed. Restart the session to reconnect.";
        this.stop();
        return;
      }
    }
  }

  send(frame) {
    const checked = validateFrame(frame);
    if (this.closed || !this.#child.stdin.writable) throw new Error(this.failure || "Gator is disconnected");
    this.#write(checked);
  }

  finishExternal(error = "") {
    this.#write({ version: 1, type: "external_done", error });
  }

  #write(frame) {
    if (!this.closed && this.#child.stdin.writable) this.#child.stdin.write(`${JSON.stringify(frame)}\n`);
  }

  stop() {
    if (this.closed) return Promise.resolve();
    return new Promise((resolve) => {
      const timeout = setTimeout(() => this.#child.kill("SIGTERM"), 2500);
      const force = setTimeout(() => this.#child.kill("SIGKILL"), 5000);
      this.#child.once("close", () => { clearTimeout(timeout); clearTimeout(force); resolve(); });
      this.#write({ version: 1, type: "exit" });
      this.#child.stdin.end();
    });
  }
}
