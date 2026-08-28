package main

import (
	"crypto/rand"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
)

func TestParseImageProtocol(t *testing.T) {
	for _, value := range []string{"auto", "kitty", "wezterm", "ansi", "KITTY"} {
		if _, err := parseImageProtocol(value); err != nil {
			t.Fatalf("parseImageProtocol(%q) error: %v", value, err)
		}
	}
	if _, err := parseImageProtocol("sixel"); err == nil {
		t.Fatal("parseImageProtocol(sixel) error = nil")
	}
}

func TestResolveImageProtocol(t *testing.T) {
	tests := []struct {
		name      string
		requested imageProtocol
		env       map[string]string
		want      imageProtocol
	}{
		{name: "forced ANSI", requested: imageProtocolANSI, env: map[string]string{"TERM": "xterm-kitty"}, want: imageProtocolANSI},
		{name: "forced Kitty in tmux", requested: imageProtocolKitty, env: map[string]string{"TMUX": "/tmp/tmux"}, want: imageProtocolKitty},
		{name: "forced WezTerm in tmux falls back", requested: imageProtocolWezTerm, env: map[string]string{"TMUX": "/tmp/tmux"}, want: imageProtocolANSI},
		{name: "Kitty terminal", requested: imageProtocolAuto, env: map[string]string{"KITTY_WINDOW_ID": "1"}, want: imageProtocolKitty},
		{name: "Ghostty terminal", requested: imageProtocolAuto, env: map[string]string{"TERM": "xterm-ghostty"}, want: imageProtocolKitty},
		{name: "WezTerm backend", requested: imageProtocolAuto, env: map[string]string{"TERM_PROGRAM": "WezTerm"}, want: imageProtocolWezTerm},
		{name: "iTerm2 remains ANSI", requested: imageProtocolAuto, env: map[string]string{"TERM_PROGRAM": "iTerm.app"}, want: imageProtocolANSI},
		{name: "tmux is conservative", requested: imageProtocolAuto, env: map[string]string{"TMUX": "/tmp/tmux", "KITTY_WINDOW_ID": "1"}, want: imageProtocolANSI},
		{name: "unsupported terminal", requested: imageProtocolAuto, env: map[string]string{"TERM": "xterm-256color"}, want: imageProtocolANSI},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			getenv := func(key string) string { return tt.env[key] }
			if got := resolveImageProtocol(tt.requested, getenv); got != tt.want {
				t.Fatalf("resolveImageProtocol() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPrepareKittyImageTranscodesJPEGAndBuildsPlaceholders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preview.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := range 8 {
		for x := range 8 {
			img.Set(x, y, color.RGBA{R: uint8(x * 20), G: uint8(y * 20), B: 120, A: 255})
		}
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := jpeg.Encode(file, img, nil); err != nil {
		t.Fatalf("jpeg.Encode() error: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}

	native, err := prepareKittyImage(path, 5, 3, false)
	if err != nil {
		t.Fatalf("prepareKittyImage() error: %v", err)
	}
	if !strings.Contains(native.transmission, "f=100") || !strings.Contains(native.transmission, "U=1") {
		t.Fatalf("transmission = %q", native.transmission)
	}
	if strings.Count(native.placeholders, string(kittyPlaceholder)) != 15 {
		t.Fatalf("placeholder count = %d, want 15", strings.Count(native.placeholders, string(kittyPlaceholder)))
	}
	if lipgloss.Width(native.placeholders) != 5 || lipgloss.Height(native.placeholders) != 3 {
		t.Fatalf("placeholder dimensions = %dx%d", lipgloss.Width(native.placeholders), lipgloss.Height(native.placeholders))
	}
}

func TestKittySequencesSupportChunkingTmuxAndDeletion(t *testing.T) {
	path := filepath.Join(t.TempDir(), "large.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 128, 128))
	if _, err := rand.Read(img.Pix); err != nil {
		t.Fatalf("rand.Read() error: %v", err)
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := jpeg.Encode(file, img, nil); err != nil {
		t.Fatalf("jpeg.Encode() error: %v", err)
	}
	file.Close()

	native, err := prepareKittyImage(path, 10, 5, true)
	if err != nil {
		t.Fatalf("prepareKittyImage() error: %v", err)
	}
	if !strings.Contains(native.transmission, "\x1bPtmux;") || !strings.Contains(native.transmission, "m=1") {
		t.Fatalf("tmux chunked transmission missing from %q", native.transmission)
	}
	deletion := kittyDeleteImage(native.id, true)
	if !strings.Contains(deletion, "d=I") || !strings.Contains(deletion, "\x1bPtmux;") {
		t.Fatalf("deletion = %q", deletion)
	}
}

func TestPrepareWezTermImageBuildsPositionedPlacement(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preview.jpg")
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	file, err := os.Create(path)
	if err != nil {
		t.Fatalf("Create() error: %v", err)
	}
	if err := jpeg.Encode(file, img, nil); err != nil {
		t.Fatalf("jpeg.Encode() error: %v", err)
	}
	file.Close()

	native, err := prepareWezTermImageContext(t.Context(), path, 5, 3, 10, 4)
	if err != nil {
		t.Fatalf("prepareWezTermImageContext() error: %v", err)
	}
	if native.protocol != imageProtocolWezTerm {
		t.Fatalf("protocol = %q", native.protocol)
	}
	if !strings.Contains(native.transmission, "a=t,f=100") {
		t.Fatalf("transmission = %q", native.transmission)
	}
	if !strings.Contains(native.placement, "\x1b7\x1b[5;11H\x1b_Ga=p") || !strings.HasSuffix(native.placement, "\x1b8") {
		t.Fatalf("placement = %q", native.placement)
	}
	for _, field := range []string{"c=5", "r=3", "C=1", fmt.Sprintf("i=%d", native.id)} {
		if !strings.Contains(native.placement, field) {
			t.Fatalf("placement missing %q: %q", field, native.placement)
		}
	}
	if lipgloss.Width(native.placeholders) != 5 || lipgloss.Height(native.placeholders) != 3 {
		t.Fatalf("blank grid dimensions = %dx%d", lipgloss.Width(native.placeholders), lipgloss.Height(native.placeholders))
	}
}
