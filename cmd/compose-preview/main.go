package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"

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

	// Check for --web flag
	webMode := false
	args := []string{}
	for _, arg := range os.Args[1:] {
		if arg == "--web" || arg == "-w" {
			webMode = true
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
		runWebMode(root)
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

func runWebMode(root string) {
	// In web mode, start the server and open the browser.
	// The server spawns the TUI in a PTY internally.
	goBinary, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	srv := server.New(9999, goBinary, root)
	port, err := srv.Start()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}

	url := fmt.Sprintf("http://localhost:%d", port)
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
