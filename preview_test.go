package main

import (
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
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

func TestRenderPreviewBlockPreservesPortraitAndPanoramaProportions(t *testing.T) {
	for _, tc := range []struct {
		name, placement string
		width, height   int
	}{
		{"portrait", "horizontal", 8, 32},
		{"panorama", "vertical", 32, 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "preview.png")
			img := image.NewRGBA(image.Rect(0, 0, tc.width, tc.height))
			for y := range tc.height {
				for x := range tc.width {
					img.Set(x, y, color.RGBA{R: 180, G: 70, B: 130, A: 255})
				}
			}
			file, err := os.Create(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := png.Encode(file, img); err != nil {
				t.Fatal(err)
			}
			if err := file.Close(); err != nil {
				t.Fatal(err)
			}
			block, err := renderPreviewBlock(path, 16, 8)
			if err != nil {
				t.Fatal(err)
			}
			if lipgloss.Width(block) != 16 || lipgloss.Height(block) != 8 {
				t.Fatalf("block dimensions = %dx%d", lipgloss.Width(block), lipgloss.Height(block))
			}
			lines := strings.Split(block, "\n")
			if tc.placement == "horizontal" {
				if !strings.HasPrefix(ansi.Strip(lines[3]), "   ") || strings.Contains(lines[3], strings.Repeat("▀", 16)) {
					t.Fatalf("portrait should be centered horizontally: %q", ansi.Strip(lines[3]))
				}
			} else if strings.Contains(lines[0], "▀") || !strings.Contains(lines[3], "▀") {
				t.Fatalf("panorama should be centered vertically: %q", block)
			}
		})
	}
}
