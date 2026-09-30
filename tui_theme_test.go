package main

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestShortcutHintsStayWithinWidthAndDoNotCutActions(t *testing.T) {
	for _, width := range []int{12, 30, 60, 80, 120, 180} {
		hints := shortcutHints(width, "Recent APODs", "description")
		if got := ansi.StringWidth(hints); got > width {
			t.Fatalf("width %d: hint width %d exceeds limit: %q", width, got, hints)
		}
		if width >= 80 && !strings.Contains(hints, "Enter wallpaper") {
			t.Fatalf("width %d: primary action missing: %q", width, hints)
		}
		if strings.HasSuffix(hints, " • ") {
			t.Fatalf("width %d: unfinished hint: %q", width, hints)
		}
	}
}
