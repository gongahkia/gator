import { EventEmitter } from "node:events";
import { spawn } from "node:child_process";
import { mkdir, writeFile } from "node:fs/promises";
import { join, resolve } from "node:path";
import { createHash, randomUUID } from "node:crypto";
import { Backend } from "./backend.mjs";
import { Sessions } from "./sessions.mjs";
import { DraftStore } from "./drafts.mjs";
import { UploadStore, uploadContext } from "./uploads.mjs";
import { FileStore, readBoundedFile, validateFileRequest } from "./files.mjs";
import { selectedFileSource } from "./file-sources.mjs";
import { stateDirectory } from "./platform.mjs";
import { validateFrame } from "./policy.mjs";
import { retainedRequestUploads } from "./request-retention.mjs";
import { renderPDF } from "./pdf.mjs";
import { importPastedImages } from "./pasted-images.mjs";
import { helper } from "./native.mjs";

// Local application services. No renderer, IPC, HTTP server, or second agent.
export class Application extends EventEmitter {
  imported = new Map();
  terminalBusy = false;
  constructor({ binary, workspace, userData, env = process.env }) {
    super();
    this.binary = resolve(binary);
    this.workspace = resolve(workspace);
    this.userData = resolve(userData);
    this.stateRoot = stateDirectory(env);
    this.uploads = new UploadStore(join(this.stateRoot, "gator", "uploads"));
    this.files = new FileStore(join(this.stateRoot, "gator", "files"));
    const draftRoot = join(this.userData, "drafts");
    this.drafts = new DraftStore(join(draftRoot, `${createHash("sha256").update(this.stateRoot).digest("hex")}.json`));
    this.env = { ...env, GATOR_DESKTOP_DRAFT_DIR: draftRoot, GATOR_DESKTOP_UPLOAD_DIR: this.uploads.root };
    if (process.platform === "win32") {
      this.env.GATOR_STATE_DIR = this.stateRoot;
      for (const key of ["GATOR_CONFIG_DIR", "XDG_CONFIG_HOME"]) if (env[key]?.trim()) this.env[key] = resolve(env[key]);
    }
    this.sessions = new Sessions(request => new Backend(this.binary, this.workspace, {
      ...this.env, GATOR_UI_SESSION_HOST: "1",
      GATOR_UI_VIEW_ORIGIN: request.view ? request.origin_conversation_id || "" : "",
      GATOR_UI_BOOT_MODE: request.new ? "new" : request.view ? "view" : "",
      GATOR_UI_START_SOURCE: request.source || (request.new || request.view ? this.sessions.current?.state?.source || "" : ""),
    }, ["ui-backend", request.goal ? "" : request.conversation_id || "", request.goal ? request.conversation_id : ""]));
    this.sessions.on("change", () => this.emit("change", this.snapshot()));
  }
  get backend() { return this.sessions.current; }
  get context() { return uploadContext(this.backend?.state); }
  snapshot() {
    return { state: this.backend?.state, error: this.backend?.closed ? this.backend.failure : undefined,
      connection: this.backend?.closed ? "disconnected" : this.backend?.state ? "connected" : "starting",
      sessionSwitching: this.sessions.switching, sessionIssue: this.sessions.issue,
      chatActivity: this.sessions.activity(), allChatsBusy: this.sessions.busy,
      allChatsQueued: this.sessions.pending, terminalBusy: this.terminalBusy };
  }
  async start() {
    await mkdir(this.workspace, { recursive: true, mode: 0o700 });
    // Malformed journals defer cleanup rather than deleting potentially admitted inputs.
    try {
      const retained = await retainedRequestUploads(join(this.stateRoot, "gator", "conversations"));
      const saved = await this.drafts.request({ action: "list" });
      for (const draft of saved.drafts) for (const file of draft.uploads ?? []) retained.add(file.id);
      await this.uploads.prune(Date.now(), retained);
    } catch (error) { this.cleanupIssue = error.message; }
    await this.sessions.open();
    return this.snapshot();
  }
  mutable() {
    if (this.sessions.switching) throw new Error("Wait for this conversation to open.");
    if (!this.backend || this.backend.closed) throw new Error("Reconnect to this chat before continuing.");
    if (this.backend.state?.external || this.terminalBusy) throw new Error("Finish the external step first.");
  }
  async send(frame) {
    this.mutable(); validateFrame(frame); this.sessions.assertSend(frame);
    if (await this.sessions.resumeConnection(frame)) return;
    await this.sessions.prepareProfileAction(frame);
    if (frame.type !== "upload_submit") {
      await this.backend.apply(frame); await this.sessions.settled(); return;
    }
    const owner = this.backend;
    if (frame.upload.files.length) await this.uploads.check(frame.upload.files);
    this.mutable();
    if (owner !== this.backend || uploadContext(owner.state) !== frame.upload.context) throw new Error("The conversation changed. Your draft is kept.");
    await owner.submitUpload(frame);
    await this.sessions.settled();
  }
  async importFiles(paths, context) {
    this.mutable();
    if (context !== this.context) throw new Error("Return to the intended conversation before attaching files.");
    const owner = this.backend, imported = await this.uploads.import(paths);
    if (owner !== this.backend || context !== this.context) throw new Error("The conversation changed before attaching files.");
    for (const file of imported) {
      this.imported.set(file.id, context);
      await this.files.retain({ kind: "upload", id: file.id }, file.name, await readBoundedFile(this.uploads.root, file.id));
    }
    return imported;
  }
  async file(request) {
    this.mutable(); validateFileRequest(request);
    const owner = this.backend, context = request.context;
    const current = () => this.backend === owner && !owner.closed && !owner.state?.external && uploadContext(owner.state) === context;
    if (!current()) throw new Error("Return to the same conversation before opening this file.");
    const result = await this.files.request(request, async ref => {
      const saved = await this.drafts.request({ action: "list" });
      const source = await selectedFileSource(ref, { state: owner.state, context, drafts: saved, imported: this.imported, uploads: this.uploads, files: this.files });
      if (!current()) throw new Error("The conversation changed. File edits are retained.");
      return source;
    });
    if (!current()) throw new Error("The conversation changed. File edits are retained.");
    if (request.action === "attach") {
      const file = await this.uploads.importContent(result.name, result.bytes);
      this.imported.set(file.id, context);
      await this.files.link({ kind: "upload", id: file.id }, request.ref, result.bytes);
      return file;
    }
    return result;
  }
  async exportFile(request, destination) {
    const result = await this.file({ ...request, action: "export" });
    await writeFile(destination, result.bytes, { mode: 0o600, flag: "wx" });
  }
  async pdf(request, page, width) {
    const view = await this.file({ ...request, action: "open" });
    if (view.kind !== "pdf") throw new Error("Choose a PDF file for page preview.");
    return renderPDF(new Uint8Array(Buffer.from(view.data, "base64")), page, width);
  }
  async pasteImages(images, decode) {
    this.mutable();
    const context = this.context, owner = this.backend;
    const selected = await importPastedImages(this.uploads, images, decode);
    if (this.backend !== owner || this.context !== context) throw new Error("The chat changed before image intake completed.");
    for (const file of selected) {
      this.imported.set(file.id, context);
      await this.files.retain({ kind: "upload", id: file.id }, file.name, await readBoundedFile(this.uploads.root, file.id));
    }
    return selected;
  }
  async openSession(id) {
    if (typeof id !== "string" || !this.sessions.activity().some(row => row.id === id)) throw new Error("This chat is no longer in the open-chat list.");
    await this.sessions.open({ conversation_id: id });
  }
  async version() { return (await helper(this.binary, ["--version"], { maxBytes: 4096 })).toString("utf8").trim().replace("Gator desktop runtime", "Gator terminal"); }
  async runExternal() {
    const owner = this.backend, job = owner?.state?.external;
    if (!job || owner.closed || this.terminalBusy || this.sessions.switching) throw new Error("There is no current reviewed external step.");
    this.terminalBusy = true; this.emit("change", this.snapshot());
    let failure = "";
    try {
      const environment = { ...this.env, GATOR_INTERNAL_COMMANDS: "1" };
      for (const entry of job.env ?? []) if (/^(GATOR_STATE_DIR|GATOR_CONFIG_DIR)=/.test(entry)) { const split = entry.indexOf("="); environment[entry.slice(0, split)] = entry.slice(split + 1); }
      await new Promise((resolve, reject) => {
        const child = spawn(job.program, job.args ?? [], { cwd: job.dir || this.workspace, env: environment, stdio: "inherit", windowsHide: false });
        child.once("error", reject);
        child.once("close", code => code === 0 ? resolve() : reject(new Error(`The external step ended with status ${code}.`)));
      });
    } catch (error) { failure = error.message; }
    finally { this.terminalBusy = false; owner.finishExternal(failure); this.emit("change", this.snapshot()); }
    if (failure) throw new Error(failure);
  }
  async close() { if (this.terminalBusy) throw new Error("Finish or cancel the external step before closing Gator."); await this.drafts.flush(); await this.files.flush(); await this.sessions.stop(); }
}
