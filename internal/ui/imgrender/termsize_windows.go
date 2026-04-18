package imgrender

import "io"

func terminalSize(w io.Writer) (int, int) {
	return 80, 24
}
