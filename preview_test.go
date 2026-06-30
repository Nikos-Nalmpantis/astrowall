package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPreviewBlockRendersANSIOutput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preview.png")
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{R: uint8(20 * x), G: uint8(30 * y), B: 120, A: 255})
		}
	}

	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	defer file.Close()
	if err := png.Encode(file, img); err != nil {
		t.Fatalf("png.Encode() error: %v", err)
	}

	rendered, err := renderPreviewBlock(path, 4, 2)
	if err != nil {
		t.Fatalf("renderPreviewBlock() error: %v", err)
	}
	if rendered == "" {
		t.Fatal("rendered preview is empty")
	}
	if !strings.Contains(rendered, "▀") {
		t.Fatalf("rendered preview = %q, want half-block characters", rendered)
	}
	if !strings.Contains(rendered, "\x1b[") {
		t.Fatalf("rendered preview = %q, want ANSI colorized output", rendered)
	}
}

func TestRenderPreviewBlockRejectsMissingPath(t *testing.T) {
	if _, err := renderPreviewBlock("", 10, 5); err == nil {
		t.Fatal("renderPreviewBlock() error = nil, want error for empty path")
	}
}
