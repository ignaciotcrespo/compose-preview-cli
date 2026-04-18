package ui

import (
	"syscall"
	"unsafe"
)

// termState holds the terminal state for save/restore.
type termState = syscall.Termios

func makeRaw(fd uintptr) (termState, error) {
	var oldState termState
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, fd,
		uintptr(syscall.TCGETS), uintptr(unsafe.Pointer(&oldState)), 0, 0, 0); err != 0 {
		return oldState, err
	}
	newState := oldState
	newState.Lflag &^= syscall.ICANON | syscall.ECHO
	newState.Cc[syscall.VMIN] = 1
	newState.Cc[syscall.VTIME] = 0
	if _, _, err := syscall.Syscall6(syscall.SYS_IOCTL, fd,
		uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&newState)), 0, 0, 0); err != 0 {
		return oldState, err
	}
	return oldState, nil
}

func restoreTerminal(fd uintptr, state termState) {
	syscall.Syscall6(syscall.SYS_IOCTL, fd,
		uintptr(syscall.TCSETS), uintptr(unsafe.Pointer(&state)), 0, 0, 0)
}
