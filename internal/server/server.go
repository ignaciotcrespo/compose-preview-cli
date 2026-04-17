// Package server provides a localhost HTTP server that embeds the TUI
// in a browser via xterm.js + WebSocket, with a live preview image panel.
package server

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var sharedDir = filepath.Join(os.TempDir(), "compose-preview")

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// Server serves the full app (TUI + preview) in a browser.
type Server struct {
	port       int
	goBinary   string
	projectDir string
	ptmx       *os.File
	listener   net.Listener
	sseClients map[chan string]bool
	mu         sync.Mutex
	lastJSON   string
}

// New creates a server.
func New(port int, goBinary, projectDir string) *Server {
	return &Server{
		port:       port,
		goBinary:   goBinary,
		projectDir: projectDir,
		sseClients: make(map[chan string]bool),
	}
}

// URL returns the server URL.
func (s *Server) URL() string {
	return fmt.Sprintf("http://localhost:%d", s.port)
}

// Start starts the HTTP server and PTY process. Returns the actual port.
func (s *Server) Start() (int, error) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/ws", s.handleWebSocket)
	mux.HandleFunc("/events", s.handleSSE)
	mux.HandleFunc("/screenshot", s.handleScreenshot)

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%d", s.port))
	if err != nil {
		listener, err = net.Listen("tcp", "localhost:0")
		if err != nil {
			return 0, err
		}
	}
	s.port = listener.Addr().(*net.TCPAddr).Port
	s.listener = listener

	go http.Serve(listener, mux)
	go s.watchState()

	return s.port, nil
}

// Stop shuts down the server and kills the PTY process.
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
		s.listener = nil
	}
	if s.ptmx != nil {
		s.ptmx.Close()
		s.ptmx = nil
	}
}

// startPTY starts the Go TUI in a PTY. Called on first WebSocket connection.
func (s *Server) startPTY() (*os.File, error) {
	if s.ptmx != nil {
		return s.ptmx, nil
	}

	cmd := exec.Command(s.goBinary, s.projectDir)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COMPOSE_PREVIEW_ELECTRON=1")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}
	pty.Setsize(ptmx, &pty.Winsize{Rows: 40, Cols: 120})
	s.ptmx = ptmx

	// Clean up when process exits
	go func() {
		cmd.Wait()
		s.ptmx = nil
	}()

	return ptmx, nil
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()

	ptmx, err := s.startPTY()
	if err != nil {
		conn.WriteMessage(websocket.TextMessage, []byte(fmt.Sprintf("Error: %v\r\n", err)))
		return
	}

	// PTY → WebSocket
	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := ptmx.Read(buf)
			if err != nil {
				conn.WriteMessage(websocket.CloseMessage, nil)
				return
			}
			conn.WriteMessage(websocket.BinaryMessage, buf[:n])
		}
	}()

	// WebSocket → PTY
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}

		// Check for resize message: \x01{"cols":N,"rows":N}
		if len(msg) > 1 && msg[0] == 1 {
			var size struct {
				Cols uint16 `json:"cols"`
				Rows uint16 `json:"rows"`
			}
			if json.Unmarshal(msg[1:], &size) == nil && size.Cols > 0 && size.Rows > 0 {
				pty.Setsize(ptmx, &pty.Winsize{Rows: size.Rows, Cols: size.Cols})
				continue
			}
		}

		ptmx.Write(msg)
	}
}

// SSE for screenshot updates

func (s *Server) watchState() {
	stateFile := filepath.Join(sharedDir, "state.json")
	var lastMod time.Time
	for {
		time.Sleep(200 * time.Millisecond)
		info, err := os.Stat(stateFile)
		if err != nil {
			continue
		}
		if info.ModTime().After(lastMod) {
			lastMod = info.ModTime()
			data, _ := os.ReadFile(stateFile)
			s.notifySSE(string(data))
		}
	}
}

func (s *Server) notifySSE(jsonData string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastJSON = jsonData
	for ch := range s.sseClients {
		select {
		case ch <- jsonData:
		default:
		}
	}
}

func (s *Server) handleSSE(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := make(chan string, 10)
	s.mu.Lock()
	s.sseClients[ch] = true
	last := s.lastJSON
	s.mu.Unlock()

	if last != "" {
		fmt.Fprintf(w, "data: %s\n\n", last)
		flusher.Flush()
	}

	defer func() {
		s.mu.Lock()
		delete(s.sseClients, ch)
		s.mu.Unlock()
	}()

	for {
		select {
		case msg := <-ch:
			fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleScreenshot(w http.ResponseWriter, r *http.Request) {
	data, err := os.ReadFile(filepath.Join(sharedDir, "current.png"))
	if err != nil {
		http.Error(w, "No screenshot", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "no-cache")
	w.Write(data)
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	io.WriteString(w, indexHTML)
}

var indexHTML = `<!DOCTYPE html>
<html>
<head>
  <meta charset="UTF-8">
  <title>Compose Preview</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/css/xterm.min.css">
  <style>
    * { margin: 0; padding: 0; box-sizing: border-box; }
    body {
      display: flex;
      height: 100vh;
      background: #1e1e1e;
      color: #ccc;
      font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
      overflow: hidden;
    }

    #terminal-pane {
      flex: 1;
      min-width: 0;
      display: flex;
      flex-direction: column;
      border-right: 1px solid #333;
    }
    #terminal-container { flex: 1; padding: 4px; }

    #divider {
      width: 5px;
      cursor: col-resize;
      background: #333;
      transition: background 0.15s;
    }
    #divider:hover { background: #555; }

    #preview-pane {
      width: 380px;
      display: flex;
      flex-direction: column;
      background: #111;
    }
    #preview-header {
      padding: 10px 14px;
      font-size: 12px;
      border-bottom: 1px solid #333;
      display: flex;
      align-items: center;
      gap: 10px;
    }
    .fqn {
      color: #5aafff;
      font-family: "JetBrains Mono", "Fira Code", monospace;
      font-size: 11px;
      flex: 1;
      overflow: hidden;
      text-overflow: ellipsis;
      white-space: nowrap;
    }
    .status { color: #888; font-size: 11px; }
    .status.capturing {
      color: #e8a838;
      animation: pulse 1.2s ease-in-out infinite;
    }
    .time { color: #5aafff; }
    @keyframes pulse {
      0%, 100% { opacity: 1; }
      50% { opacity: 0.4; }
    }

    #preview-image-container {
      flex: 1;
      display: flex;
      align-items: center;
      justify-content: center;
      padding: 12px;
      overflow: hidden;
    }
    #preview-image {
      max-width: 100%;
      max-height: 100%;
      object-fit: contain;
      border-radius: 6px;
      box-shadow: 0 4px 24px rgba(0,0,0,0.5);
    }
    #placeholder {
      color: #555;
      font-size: 15px;
      text-align: center;
      line-height: 1.8;
    }
    #placeholder strong { color: #888; }
  </style>
</head>
<body>
  <div id="terminal-pane">
    <div id="terminal-container"></div>
  </div>
  <div id="divider"></div>
  <div id="preview-pane">
    <div id="preview-header">
      <span class="fqn" id="preview-fqn">No preview selected</span>
      <span class="status" id="preview-status"></span>
    </div>
    <div id="preview-image-container">
      <div id="placeholder">
        Select a preview and press<br>
        <strong>Enter</strong> to run &amp; capture<br>
        or <strong>s</strong> to screenshot
      </div>
      <img id="preview-image" style="display:none">
    </div>
  </div>

  <script src="https://cdn.jsdelivr.net/npm/@xterm/xterm@5.5.0/lib/xterm.min.js"></script>
  <script src="https://cdn.jsdelivr.net/npm/@xterm/addon-fit@0.10.0/lib/addon-fit.min.js"></script>
  <script>
    // --- Terminal ---
    const term = new Terminal({
      fontFamily: '"JetBrains Mono", "Fira Code", "SF Mono", Menlo, monospace',
      fontSize: 13,
      theme: { background: '#1e1e1e', foreground: '#cccccc', cursor: '#ffffff', selectionBackground: '#264f78' },
      cursorBlink: true,
    });
    const fitAddon = new FitAddon.FitAddon();
    term.loadAddon(fitAddon);
    term.open(document.getElementById('terminal-container'));
    fitAddon.fit();

    const proto = location.protocol === 'https:' ? 'wss:' : 'ws:';
    const ws = new WebSocket(proto + '//' + location.host + '/ws');
    ws.binaryType = 'arraybuffer';

    ws.onmessage = (e) => {
      if (e.data instanceof ArrayBuffer) {
        term.write(new Uint8Array(e.data));
      } else {
        term.write(e.data);
      }
    };

    term.onData((data) => ws.send(data));

    function sendResize() {
      fitAddon.fit();
      const msg = new Uint8Array([1, ...new TextEncoder().encode(JSON.stringify({ cols: term.cols, rows: term.rows }))]);
      if (ws.readyState === WebSocket.OPEN) ws.send(msg);
    }

    const ro = new ResizeObserver(() => sendResize());
    ro.observe(document.getElementById('terminal-container'));
    ws.onopen = () => sendResize();

    // --- Preview ---
    const img = document.getElementById('preview-image');
    const placeholder = document.getElementById('placeholder');
    const fqnEl = document.getElementById('preview-fqn');
    const statusEl = document.getElementById('preview-status');

    const es = new EventSource('/events');
    es.onmessage = (e) => {
      try {
        const data = JSON.parse(e.data);
        const parts = (data.fqn || '').split('.');
        fqnEl.textContent = parts.slice(-2).join('.');
        fqnEl.title = data.fqn;

        if (data.capturing) {
          statusEl.textContent = 'capturing...';
          statusEl.className = 'status capturing';
        } else if (data.hasScreenshot) {
          img.src = '/screenshot?t=' + data.timestamp;
          img.style.display = 'block';
          placeholder.style.display = 'none';
          let s = '';
          if (data.capturedAt) s += '<span class="time">' + data.capturedAt + '</span> ';
          if (data.age) s += '(' + data.age + ')';
          statusEl.innerHTML = s;
          statusEl.className = 'status';
        } else {
          img.style.display = 'none';
          placeholder.style.display = 'block';
          placeholder.innerHTML = 'No screenshot cached<br>Press <strong>Enter</strong> to capture';
          statusEl.innerHTML = '';
          statusEl.className = 'status';
        }
      } catch {}
    };

    // --- Resizable divider ---
    const divider = document.getElementById('divider');
    const previewPane = document.getElementById('preview-pane');
    let dragging = false;
    divider.addEventListener('mousedown', () => dragging = true);
    document.addEventListener('mousemove', (e) => {
      if (!dragging) return;
      const w = document.body.clientWidth - e.clientX;
      previewPane.style.width = Math.max(200, Math.min(800, w)) + 'px';
      sendResize();
    });
    document.addEventListener('mouseup', () => dragging = false);

    // Focus terminal
    document.getElementById('terminal-pane').addEventListener('click', () => term.focus());
    term.focus();
  </script>
</body>
</html>
`
