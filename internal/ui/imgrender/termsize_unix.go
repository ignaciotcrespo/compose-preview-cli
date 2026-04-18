//go:build !windows

package imgrender

import (
	"io"
	"os"
	"syscall"
	"unsafe"
)

func terminalSize(w io.Writer) (int, int) {
	fd := uintptr(syscall.Stdout)
	if f, ok := w.(*os.File); ok {
		fd = f.Fd()
	}
	type winsize struct {
		Row, Col, Xpixel, Ypixel uint16
	}
	var ws winsize
	_, _, _ = syscall.Syscall(syscall.SYS_IOCTL, fd,
		uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws)))
	width, height := int(ws.Col), int(ws.Row)
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 24
	}
	return width, height
}
