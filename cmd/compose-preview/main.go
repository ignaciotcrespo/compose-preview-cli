package main

import (
	"fmt"
	"os"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/ignaciotcrespo/compose-preview-cli/internal/gradle"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/scanner"
	"github.com/ignaciotcrespo/compose-preview-cli/internal/ui"
)

func main() {
	// Determine project root
	dir := "."
	if len(os.Args) > 1 {
		dir = os.Args[1]
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
	p := tea.NewProgram(model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
