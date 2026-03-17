const { app, BrowserWindow, ipcMain } = require("electron");
const path = require("path");
const os = require("os");
const fs = require("fs");
const pty = require("node-pty");

const SHARED_DIR = path.join(os.tmpdir(), "compose-preview");
const STATE_FILE = path.join(SHARED_DIR, "state.json");
const IMAGE_FILE = path.join(SHARED_DIR, "current.png");

let mainWindow;
let ptyProcess;

function createWindow() {
  mainWindow = new BrowserWindow({
    width: 1400,
    height: 900,
    title: "Compose Preview",
    backgroundColor: "#1e1e1e",
    webPreferences: {
      preload: path.join(__dirname, "preload.js"),
      nodeIntegration: false,
      contextIsolation: true,
    },
  });

  mainWindow.loadFile("index.html");

  // Start the Go TUI in a PTY
  const projectPath = process.argv[2] || process.cwd();
  const goBinary = findGoBinary();

  ptyProcess = pty.spawn(goBinary, [projectPath], {
    name: "xterm-256color",
    cols: 120,
    rows: 40,
    cwd: projectPath,
    env: { ...process.env, TERM: "xterm-256color", COMPOSE_PREVIEW_ELECTRON: "1" },
  });

  ptyProcess.onData((data) => {
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.webContents.send("terminal-data", data);
    }
  });

  ptyProcess.onExit(() => {
    if (mainWindow && !mainWindow.isDestroyed()) {
      mainWindow.close();
    }
  });

  // Watch for screenshot changes
  fs.mkdirSync(SHARED_DIR, { recursive: true });
  watchStateFile();

  // Handle terminal input from renderer
  ipcMain.on("terminal-input", (_, data) => {
    if (ptyProcess) ptyProcess.write(data);
  });

  ipcMain.on("terminal-resize", (_, { cols, rows }) => {
    if (ptyProcess) ptyProcess.resize(cols, rows);
  });

  mainWindow.on("closed", () => {
    if (ptyProcess) ptyProcess.kill();
    mainWindow = null;
  });
}

function findGoBinary() {
  const suffix = process.platform === "win32" ? ".exe" : "";
  const name = "compose-preview" + suffix;

  const candidates = [
    // Bundled in Electron app (electron-builder extraResources)
    path.join(process.resourcesPath || "", "bin", name),
    // Development: next to electron/ directory
    path.join(__dirname, "..", name),
    path.join(__dirname, "..", "dist", name),
  ];

  // Check GOPATH/bin
  const gopath = process.env.GOPATH || path.join(os.homedir(), "go");
  candidates.push(path.join(gopath, "bin", name));

  // Check PATH
  const pathDirs = (process.env.PATH || "").split(path.delimiter);
  for (const dir of pathDirs) {
    candidates.push(path.join(dir, name));
  }

  for (const c of candidates) {
    try {
      if (fs.existsSync(c)) return c;
    } catch {}
  }

  // Fallback: assume it's on PATH
  return name;
}

function watchStateFile() {
  let lastTimestamp = 0;

  // Poll every 200ms (fs.watch is unreliable on some OS)
  setInterval(() => {
    try {
      const raw = fs.readFileSync(STATE_FILE, "utf-8");
      const state = JSON.parse(raw);
      if (state.timestamp > lastTimestamp) {
        lastTimestamp = state.timestamp;

        // Read image if it exists
        let imageData = null;
        if (state.hasScreenshot && fs.existsSync(IMAGE_FILE)) {
          const buf = fs.readFileSync(IMAGE_FILE);
          imageData = `data:image/png;base64,${buf.toString("base64")}`;
        }

        if (mainWindow && !mainWindow.isDestroyed()) {
          mainWindow.webContents.send("preview-update", {
            ...state,
            imageData,
          });
        }
      }
    } catch {
      // state.json doesn't exist yet or is being written
    }
  }, 200);
}

app.whenReady().then(createWindow);

app.on("window-all-closed", () => {
  app.quit();
});
