package imgrender

import (
	"fmt"
	"image/color"
	"strings"
)

// halfBlockProtocol renders images using Unicode half-block characters (▀)
// with 24-bit true-color ANSI escape sequences.
// Works in any terminal with true-color support (most modern terminals).
// Each character cell represents 2 vertical pixels.
type halfBlockProtocol struct{}

func (p *halfBlockProtocol) Name() string { return "half-block" }

func (p *halfBlockProtocol) Render(pngData []byte, width, height int) string {
	// Each cell = 2 vertical pixels
	resized, err := resizeImage(pngData, width, height*2)
	if err != nil {
		return fmt.Sprintf("  (decode error: %v)", err)
	}

	bounds := resized.Bounds()
	newW := bounds.Dx()
	newH := bounds.Dy()

	padLeft := (width - newW) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	padding := strings.Repeat(" ", padLeft)

	var b strings.Builder
	for y := 0; y < newH-1; y += 2 {
		b.WriteString(padding)
		for x := 0; x < newW; x++ {
			tr, tg, tb := rgb(resized.At(x, y))
			br, bg, bb := rgb(resized.At(x, y+1))
			b.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm\x1b[48;2;%d;%d;%dm▀\x1b[0m", tr, tg, tb, br, bg, bb))
		}
		b.WriteString("\n")
	}
	if newH%2 != 0 {
		b.WriteString(padding)
		y := newH - 1
		for x := 0; x < newW; x++ {
			tr, tg, tb := rgb(resized.At(x, y))
			b.WriteString(fmt.Sprintf("\x1b[38;2;%d;%d;%dm▀\x1b[0m", tr, tg, tb))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func rgb(c color.Color) (uint8, uint8, uint8) {
	r, g, b, _ := c.RGBA()
	return uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)
}
