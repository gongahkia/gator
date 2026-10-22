const { contextBridge, ipcRenderer, webUtils } = require("electron");

async function attachmentRequest(channel, ...args) {
  try { return await ipcRenderer.invoke(channel, ...args); }
  catch (error) { throw new Error(error.message.replace(/^Error invoking remote method '[^']+':\s*(?:Error:\s*)?/, "")); }
}

contextBridge.exposeInMainWorld("gator", {
  platform: process.platform,
  snapshot: () => ipcRenderer.invoke("gator:snapshot"),
  openSession: (id) => ipcRenderer.invoke("gator:session", id),
  send: (frame) => frame?.type === "upload_submit" ? attachmentRequest("gator:send", frame) : ipcRenderer.invoke("gator:send", frame),
  chooseFolder: () => ipcRenderer.invoke("gator:folder"),
  chooseFiles: (context) => attachmentRequest("gator:files", context),
  dropFiles: (files, context) => attachmentRequest("gator:drop-files", context, files.map((file) => webUtils.getPathForFile(file))),
  pasteImages: (images, context) => attachmentRequest("gator:paste-images", context, images),
  files: (request) => attachmentRequest("gator:file", request),
  choosePath: (request) => ipcRenderer.invoke("gator:pick-path", request),
  openLink: (url) => ipcRenderer.invoke("gator:link", url),
  continueInTerminal: () => ipcRenderer.invoke("gator:terminal"),
  restart: () => ipcRenderer.invoke("gator:restart"),
  drafts: (request) => ipcRenderer.invoke("gator:drafts", request),
  updates: (request) => ipcRenderer.invoke("gator:updates", request),
	voice: (request) => attachmentRequest("gator:voice", request),
  subscribe(callback) {
    const listener = (_event, message) => callback(message);
    ipcRenderer.on("gator:update", listener);
    return () => ipcRenderer.removeListener("gator:update", listener);
  },
});
