import { app, BrowserWindow, dialog, ipcMain, Menu, net, protocol, session, shell } from "electron";
import { mkdir, access } from "node:fs/promises";
import { constants } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { homedir } from "node:os";
import { createHash, randomUUID } from "node:crypto";
import { Backend } from "./backend.mjs";
import { APP_URL, assetPath, externalURL } from "./policy.mjs";
import { terminalHandoff } from "./terminal.mjs";
import { DraftStore } from "./drafts.mjs";

const here = dirname(fileURLToPath(import.meta.url));
const assets = resolve(here, "../dist");
const appIcon = resolve(here, "../assets/icon.png");
app.setName("Gator");
// Allows isolated development/test profiles without changing Gator's state.
if (process.env.GATOR_DESKTOP_USER_DATA) app.setPath("userData", resolve(process.env.GATOR_DESKTOP_USER_DATA));
const stateRoot = resolve(process.env.GATOR_STATE_DIR?.trim() || process.env.XDG_STATE_HOME?.trim() || join(homedir(), ".local/state"));
const drafts = new DraftStore(join(app.getPath("userData"), "drafts", `${createHash("sha256").update(stateRoot).digest("hex")}.json`));
protocol.registerSchemesAsPrivileged([{ scheme: "gator", privileges: { standard: true, secure: true, supportFetchAPI: true } }]);

let window;
let backend;
let terminalBusy = false;
let closing = false;
let starting;
let sessionID;

function update() {
  if (window && !window.isDestroyed()) window.webContents.send("gator:update", snapshot());
}
function snapshot() {
  return { state: backend?.state, error: backend?.closed ? backend.failure : undefined, connection: backend?.closed ? "disconnected" : backend?.state ? "connected" : "starting", sessionID, terminalBusy, fullScreen: window?.isFullScreen() ?? false };
}
function trusted(event) {
  if (!window || event.sender !== window.webContents || event.senderFrame !== window.webContents.mainFrame || event.senderFrame.url !== APP_URL) throw new Error("Untrusted desktop request");
}
async function startBackend() {
  if (starting) return starting;
  starting = (async () => {
    if (backend && !backend.closed) await backend.stop();
    const binary = process.env.GATOR_DESKTOP_BINARY || (app.isPackaged ? join(process.resourcesPath, "runtime/gator") : join(here, "../runtime/gator"));
    await access(binary, constants.X_OK);
    // A desktop launch never implicitly captures the user's home directory.
    const workspace = process.env.GATOR_DESKTOP_WORKSPACE || join(app.getPath("userData"), "workspace");
    if (!process.env.GATOR_DESKTOP_WORKSPACE) await mkdir(workspace, { recursive: true, mode: 0o700 });
    backend = new Backend(binary, workspace);
    sessionID = randomUUID();
    backend.on("state", update);
    backend.on("closed", update);
  })();
  try { await starting; } finally { starting = undefined; }
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
  window.webContents.on("will-navigate", (event) => event.preventDefault());
  window.webContents.on("will-attach-webview", (event) => event.preventDefault());
  window.on("enter-full-screen", update);
  window.on("leave-full-screen", update);
  window.on("ready-to-show", () => window.show());
  window.on("close", async (event) => {
    if (closing) return;
    event.preventDefault();
    if (terminalBusy) {
      await dialog.showMessageBox(window, { type: "info", message: "Finish the action in Terminal first.", detail: "Gator is waiting for its result. You can cancel the action in that terminal window.", buttons: ["Keep Gator open"] });
      return;
    }
    if (backend?.state?.running) {
      const answer = await dialog.showMessageBox(window, { type: "question", message: "Stop the current work and close Gator?", detail: "Saved conversations and scheduled checks are retained. This foreground run will stop.", buttons: ["Keep working", "Stop and close"], defaultId: 0, cancelId: 0 });
      if (answer.response !== 1) return;
    }
    closing = true;
    await drafts.flush();
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
    session.defaultSession.setPermissionRequestHandler((_contents, _permission, callback) => callback(false));
    session.defaultSession.setPermissionCheckHandler(() => false);
    Menu.setApplicationMenu(Menu.buildFromTemplate([
      ...(process.platform === "darwin" ? [{ label: "Gator", submenu: [{ role: "about" }, { type: "separator" }, { role: "hide" }, { role: "hideOthers" }, { type: "separator" }, { role: "quit" }] }] : []),
      { label: "Edit", submenu: [{ role: "undo" }, { role: "redo" }, { type: "separator" }, { role: "cut" }, { role: "copy" }, { role: "paste" }, { role: "selectAll" }] },
      { label: "View", submenu: [{ role: "reload" }, { role: "resetZoom" }, { role: "zoomIn" }, { role: "zoomOut" }, { role: "togglefullscreen" }, ...(!app.isPackaged ? [{ role: "toggleDevTools" }] : [])] },
      { label: "Window", submenu: [{ role: "minimize" }, { role: "close" }] },
    ]));
    ipcMain.handle("gator:snapshot", (event) => { trusted(event); return snapshot(); });
    ipcMain.handle("gator:drafts", (event, request) => { trusted(event); return drafts.request(request); });
    ipcMain.handle("gator:send", (event, frame) => { trusted(event); if (backend?.state?.external) throw new Error("Finish the current step in Terminal first"); backend.send(frame); });
    ipcMain.handle("gator:folder", async (event) => {
      trusted(event);
      if (backend?.state?.running || backend?.state?.panel) return null;
      const result = await dialog.showOpenDialog(window, { title: "Choose Gator's workspace", properties: ["openDirectory"] });
      if (result.canceled || !result.filePaths[0] || backend?.state?.running || backend?.state?.panel) return null;
      backend.send({ type: "submit", text: `/source ${result.filePaths[0]}` });
      return result.filePaths[0];
    });
    ipcMain.handle("gator:link", async (event, value) => { trusted(event); const url = externalURL(value); if (!url) throw new Error("Only web links can be opened"); await shell.openExternal(url); });
    ipcMain.handle("gator:restart", async (event) => { trusted(event); if (terminalBusy || (backend?.state?.running && !backend.closed)) throw new Error("Finish the current operation first"); await startBackend(); update(); });
    ipcMain.handle("gator:terminal", async (event) => {
      trusted(event);
      if (!backend?.state?.external || terminalBusy) return;
      const owner = backend;
      const job = owner.state.external;
      terminalBusy = true;
      update();
      let failure = "";
      try { failure = await terminalHandoff(job); } catch (error) { failure = `Could not complete the terminal action: ${error.message}`; }
      finally { terminalBusy = false; owner.finishExternal(failure); update(); }
    });
    await startBackend();
    createWindow();
  }).catch((error) => { dialog.showErrorBox("Gator could not start", error.message); app.exit(1); });
  app.on("before-quit", (event) => { if (!closing && window && !window.isDestroyed()) { event.preventDefault(); window.close(); } });
  app.on("window-all-closed", () => app.quit());
  app.on("activate", () => window?.show());
}
