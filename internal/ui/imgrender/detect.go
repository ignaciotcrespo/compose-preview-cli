package imgrender

import (
	"os"
	"strings"
)

// detect returns the best available image rendering protocol for the current terminal.
// To add support for a new terminal:
//  1. Create a new file (e.g., protocol_foo.go) implementing Protocol
//  2. Add detection logic here
//
// NOTE: Inside Bubbletea's alt-screen mode, graphics protocols (Kitty, iTerm2)
// break the panel layout. The TUI always uses half-block rendering.
// Use DetectGraphics() for standalone/non-TUI contexts (e.g., --screenshot).
func detect() Protocol {
	return &halfBlockProtocol{}
}

// DetectGraphics returns the best graphics protocol for the current terminal,
// for use outside the TUI (e.g., printing an image to stdout).
// Falls back to half-block if no graphics protocol is detected.
func DetectGraphics() Protocol {
	term := os.Getenv("TERM_PROGRAM")
	termInfo := os.Getenv("TERM")

	// Kitty detection: TERM=xterm-kitty or TERM_PROGRAM=kitty
	if term == "kitty" || strings.Contains(termInfo, "kitty") {
		return &kittyProtocol{}
	}

	// iTerm2 detection (also covers WezTerm which sets LC_TERMINAL=iTerm2)
	lcTerminal := os.Getenv("LC_TERMINAL")
	if term == "iTerm.app" || lcTerminal == "iTerm2" {
		return &iterm2Protocol{}
	}

	// WezTerm also supports iTerm2 inline images
	if term == "WezTerm" {
		return &iterm2Protocol{}
	}

	// Ghostty supports Kitty graphics protocol
	if term == "ghostty" {
		return &kittyProtocol{}
	}

	return &halfBlockProtocol{}
}
