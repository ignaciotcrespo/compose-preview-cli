package imgrender

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image/png"
	"strings"
)

// kittyProtocol renders images using the Kitty graphics protocol.
// Pixel-perfect quality. Supported by Kitty terminal.
//
// Protocol spec: https://sw.kovidgoyal.net/kitty/graphics-protocol/
// Sends image data in chunks via APC escape sequences.
type kittyProtocol struct{}

func (p *kittyProtocol) Name() string { return "Kitty graphics" }

func (p *kittyProtocol) Render(pngData []byte, width, height int) string {
	resized, err := resizeImage(pngData, width*8, height*16)
	if err != nil {
		return fmt.Sprintf("  (decode error: %v)", err)
	}

	var buf bytes.Buffer
	png.Encode(&buf, resized)
	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())

	// Kitty protocol sends data in chunks of 4096 bytes
	var b strings.Builder
	chunkSize := 4096
	for i := 0; i < len(encoded); i += chunkSize {
		end := i + chunkSize
		if end > len(encoded) {
			end = len(encoded)
		}
		chunk := encoded[i:end]

		more := 1
		if end == len(encoded) {
			more = 0
		}

		if i == 0 {
			// First chunk: include control parameters
			b.WriteString(fmt.Sprintf("\x1b_Ga=T,f=100,m=%d,c=%d,r=%d;%s\x1b\\",
				more, width, height, chunk))
		} else {
			// Continuation chunks
			b.WriteString(fmt.Sprintf("\x1b_Gm=%d;%s\x1b\\", more, chunk))
		}
	}
	b.WriteString("\n")
	return b.String()
}
