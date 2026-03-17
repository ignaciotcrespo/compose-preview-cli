package imgrender

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
)

// iterm2Protocol renders images using the iTerm2 inline image protocol.
// Pixel-perfect quality. Supported by iTerm2, WezTerm, mintty, Hyper.
//
// Protocol spec: https://iterm2.com/documentation-images.html
// Format: \033]1337;File=inline=1;width=<W>c;height=<H>c;preserveAspectRatio=1:<base64>\007
type iterm2Protocol struct{}

func (p *iterm2Protocol) Name() string { return "iTerm2 inline" }

func (p *iterm2Protocol) Render(pngData []byte, width, height int) string {
	// Resize to fit panel pixel dimensions
	// Cell is roughly 8px wide × 16px tall
	resized, err := resizeImage(pngData, width*8, height*16)
	if err != nil {
		return fmt.Sprintf("  (decode error: %v)", err)
	}

	// Re-encode as PNG
	var buf bytes.Buffer
	png.Encode(&buf, resized)

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	return fmt.Sprintf("\x1b]1337;File=inline=1;width=%dc;height=%dc;preserveAspectRatio=1:%s\x07\n",
		width, height, encoded)
}
