const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("gator", {
  platform: process.platform,
  snapshot: () => ipcRenderer.invoke("gator:snapshot"),
  send: (frame) => ipcRenderer.invoke("gator:send", frame),
  chooseFolder: () => ipcRenderer.invoke("gator:folder"),
  choosePath: (request) => ipcRenderer.invoke("gator:pick-path", request),
  openLink: (url) => ipcRenderer.invoke("gator:link", url),
  continueInTerminal: () => ipcRenderer.invoke("gator:terminal"),
  restart: () => ipcRenderer.invoke("gator:restart"),
  drafts: (request) => ipcRenderer.invoke("gator:drafts", request),
  subscribe(callback) {
    const listener = (_event, message) => callback(message);
    ipcRenderer.on("gator:update", listener);
    return () => ipcRenderer.removeListener("gator:update", listener);
  },
});
