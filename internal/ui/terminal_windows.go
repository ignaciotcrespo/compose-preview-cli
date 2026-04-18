package ui

import "fmt"

// termState holds the terminal state for save/restore.
type termState struct{}

func makeRaw(fd uintptr) (termState, error) {
	return termState{}, fmt.Errorf("raw mode not supported on windows")
}

func restoreTerminal(fd uintptr, state termState) {}
