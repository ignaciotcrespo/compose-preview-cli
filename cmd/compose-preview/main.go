package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/gradle"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/scanner"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/server"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui"
)

// version is set by goreleaser via ldflags.
var version = "dev"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("compose-preview", version)
		return
	}

	// Check for flags
	webMode := false
	listMode := false
	webPort := 9999
	args := []string{}
	for i := 1; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "--web" || arg == "-w" {
			webMode = true
		} else if arg == "--list" || arg == "-l" {
			listMode = true
		} else if (arg == "--port" || arg == "-p") && i+1 < len(os.Args) {
			i++
			p, err := strconv.Atoi(os.Args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid port %q\n", os.Args[i])
				os.Exit(1)
			}
			webPort = p
		} else {
			args = append(args, arg)
		}
	}

	// Determine project root
	dir := "."
	if len(args) > 0 {
		dir = args[0]
	}

	absDir, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	root, err := gradle.FindProjectRoot(absDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		fmt.Fprintf(os.Stderr, "Run this command from an Android/Gradle project directory, or pass the path as an argument.\n")
		os.Exit(1)
	}

	if webMode {
		runWebMode(root, webPort)
		return
	}

	if listMode {
		runListMode(root)
		return
	}

	// Scan for previews
	fmt.Fprintf(os.Stderr, "Scanning %s for @Preview composables...\n", root)
	result := scanner.Scan(root)
	fmt.Fprintf(os.Stderr, "Found %d previews across %d modules\n", len(result.AllPreviews), len(result.Modules))

	if len(result.AllPreviews) == 0 {
		fmt.Fprintf(os.Stderr, "No @Preview composables found.\n")
		os.Exit(0)
	}

	// Launch TUI
	model := ui.NewModel(result, root)
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion(), tea.WithReportFocus())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runWebMode(root string, port int) {
	// In web mode, start the server and open the browser.
	// The server spawns the TUI in a PTY internally.
	goBinary, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	srv := server.New(port, goBinary, root)
	actualPort, err := srv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}

	url := fmt.Sprintf("http://localhost:%d", actualPort)
	fmt.Fprintf(os.Stderr, "Compose Preview running at %s\n", url)

	// Open browser
	switch runtime.GOOS {
	case "darwin":
		exec.Command("open", url).Start()
	case "windows":
		exec.Command("cmd", "/c", "start", url).Start()
	default:
		exec.Command("xdg-open", url).Start()
	}

	// Keep running until Ctrl+C
	fmt.Fprintf(os.Stderr, "Press Ctrl+C to stop\n")
	select {}
}

func runListMode(root string) {
	result := scanner.Scan(root)

	type previewEntry struct {
		Module   string `json:"module"`
		Function string `json:"function"`
		FQN      string `json:"fqn"`
		File     string `json:"file"`
		Line     int    `json:"line"`
		Name     string `json:"name,omitempty"`
	}

	var entries []previewEntry
	for _, p := range result.AllPreviews {
		rel, _ := filepath.Rel(root, p.FilePath)
		entries = append(entries, previewEntry{
			Module:   p.Module,
			Function: p.FunctionName,
			FQN:      p.FQN,
			File:     rel,
			Line:     p.LineNumber,
			Name:     p.PreviewName,
		})
	}

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(entries)
}
