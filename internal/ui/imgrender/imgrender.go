// Package imgrender renders images in terminal cells.
//
// Architecture: a Protocol interface with auto-detection.
// To add a new terminal, implement Protocol and register it in detect().
package imgrender

import (
	"bytes"
	"image"
	_ "image/png"
	"io"

	"golang.org/x/image/draw"
)

// Protocol renders PNG image data into a string that a terminal can display
// within the given character cell dimensions (width × height).
type Protocol interface {
	Name() string
	Render(pngData []byte, width, height int) string
}

// active is the detected protocol, set once at init.
var active Protocol

func init() {
	active = detect()
}

// Render converts PNG data to a displayable string using the best available protocol.
func Render(pngData []byte, width, height int) string {
	return active.Render(pngData, width, height)
}

// ProtocolName returns the name of the active rendering protocol.
func ProtocolName() string {
	return active.Name()
}

// IsGraphicsProtocol returns true if the active protocol uses terminal graphics
// escape sequences (Kitty, iTerm2) that would be corrupted by lipgloss processing.
// When true, callers should place the rendered output directly into the view
// without passing it through lipgloss styling functions.
func IsGraphicsProtocol() bool {
	switch active.(type) {
	case *kittyProtocol, *iterm2Protocol:
		return true
	default:
		return false
	}
}

// TerminalSize returns the terminal width and height from the given writer,
// falling back to 80x24 if detection fails.
// Implementation is in termsize_unix.go / termsize_windows.go.
func TerminalSize(w io.Writer) (int, int) {
	return terminalSize(w)
}

// resizeImage decodes PNG data and scales it to fit within the given pixel dimensions,
// preserving aspect ratio. Shared helper for all protocols.
func resizeImage(pngData []byte, targetW, targetH int) (*image.RGBA, error) {
	img, _, err := image.Decode(bytes.NewReader(pngData))
	if err != nil {
		return nil, err
	}

	bounds := img.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()

	scaleW := float64(targetW) / float64(srcW)
	scaleH := float64(targetH) / float64(srcH)
	scale := scaleW
	if scaleH < scale {
		scale = scaleH
	}

	newW := int(float64(srcW) * scale)
	newH := int(float64(srcH) * scale)
	if newW < 1 {
		newW = 1
	}
	if newH < 1 {
		newH = 1
	}

	resized := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.CatmullRom.Scale(resized, resized.Bounds(), img, img.Bounds(), draw.Over, nil)
	return resized, nil
}
