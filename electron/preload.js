const { contextBridge, ipcRenderer } = require("electron");

contextBridge.exposeInMainWorld("api", {
  onTerminalData: (callback) => ipcRenderer.on("terminal-data", (_, data) => callback(data)),
  sendTerminalInput: (data) => ipcRenderer.send("terminal-input", data),
  sendTerminalResize: (cols, rows) => ipcRenderer.send("terminal-resize", { cols, rows }),
  onPreviewUpdate: (callback) => ipcRenderer.on("preview-update", (_, data) => callback(data)),
});
