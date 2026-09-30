package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/png"
	"strings"
	"sync/atomic"
	"time"

	"github.com/disintegration/imaging"
)

type imageProtocol string

const (
	imageProtocolAuto    imageProtocol = "auto"
	imageProtocolKitty   imageProtocol = "kitty"
	imageProtocolWezTerm imageProtocol = "wezterm"
	imageProtocolANSI    imageProtocol = "ansi"
	kittyChunkSize                     = 4096
	kittyPlaceholder                   = '\U0010EEEE'
)

type nativeImage struct {
	id           uint32
	path         string
	width        int
	height       int
	protocol     imageProtocol
	transmission string
	placement    string
	placeholders string
}

var (
	kittyImageCounter = uint32(time.Now().UnixNano()) & 0x00ffffff
	kittyPrepareSlot  = make(chan struct{}, 1)
)

func parseImageProtocol(value string) (imageProtocol, error) {
	switch protocol := imageProtocol(strings.ToLower(value)); protocol {
	case imageProtocolAuto, imageProtocolKitty, imageProtocolWezTerm, imageProtocolANSI:
		return protocol, nil
	default:
		return "", fmt.Errorf("invalid image protocol %q: use auto, kitty, wezterm, or ansi", value)
	}
}

func resolveImageProtocol(requested imageProtocol, getenv func(string) string) imageProtocol {
	if requested == imageProtocolWezTerm && getenv("TMUX") != "" {
		return imageProtocolANSI
	}
	if requested != imageProtocolAuto {
		return requested
	}
	if getenv("TMUX") != "" {
		return imageProtocolANSI
	}
	term := strings.ToLower(getenv("TERM"))
	termProgram := strings.ToLower(getenv("TERM_PROGRAM"))
	if termProgram == "wezterm" {
		return imageProtocolWezTerm
	}
	if getenv("KITTY_WINDOW_ID") != "" || getenv("GHOSTTY_RESOURCES_DIR") != "" ||
		strings.Contains(term, "kitty") || strings.Contains(term, "ghostty") || termProgram == "ghostty" {
		return imageProtocolKitty
	}
	return imageProtocolANSI
}

func prepareNativeImage(ctx context.Context, protocol imageProtocol, path string, width, height, x, y int, tmux bool) (nativeImage, error) {
	switch protocol {
	case imageProtocolKitty:
		return prepareKittyImageContext(ctx, path, width, height, tmux)
	case imageProtocolWezTerm:
		return prepareWezTermImageContext(ctx, path, width, height, x, y)
	default:
		return nativeImage{}, fmt.Errorf("native image protocol %q is not supported", protocol)
	}
}

func prepareKittyImage(path string, width, height int, tmux bool) (nativeImage, error) {
	return prepareKittyImageContext(context.Background(), path, width, height, tmux)
}

func prepareKittyImageContext(ctx context.Context, path string, width, height int, tmux bool) (nativeImage, error) {
	if path == "" {
		return nativeImage{}, fmt.Errorf("preview path is empty")
	}
	if width <= 0 || height <= 0 || height > len(kittyDiacritics) {
		return nativeImage{}, fmt.Errorf("unsupported Kitty image area %dx%d", width, height)
	}
	select {
	case kittyPrepareSlot <- struct{}{}:
		defer func() { <-kittyPrepareSlot }()
	case <-ctx.Done():
		return nativeImage{}, ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return nativeImage{}, err
	}
	image, err := imaging.Open(path)
	if err != nil {
		return nativeImage{}, err
	}
	if err := ctx.Err(); err != nil {
		return nativeImage{}, err
	}
	image = fitNativePreview(image, width, height)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image); err != nil {
		return nativeImage{}, fmt.Errorf("encoding Kitty preview: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nativeImage{}, err
	}
	id := nextKittyImageID()
	encoded := base64.StdEncoding.EncodeToString(pngData.Bytes())
	var transmission strings.Builder
	for offset := 0; offset < len(encoded); offset += kittyChunkSize {
		end := min(offset+kittyChunkSize, len(encoded))
		more := 0
		if end < len(encoded) {
			more = 1
		}
		var sequence string
		if offset == 0 {
			sequence = fmt.Sprintf("\x1b_Ga=t,f=100,t=d,i=%d,q=2,m=%d;%s\x1b\\", id, more, encoded[offset:end])
		} else {
			sequence = fmt.Sprintf("\x1b_Gm=%d,q=2;%s\x1b\\", more, encoded[offset:end])
		}
		transmission.WriteString(wrapKittySequence(sequence, tmux))
	}
	placement := fmt.Sprintf("\x1b_Ga=p,U=1,i=%d,c=%d,r=%d,q=2\x1b\\", id, width, height)
	transmission.WriteString(wrapKittySequence(placement, tmux))

	return nativeImage{
		id:           id,
		path:         path,
		width:        width,
		height:       height,
		protocol:     imageProtocolKitty,
		transmission: transmission.String(),
		placeholders: kittyPlaceholderGrid(id, width, height),
	}, nil
}

func prepareWezTermImageContext(ctx context.Context, path string, width, height, x, y int) (nativeImage, error) {
	if path == "" {
		return nativeImage{}, fmt.Errorf("preview path is empty")
	}
	if width <= 0 || height <= 0 || x < 0 || y < 0 {
		return nativeImage{}, fmt.Errorf("unsupported WezTerm image area %dx%d at %d,%d", width, height, x, y)
	}
	select {
	case kittyPrepareSlot <- struct{}{}:
		defer func() { <-kittyPrepareSlot }()
	case <-ctx.Done():
		return nativeImage{}, ctx.Err()
	}
	image, err := imaging.Open(path)
	if err != nil {
		return nativeImage{}, err
	}
	image = fitNativePreview(image, width, height)
	var pngData bytes.Buffer
	if err := png.Encode(&pngData, image); err != nil {
		return nativeImage{}, fmt.Errorf("encoding WezTerm preview: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return nativeImage{}, err
	}
	encoded := base64.StdEncoding.EncodeToString(pngData.Bytes())
	id := nextKittyImageID()
	var transmission strings.Builder
	for offset := 0; offset < len(encoded); offset += kittyChunkSize {
		end := min(offset+kittyChunkSize, len(encoded))
		more := 0
		if end < len(encoded) {
			more = 1
		}
		if offset == 0 {
			fmt.Fprintf(&transmission, "\x1b_Ga=t,f=100,t=d,i=%d,q=2,m=%d;%s\x1b\\", id, more, encoded[offset:end])
		} else {
			fmt.Fprintf(&transmission, "\x1b_Gm=%d,q=2;%s\x1b\\", more, encoded[offset:end])
		}
	}
	placement := fmt.Sprintf("\x1b7\x1b[%d;%dH\x1b_Ga=p,i=%d,c=%d,r=%d,C=1,q=2\x1b\\\x1b8", y+1, x+1, id, width, height)
	return nativeImage{
		id:           id,
		path:         path,
		width:        width,
		height:       height,
		protocol:     imageProtocolWezTerm,
		transmission: transmission.String(),
		placement:    placement,
		placeholders: blankImageGrid(width, height),
	}, nil
}

// Place the fitted image on a transparent cell-sized canvas so c/r placement
// does not stretch portrait and panoramic APODs back to the pane dimensions.
func fitNativePreview(source image.Image, width, height int) image.Image {
	canvas := image.NewNRGBA(image.Rect(0, 0, width*8, height*16))
	return imaging.PasteCenter(canvas, imaging.Fit(source, width*8, height*16, imaging.Lanczos))
}

func blankImageGrid(width, height int) string {
	row := strings.Repeat(" ", width)
	rows := make([]string, height)
	for i := range rows {
		rows[i] = row
	}
	return strings.Join(rows, "\n")
}

func nextKittyImageID() uint32 {
	id := atomic.AddUint32(&kittyImageCounter, 1) & 0x00ffffff
	if id == 0 {
		id = atomic.AddUint32(&kittyImageCounter, 1) & 0x00ffffff
	}
	return id
}

func kittyPlaceholderGrid(id uint32, width, height int) string {
	red := (id >> 16) & 0xff
	green := (id >> 8) & 0xff
	blue := id & 0xff
	color := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", red, green, blue)
	var output strings.Builder
	for row := range height {
		output.WriteString(color)
		output.WriteRune(kittyPlaceholder)
		output.WriteRune(kittyDiacritics[row])
		output.WriteRune(kittyDiacritics[0])
		for range width - 1 {
			output.WriteRune(kittyPlaceholder)
		}
		output.WriteString("\x1b[39m")
		if row < height-1 {
			output.WriteByte('\n')
		}
	}
	return output.String()
}

func kittyDeleteImage(id uint32, tmux bool) string {
	if id == 0 {
		return ""
	}
	return wrapKittySequence(fmt.Sprintf("\x1b_Ga=d,d=I,i=%d,q=2\x1b\\", id), tmux)
}

func wrapKittySequence(sequence string, tmux bool) string {
	if !tmux {
		return sequence
	}
	return "\x1bPtmux;" + strings.ReplaceAll(sequence, "\x1b", "\x1b\x1b") + "\x1b\\"
}

// First entries from Kitty's frozen row/column diacritic table. TUI image
// heights are bounded by the terminal and fall back to ANSI beyond this list.
var kittyDiacritics = []rune{
	0x0305, 0x030D, 0x030E, 0x0310, 0x0312, 0x033D, 0x033E, 0x033F,
	0x0346, 0x034A, 0x034B, 0x034C, 0x0350, 0x0351, 0x0352, 0x0357,
	0x035B, 0x0363, 0x0364, 0x0365, 0x0366, 0x0367, 0x0368, 0x0369,
	0x036A, 0x036B, 0x036C, 0x036D, 0x036E, 0x036F, 0x0483, 0x0484,
	0x0485, 0x0486, 0x0487, 0x0592, 0x0593, 0x0594, 0x0595, 0x0597,
	0x0598, 0x0599, 0x059C, 0x059D, 0x059E, 0x059F, 0x05A0, 0x05A1,
	0x05A8, 0x05A9, 0x05AB, 0x05AC, 0x05AF, 0x05C4, 0x0610, 0x0611,
	0x0612, 0x0613, 0x0614, 0x0615, 0x0616, 0x0617, 0x0657, 0x0658,
	0x0659, 0x065A, 0x065B, 0x065D, 0x065E, 0x06D6, 0x06D7, 0x06D8,
	0x06D9, 0x06DA, 0x06DB, 0x06DC, 0x06DF, 0x06E0, 0x06E1, 0x06E2,
	0x06E4, 0x06E7, 0x06E8, 0x06EB, 0x06EC, 0x0730, 0x0732, 0x0733,
	0x0735, 0x0736, 0x073A, 0x073D, 0x073F, 0x0740, 0x0741, 0x0743,
	0x0745, 0x0747, 0x0749, 0x074A, 0x07EB, 0x07EC, 0x07ED, 0x07EE,
	0x07EF, 0x07F0, 0x07F1, 0x07F3, 0x0816, 0x0817, 0x0818, 0x0819,
	0x081B, 0x081C, 0x081D, 0x081E, 0x081F, 0x0820, 0x0821, 0x0822,
	0x0823, 0x0825, 0x0826, 0x0827, 0x0829, 0x082A, 0x082B, 0x082C,
	0x082D, 0x0951, 0x0953, 0x0954, 0x0F82, 0x0F83, 0x0F86, 0x0F87,
}
