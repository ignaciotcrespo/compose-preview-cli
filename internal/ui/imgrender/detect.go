package imgrender

import "os"

// detect returns the best available image rendering protocol for the current terminal.
// To add support for a new terminal:
//  1. Create a new file (e.g., protocol_foo.go) implementing Protocol
//  2. Add detection logic here
func detect() Protocol {
	// NOTE: iTerm2 and Kitty inline image protocols don't work inside
	// Bubbletea alt-screen mode — the escape sequences break the panel layout.
	// Half-block is the only reliable option inside a TUI framework.
	// The protocol files are kept for future use (e.g., standalone image viewer).
	_ = os.Getenv("TERM_PROGRAM") // reserved for future protocol detection
	return &halfBlockProtocol{}
}
