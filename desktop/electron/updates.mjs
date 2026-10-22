import { EventEmitter } from "node:events";

export const releaseURL = "https://github.com/gongahkia/gator/releases";

export function updateBlockReason(snapshot) {
  const state = snapshot?.state;
  if (!state || snapshot.connection !== "connected") return "Reconnect to Gator before restarting for an update.";
  if (snapshot.terminalBusy || state.external) return "Finish the current sign-in or reviewed action first.";
  if (state.running || state.cancelling || state.pending_approval) return "Finish or stop the current work before restarting.";
  if (state.queued || state.queue?.length) return "Send or remove queued messages before restarting.";
  if (state.panel?.kind === "confirmation" || state.panel?.rows?.some((row) => row.input)) return "Save or close the form you’re editing before restarting.";
  return "";
}

// Main-process owned. Updates never auto-download or install on ordinary quit.
// Only signed release builds opt into a fixed, architecture-specific feed.
export class Updates extends EventEmitter {
  #updater;
  #blocked;
  #confirm;
  #prepare;
  #busy = false;
  #rollback;
  #restoring;
  #state;
  constructor({ updater, version, supported, reason = "", arch = process.arch, blocked, confirm, prepare }) {
    super();
    this.#blocked = blocked; this.#confirm = confirm; this.#prepare = prepare;
    this.#state = { phase: supported ? "idle" : "manual", installed: version, reason };
    if (!supported) return;
    this.#updater = updater;
    updater.autoDownload = false;
    updater.autoInstallOnAppQuit = false;
    updater.allowPrerelease = false;
    updater.channel = `stable-${arch}`;
    updater.allowDowngrade = false; // Setting channel otherwise enables this.
    updater.disableWebInstaller = true;
    updater.on("error", () => {
      if (this.#state.phase === "installing") void this.#restoreAfterFailure();
      else if (!this.#busy) this.#set({ phase: "error", reason: "The update could not complete. Try again, or open the releases page." });
    });
    updater.on("download-progress", (progress) => { if (this.#state.phase === "downloading") this.#set({ percent: Math.max(0, Math.min(100, Math.floor(progress.percent || 0))) }); });
  }
  snapshot() { return { ...this.#state }; }
  #set(next) { this.#state = { ...this.#state, ...next }; this.emit("change", this.snapshot()); }
  async #restoreAfterFailure() {
    if (this.#restoring) return this.#restoring;
    this.#restoring = (async () => {
      try { await this.#rollback?.(); this.#set({ phase: "ready", reason: "The installer could not start. Gator reconnected without resubmitting work. Try again or use the releases page." }); }
      catch { this.#set({ phase: "ready", reason: "The installer could not start. Reconnect to Gator before trying again. Saved conversations and drafts are retained." }); }
      finally { this.#rollback = undefined; }
    })();
    try { await this.#restoring; } finally { this.#restoring = undefined; }
  }
  async request(request) {
    if (!request || typeof request !== "object" || Array.isArray(request) || Object.keys(request).some((key) => key !== "action") || !["check", "download", "install"].includes(request.action)) throw new Error("Invalid update request");
    if (!this.#updater) throw new Error(this.#state.reason || "Use the releases page to update this build.");
    if (this.#busy) throw new Error("An update operation is already in progress.");
    if (request.action === "download" && this.#state.phase !== "available") throw new Error("Check for a new version first.");
    if (request.action === "install" && this.#state.phase !== "ready") throw new Error("Download the update first.");
    if (request.action === "check" && ["ready", "installing"].includes(this.#state.phase)) throw new Error("An update is already ready to install.");
    this.#busy = true;
    try {
      if (request.action === "check") {
        this.#set({ phase: "checking", reason: "", version: undefined, percent: undefined });
        const result = await this.#updater.checkForUpdates();
        if (!result?.updateInfo?.version) throw new Error("No release information returned");
        const available = result.isUpdateAvailable === true;
        this.#set({ phase: available ? "available" : "current", version: available ? result.updateInfo.version : undefined, checkedAt: new Date().toISOString() });
      } else if (request.action === "download") {
        this.#set({ phase: "downloading", percent: 0, reason: "" });
        const files = await this.#updater.downloadUpdate();
        if (!Array.isArray(files) || !files.length) throw new Error("No verified update returned");
        this.#set({ phase: "ready", percent: 100 });
      } else {
        let reason = this.#blocked();
        if (reason) { this.#set({ reason }); return this.snapshot(); }
        if (!await this.#confirm(this.#state.version)) return this.snapshot();
        reason = this.#blocked(); // Work may have started while the review was open.
        if (reason) { this.#set({ reason }); return this.snapshot(); }
        this.#set({ phase: "installing", reason: "" });
        this.#rollback = await this.#prepare();
        try { this.#updater.quitAndInstall(false, true); }
        catch { await this.#restoreAfterFailure(); }
      }
    } catch {
      this.#set({ phase: request.action === "install" ? "ready" : "error", reason: request.action === "install" ? "Gator could not prepare to restart. Check Recovery for background work, then try again. Your saved work is retained." : "The update could not complete. Try again, or open the releases page." });
    } finally { this.#busy = false; }
    return this.snapshot();
  }
}
