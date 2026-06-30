package main

import (
	"fmt"
	"image"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/disintegration/imaging"
)

func renderPreviewBlock(previewPath string, width, height int) (string, error) {
	if previewPath == "" {
		return "", fmt.Errorf("preview path is empty")
	}
	if width < 2 || height < 2 {
		return "", fmt.Errorf("preview area too small")
	}

	if _, err := os.Stat(previewPath); err != nil {
		return "", err
	}

	img, err := imaging.Open(previewPath)
	if err != nil {
		return "", err
	}

	resized := imaging.Fit(img, width, height*2, imaging.Lanczos)
	return imageToANSIHalfBlocks(resized, width, height), nil
}

func imageToANSIHalfBlocks(img image.Image, width, height int) string {
	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 || width <= 0 || height <= 0 {
		return ""
	}

	var lines []string
	for y := 0; y < height; y++ {
		var row strings.Builder
		for x := 0; x < width; x++ {
			topY := y * 2
			bottomY := topY + 1
			top := samplePixel(img, x, topY, width, height*2)
			bottom := samplePixel(img, x, bottomY, width, height*2)

			segment := lipgloss.NewStyle().
				Foreground(lipgloss.Color(top)).
				Background(lipgloss.Color(bottom)).
				Render("▀")
			row.WriteString(segment)
		}
		lines = append(lines, row.String())
	}
	return strings.Join(lines, "\n")
}

func samplePixel(img image.Image, x, y, targetWidth, targetHeight int) string {
	bounds := img.Bounds()
	sx := bounds.Min.X + (x * bounds.Dx() / max(1, targetWidth))
	if sx >= bounds.Max.X {
		sx = bounds.Max.X - 1
	}
	sy := bounds.Min.Y + (y * bounds.Dy() / max(1, targetHeight))
	if sy >= bounds.Max.Y {
		sy = bounds.Max.Y - 1
	}
	r, g, b, _ := img.At(sx, sy).RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
