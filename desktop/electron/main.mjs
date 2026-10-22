import { app, BrowserWindow, dialog, ipcMain, Menu, net, protocol, session, shell, systemPreferences } from "electron";
import { mkdir, access, readFile, writeFile } from "node:fs/promises";
import { constants, existsSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { createHash, randomUUID } from "node:crypto";
import { Backend } from "./backend.mjs";
import { APP_URL, assetPath, externalURL, validateFrame } from "./policy.mjs";
import { UploadStore, uploadContext, uploadExtensions } from "./uploads.mjs";
import { terminalHandoff } from "./terminal.mjs";
import { DraftStore } from "./drafts.mjs";
import { runtimeName, stateDirectory } from "./platform.mjs";
import { Updates, releaseURL, updateBlockReason } from "./updates.mjs";
import { acquireUpdateGuard } from "./update-guard.mjs";
import { microphoneAllowed, runSpeechRequest, validateVoiceRequest } from "./voice.mjs";
import { FileStore, readBoundedFile, validateFileRequest } from "./files.mjs";
import { selectedFileSource } from "./file-sources.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const assets = resolve(here, "../dist");
const appIcon = resolve(here, "../assets/icon.png");
app.setName("Gator");
if (process.platform === "win32") app.setAppUserModelId("com.gongahkia.gator");
// Allows isolated development/test profiles without changing Gator's state.
if (process.env.GATOR_DESKTOP_USER_DATA) app.setPath("userData", resolve(process.env.GATOR_DESKTOP_USER_DATA));
const stateRoot = stateDirectory();
const uploads = new UploadStore(join(stateRoot, "gator", "uploads"));
const files = new FileStore(join(stateRoot, "gator", "files"));
const importedFiles = new Map();
const drafts = new DraftStore(join(app.getPath("userData"), "drafts", `${createHash("sha256").update(stateRoot).digest("hex")}.json`));
protocol.registerSchemesAsPrivileged([{ scheme: "gator", privileges: { standard: true, secure: true, supportFetchAPI: true } }]);

let window;
let backend;
let terminalBusy = false;
let closing = false;
let starting;
let sessionID;
let updates;
let updatesOpen = false;
let updatePreparing = false;
let microphoneGrant;
let speechAbort;
let voiceGeneration = 0;

const runtimeBinary = () => process.env.GATOR_DESKTOP_BINARY || (app.isPackaged ? join(process.resourcesPath, "runtime", runtimeName()) : join(here, "../runtime", runtimeName()));
const runtimeWorkspace = () => process.env.GATOR_DESKTOP_WORKSPACE || join(app.getPath("userData"), "workspace");
function runtimeEnvironment() {
  const environment = { ...process.env, GATOR_DESKTOP_DRAFT_DIR: join(app.getPath("userData"), "drafts"), GATOR_DESKTOP_UPLOAD_DIR: uploads.root };
  // Preserve existing Unix workspace-relative overrides. The Windows port
  // already resolves its profile overrides in the host before changing cwd.
  if (process.platform === "win32") {
    environment.GATOR_STATE_DIR = stateRoot;
    for (const key of ["GATOR_CONFIG_DIR", "XDG_CONFIG_HOME"]) if (environment[key]?.trim()) environment[key] = resolve(environment[key]);
  }
  return environment;
}

function update() {
  if (window && !window.isDestroyed()) window.webContents.send("gator:update", snapshot());
}
function snapshot() {
  return { state: backend?.state, error: backend?.closed ? backend.failure : undefined, connection: backend?.closed ? "disconnected" : backend?.state ? "connected" : "starting", sessionID, terminalBusy, fullScreen: window?.isFullScreen() ?? false, updates: updates?.snapshot(), updatesOpen, updatePreparing };
}
function trusted(event) {
  if (!window || event.sender !== window.webContents || event.senderFrame !== window.webContents.mainFrame || event.senderFrame.url !== APP_URL) throw new Error("Untrusted desktop request");
}
function mutable(event) { trusted(event); if (updatePreparing) throw new Error("Gator is preparing to restart. Please wait."); }
async function startBackend() {
  if (starting) return starting;
  starting = (async () => {
    if (backend && !backend.closed) await backend.stop();
    const binary = runtimeBinary();
    await access(binary, constants.X_OK);
    // A desktop launch never implicitly captures the user's home directory.
    const workspace = runtimeWorkspace();
    if (!process.env.GATOR_DESKTOP_WORKSPACE) await mkdir(workspace, { recursive: true, mode: 0o700 });
    backend = new Backend(binary, workspace, runtimeEnvironment());
    sessionID = randomUUID();
    backend.on("state", update);
    backend.on("closed", update);
  })();
  try { await starting; } finally { starting = undefined; }
}

async function configureUpdates() {
  const metadata = JSON.parse(await readFile(join(app.getAppPath(), "package.json"), "utf8"));
  const official = app.isPackaged && metadata.gatorDistribution?.released === true;
  const signed = official && metadata.gatorDistribution?.signed === true;
  const installedWindows = process.platform !== "win32" || existsSync(join(dirname(app.getPath("exe")), "Uninstall Gator.exe"));
  const supported = signed && ["darwin", "win32"].includes(process.platform) && installedWindows;
  const reason = !official ? "This development build uses manual updates. Signed releases can check and download updates here." : !installedWindows ? "This portable copy uses manual updates. Install the Windows setup version for in-app updates." : "Update this installation through your package manager or the releases page.";
  const updater = supported ? (await import("electron-updater")).default.autoUpdater : undefined;
  updates = new Updates({ updater, supported, reason: supported ? "" : reason, version: app.getVersion(),
    blocked: () => updateBlockReason(snapshot()),
    confirm: async (version) => (await dialog.showMessageBox(window, { type: "question", message: `Restart Gator to install ${version}?`, detail: "Saved conversations, settings and retained drafts stay on this device. Work does not resume or send automatically after restarting. If background checks are running, disable start at login and stop them in Recovery first. Re-enable them after updating.", buttons: ["Keep Gator open", "Restart and install"], defaultId: 0, cancelId: 0 })).response === 1,
    prepare: async () => {
      updatePreparing = true; update();
      try {
        await drafts.flush(); await files.flush();
        await backend.prepareUpdate();
        const guard = await acquireUpdateGuard(runtimeBinary(), runtimeWorkspace(), runtimeEnvironment());
        await guard.release();
        closing = true;
      } catch (error) {
        updatePreparing = false;
        if (backend?.closed) await startBackend();
        update();
        throw error;
      }
      return async () => { closing = false; updatePreparing = false; await startBackend(); update(); };
    },
  });
  updates.on("change", update);
}

function createWindow() {
  window = new BrowserWindow({
    title: "Gator", width: 1440, height: 960, minWidth: 760, minHeight: 560,
    backgroundColor: "#101010", show: false, icon: appIcon,
    titleBarStyle: process.platform === "darwin" ? "hiddenInset" : "default",
    trafficLightPosition: { x: 20, y: 20 },
    webPreferences: { preload: join(here, "preload.cjs"), contextIsolation: true, sandbox: true, nodeIntegration: false, webviewTag: false, spellcheck: false },
  });
  window.webContents.setWindowOpenHandler(() => ({ action: "deny" }));
	window.on("closed", () => { microphoneGrant = undefined; speechAbort?.abort(); });
  window.webContents.on("will-navigate", (event) => event.preventDefault());
  window.webContents.on("will-attach-webview", (event) => event.preventDefault());
  window.on("enter-full-screen", update);
  window.on("leave-full-screen", update);
  window.on("ready-to-show", () => window.show());
  window.on("close", async (event) => {
    if (closing) return;
    event.preventDefault();
    if (updatePreparing) return;
    if (terminalBusy) {
      await dialog.showMessageBox(window, { type: "info", message: "Finish the action in Terminal first.", detail: "Gator is waiting for its result. You can cancel the action in that terminal window.", buttons: ["Keep Gator open"] });
      return;
    }
    if (backend?.state?.running) {
      const answer = await dialog.showMessageBox(window, { type: "question", message: "Stop the current work and close Gator?", detail: "Saved conversations and scheduled checks are retained. This foreground run will stop.", buttons: ["Keep working", "Stop and close"], defaultId: 0, cancelId: 0 });
      if (answer.response !== 1) return;
    }
    closing = true;
    try { await drafts.flush(); await files.flush(); }
    catch {
      closing = false;
      const answer = await dialog.showMessageBox(window, { type: "warning", message: "Your unsent work could not be saved.", detail: "Keep Gator open to copy your message and file edits, or close without saving those changes.", buttons: ["Keep Gator open", "Close without saving"], defaultId: 0, cancelId: 0 });
      if (answer.response !== 1) return;
      closing = true;
    }
    await backend?.stop();
    window.destroy();
    app.quit();
  });
  window.loadURL(APP_URL);
}

if (!app.requestSingleInstanceLock()) app.quit();
else {
  app.on("second-instance", () => { window?.show(); window?.focus(); });
  app.whenReady().then(async () => {
    if (process.platform === "darwin") app.dock?.setIcon(appIcon);
    protocol.handle("gator", (request) => {
      const path = assetPath(request.url, assets);
      return path ? net.fetch(pathToFileURL(path).href) : new Response("Not found", { status: 404 });
    });
    session.defaultSession.setPermissionRequestHandler((contents, permission, callback, details) => callback(microphoneAllowed(window?.webContents, contents, permission, details, microphoneGrant)));
    session.defaultSession.setPermissionCheckHandler((contents, permission, _origin, details) => microphoneAllowed(window?.webContents, contents, permission, details, microphoneGrant));
    await configureUpdates();
    const showUpdates = () => { updatesOpen = true; update(); };
    Menu.setApplicationMenu(Menu.buildFromTemplate([
      ...(process.platform === "darwin" ? [{ label: "Gator", submenu: [{ label: "About & updates…", click: showUpdates }, { type: "separator" }, { role: "hide" }, { role: "hideOthers" }, { type: "separator" }, { role: "quit" }] }] : []),
      { label: "Edit", submenu: [{ role: "undo" }, { role: "redo" }, { type: "separator" }, { role: "cut" }, { role: "copy" }, { role: "paste" }, { role: "selectAll" }] },
      { label: "View", submenu: [{ role: "reload" }, { role: "resetZoom" }, { role: "zoomIn" }, { role: "zoomOut" }, { role: "togglefullscreen" }, ...(!app.isPackaged ? [{ role: "toggleDevTools" }] : [])] },
      { label: "Window", submenu: [{ role: "minimize" }, { role: "close" }] },
      { label: "Help", submenu: [{ label: "About & updates…", click: showUpdates }, { label: "Release notes", click: () => void shell.openExternal(releaseURL) }] },
    ]));
    ipcMain.handle("gator:snapshot", (event) => { trusted(event); return snapshot(); });
    ipcMain.handle("gator:file", async (event, request) => {
      mutable(event); validateFileRequest(request);
      const owner = backend, context = request.context;
      const current = () => backend === owner && !owner?.closed && !owner?.state?.external && uploadContext(owner?.state) === context;
      if (!current()) throw new Error("Return to the same conversation before opening or editing this file.");
      const result = await files.request(request, async ref => {
        const saved = await drafts.request({ action: "list" });
        const source = await selectedFileSource(ref, { state: owner.state, context, drafts: saved, imported: importedFiles, uploads, files });
        if (!current()) throw new Error("The conversation changed. Your file edits are retained.");
        return source;
      });
      if (!current()) throw new Error("The conversation changed. Your file edits are retained.");
      if (request.action === "attach") {
        const uploaded = await uploads.importContent(result.name, result.bytes); importedFiles.set(uploaded.id, context);
        await files.link({ kind: "upload", id: uploaded.id }, request.ref, result.bytes); return uploaded;
      }
      if (request.action === "export") {
        const selected = await dialog.showSaveDialog(window, { title: "Save a copy", defaultPath: result.name });
        if (selected.canceled || !selected.filePath) return { exported: false };
        if (!current()) throw new Error("The conversation changed before export. Try again in the intended conversation.");
        await writeFile(selected.filePath, result.bytes, { mode: 0o600 }); return { exported: true };
      }
      return result;
    });
    ipcMain.handle("gator:voice", async (event, value) => {
      mutable(event);
      const request = validateVoiceRequest(value);
      const call = (payload, signal) => runSpeechRequest(runtimeBinary(), runtimeWorkspace(), runtimeEnvironment(), payload, signal);
      if (request.action === "release" || request.action === "cancel") { voiceGeneration++; microphoneGrant = undefined; if (request.action === "cancel") speechAbort?.abort(); return {}; }
      if (request.action === "status") return call({ action: "status" });
      if (request.action === "configure") { const result = await call({ action: "configure", preferences: request.preferences }); backend?.refreshPreferences(); return result; }
      const owner = backend;
      const context = uploadContext(owner?.state);
      if (!owner || owner.closed || owner.state?.panel || owner.state?.external || request.context !== context) throw new Error("Return to the same conversation before using dictation.");
      if (request.action === "authorize") {
		const generation = ++voiceGeneration;
        if (speechAbort) throw new Error("Finish the current transcription first.");
        const status = await call({ action: "status" });
        if (!status.ready) throw new Error(status.issue || "Configure a voice provider first.");
        if (process.platform === "darwin" && !await systemPreferences.askForMediaAccess("microphone")) throw new Error("Allow microphone access for Gator in macOS System Settings → Privacy & Security → Microphone.");
        if (generation !== voiceGeneration || backend !== owner || uploadContext(owner.state) !== context) throw new Error("Dictation was cancelled or the conversation changed before recording started.");
        microphoneGrant = { context, route: status.route, expires: Date.now() + 130000 };
        return { ...status, allowed: true };
      }
      if (!microphoneGrant || microphoneGrant.context !== context || microphoneGrant.expires < Date.now() || speechAbort) throw new Error("Start dictation from the microphone button before transcribing.");
      const grant = microphoneGrant;
      microphoneGrant = undefined;
      const controller = new AbortController(); speechAbort = controller;
      try {
        const result = await call({ action: "transcribe", route: grant.route, recording: { media_type: request.media_type, data: Buffer.from(request.data).toString("base64") } }, controller.signal);
        if (backend !== owner || uploadContext(owner.state) !== context) throw new Error("The conversation changed. The transcript was not added to another chat.");
        return result;
      } finally { if (speechAbort === controller) speechAbort = undefined; }
    });
    ipcMain.handle("gator:drafts", (event, request) => { mutable(event); return drafts.request(request); });
    ipcMain.handle("gator:updates", async (event, request) => {
      trusted(event);
      if (!request || typeof request !== "object" || Array.isArray(request) || Object.keys(request).some((key) => key !== "action")) throw new Error("Invalid update action");
      if (request.action === "open" || request.action === "close") { updatesOpen = request.action === "open"; update(); return updates.snapshot(); }
      if (request.action === "releases") { await shell.openExternal(releaseURL); return updates.snapshot(); }
      return updates.request(request);
    });
    ipcMain.handle("gator:send", async (event, frame) => {
      mutable(event);
      if (backend?.state?.external) throw new Error("Finish the current step in Terminal first");
      validateFrame(frame);
      if (frame.type !== "upload_submit") return backend.send(frame);
      const owner = backend;
      await uploads.check(frame.upload.files);
      mutable(event);
      if (backend !== owner || owner.closed || uploadContext(owner.state) !== frame.upload.context) throw new Error("The conversation changed. Review your attachments before sending.");
      return owner.submitUpload(frame);
    });
    const importFiles = async (event, context, select) => {
      mutable(event);
      const owner = backend;
      const current = () => backend === owner && !owner.closed && !owner.state?.panel && !owner.state?.external && uploadContext(owner.state) === context;
      if (typeof context !== "string" || !context || !current()) throw new Error("Return to your conversation before attaching files.");
      const paths = await select();
      if (!paths.length) return [];
      mutable(event);
      if (!current()) throw new Error("The conversation changed. Attach these files again in the intended conversation.");
      const selected = await uploads.import(paths);
      if (!current()) throw new Error("The conversation changed. Attach these files again in the intended conversation.");
      for (const file of selected) {
        importedFiles.set(file.id, context);
        await files.retain({ kind: "upload", id: file.id }, file.name, await readBoundedFile(uploads.root, file.id));
      }
      return selected;
    };
    ipcMain.handle("gator:files", (event, context) => importFiles(event, context, async () => {
      const result = await dialog.showOpenDialog(window, { title: "Attach files to your message", buttonLabel: "Attach", properties: ["openFile", "multiSelections"], filters: [{ name: "Documents, text and images", extensions: uploadExtensions }, { name: "All files", extensions: ["*"] }] });
      return result.canceled ? [] : result.filePaths;
    }));
    ipcMain.handle("gator:drop-files", (event, context, paths) => importFiles(event, context, async () => paths));
    ipcMain.handle("gator:folder", async (event) => {
      mutable(event);
      if (backend?.state?.running || backend?.state?.panel) return null;
      const result = await dialog.showOpenDialog(window, { title: "Choose Gator's workspace", properties: ["openDirectory"] });
      if (result.canceled || !result.filePaths[0] || backend?.state?.running || backend?.state?.panel) return null;
      backend.send({ type: "submit", text: `/source ${result.filePaths[0]}` });
      return result.filePaths[0];
    });
    ipcMain.handle("gator:pick-path", async (event, request) => {
      mutable(event);
      const owner = backend;
      const panel = owner?.state?.panel;
      if (!request || !["directory", "file", "save"].includes(request.kind) || !panel?.id || request.panel_id !== panel.id || owner.state.running || owner.state.external || !panel.rows?.some((row) => row.input?.kind === request.kind || request.kind === "directory" && row.input?.kind === "mappings")) return null;
      const result = request.kind === "save"
        ? await dialog.showSaveDialog(window, { title: "Choose a destination" })
        : await dialog.showOpenDialog(window, { title: request.kind === "directory" ? "Choose a folder" : "Choose a file", properties: [request.kind === "directory" ? "openDirectory" : "openFile"] });
      if (result.canceled || backend !== owner || owner.state.running || owner.state.external || owner.state.panel?.id !== panel.id) return null;
      // Selection only fills the reviewed form. No source command, file write,
      // restore, export, or other agent action is sent by this picker.
      return request.kind === "save" ? result.filePath || null : result.filePaths[0] || null;
    });
    ipcMain.handle("gator:link", async (event, value) => { trusted(event); const url = externalURL(value); if (!url) throw new Error("Only web links can be opened"); await shell.openExternal(url); });
    ipcMain.handle("gator:restart", async (event) => { mutable(event); if (terminalBusy || (backend?.state?.running && !backend.closed)) throw new Error("Finish the current operation first"); await startBackend(); update(); });
    ipcMain.handle("gator:terminal", async (event) => {
      mutable(event);
      if (!backend?.state?.external || terminalBusy) return;
      const owner = backend;
      const job = owner.state.external;
      terminalBusy = true;
      update();
      let failure = "";
      try { failure = await terminalHandoff(job); } catch (error) { failure = `Could not complete the terminal action: ${error.message}`; }
      finally { terminalBusy = false; owner.finishExternal(failure); update(); }
    });
    await uploads.prune();
    await startBackend();
    createWindow();
  }).catch((error) => { console.error("Gator could not start:", error.message); dialog.showErrorBox("Gator could not start", error.message); app.exit(1); });
  app.on("before-quit", (event) => { if (!closing && window && !window.isDestroyed()) { event.preventDefault(); window.close(); } });
  app.on("window-all-closed", () => app.quit());
  app.on("activate", () => window?.show());
}
