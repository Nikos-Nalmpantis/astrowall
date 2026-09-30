package main

import (
	"context"
	"fmt"
	"image"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/disintegration/imaging"
)

func renderPreviewBlock(previewPath string, width, height int) (string, error) {
	return renderPreviewBlockContext(context.Background(), previewPath, width, height)
}

func renderPreviewBlockContext(ctx context.Context, previewPath string, width, height int) (string, error) {
	if previewPath == "" {
		return "", fmt.Errorf("preview path is empty")
	}
	if width < 2 || height < 2 {
		return "", fmt.Errorf("preview area too small")
	}

	if err := ctx.Err(); err != nil {
		return "", err
	}
	img, err := imaging.Open(previewPath)
	if err != nil {
		return "", err
	}

	resized := imaging.Fit(img, width, height*2, imaging.Lanczos)
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return imageToANSIHalfBlocksContext(ctx, resized, width, height)
}

func imageToANSIHalfBlocks(img image.Image, width, height int) string {
	result, _ := imageToANSIHalfBlocksContext(context.Background(), img, width, height)
	return result
}

func imageToANSIHalfBlocksContext(ctx context.Context, img image.Image, width, height int) (string, error) {
	bounds := img.Bounds()
	if bounds.Dx() == 0 || bounds.Dy() == 0 || width <= 0 || height <= 0 {
		return "", nil
	}

	imageWidth := min(width, bounds.Dx())
	imageHeight := min(height, (bounds.Dy()+1)/2)
	left := (width - imageWidth) / 2
	top := (height - imageHeight) / 2
	lines := make([]string, 0, height)
	for y := range height {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if y < top || y >= top+imageHeight {
			lines = append(lines, strings.Repeat(" ", width))
			continue
		}
		var row strings.Builder
		row.WriteString(strings.Repeat(" ", left))
		for x := range imageWidth {
			pixelY := (y - top) * 2
			topColor := samplePixel(img, x, pixelY)
			bottomColor := topColor
			if pixelY+1 < bounds.Dy() {
				bottomColor = samplePixel(img, x, pixelY+1)
			}

			segment := lipgloss.NewStyle().
				Foreground(lipgloss.Color(topColor)).
				Background(lipgloss.Color(bottomColor)).
				Render("▀")
			row.WriteString(segment)
		}
		row.WriteString(strings.Repeat(" ", width-left-imageWidth))
		lines = append(lines, row.String())
	}
	return strings.Join(lines, "\n"), nil
}

func samplePixel(img image.Image, x, y int) string {
	bounds := img.Bounds()
	r, g, b, _ := img.At(bounds.Min.X+x, bounds.Min.Y+y).RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
