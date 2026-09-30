package main

import (
	"fmt"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

// Palette shared by the browser, its lists, and its detail view.
var (
	colorStarlight = lipgloss.Color("#F2F0FF")
	colorMuted     = lipgloss.Color("#A5A9C5")
	colorViolet    = lipgloss.Color("#C5A3FF")
	colorCyan      = lipgloss.Color("#79DFEA")
	colorGold      = lipgloss.Color("#FFD17A")
	colorRed       = lipgloss.Color("#FF929E")
	colorBorder    = lipgloss.Color("#646785")
	colorPanel     = lipgloss.Color("#33304D")

	selectedAccent = colorViolet
	primaryText    = lipgloss.NewStyle().Foreground(colorStarlight)
	secondaryText  = lipgloss.NewStyle().Foreground(colorMuted)
	accentText     = lipgloss.NewStyle().Foreground(colorCyan)
	favoriteText   = lipgloss.NewStyle().Foreground(colorGold)
	statusError    = lipgloss.NewStyle().Foreground(colorRed)
)

func styleStatus(status string) string {
	switch {
	case strings.Contains(status, "failed"), strings.Contains(status, "unavailable"), strings.Contains(status, "cannot be empty"):
		return statusError.Render(status)
	case strings.HasPrefix(status, "Syncing"), strings.HasPrefix(status, "Checking"), strings.HasPrefix(status, "Setting"), strings.HasPrefix(status, "Validating"):
		return accentText.Render(status)
	default:
		return primaryText.Render(status)
	}
}

// Pick complete groups of controls rather than truncating mid-action.
func shortcutHints(width int, pane, detail string) string {
	options := []string{
		"/ search  •  b Recent/Archive  •  h History  •  s sync  •  p retry preview  •  Tab panes  •  j/k move  •  d " + detail + "  •  Enter wallpaper  •  f favorite  •  o page  •  u media  •  a API key  •  ? help  •  q quit",
		"/ search  •  b Recent/Archive  •  h History  •  s sync  •  p retry  •  Tab panes  •  j/k move  •  d " + detail + "  •  Enter wallpaper  •  f favorite  •  ? help  •  q quit",
		"/ search  •  b Archive  •  h History  •  Tab panes  •  Enter wallpaper  •  ? help",
		"/ search  •  b Archive  •  Tab panes  •  Enter wallpaper",
		"? help",
	}
	for _, hints := range options {
		text := pane + "  ·  " + hints
		if ansi.StringWidth(text) <= width {
			return text
		}
	}
	return ansi.Truncate(pane, width, "")
}

func (m tuiModel) renderDashboardHeader(width int) string {
	brand := accentText.Bold(true).Render("✦ ASTROWALL")
	if m.lastAppliedDate != "" {
		label := "LAST APPLIED " + m.lastAppliedDate
		counts := fmt.Sprintf("LIBRARY %d  ·  ★ %d", m.libraryCount, len(m.favoriteRecords))
		gap := width - ansi.StringWidth(brand) - ansi.StringWidth(label) - ansi.StringWidth(counts) - 4
		if gap >= 2 {
			return brand + "  " + secondaryText.Render(label) + strings.Repeat(" ", gap+2) + favoriteText.Render(counts)
		}
	}
	for _, summary := range []string{
		fmt.Sprintf("LIBRARY %d  ·  ★ %d", m.libraryCount, len(m.favoriteRecords)),
		fmt.Sprintf("%d APODs  ·  ★ %d", m.libraryCount, len(m.favoriteRecords)),
		fmt.Sprintf("★ %d", len(m.favoriteRecords)),
		"",
	} {
		if summary == "" {
			return ansi.Truncate(brand, width, "")
		}
		gap := width - ansi.StringWidth(brand) - ansi.StringWidth(summary)
		if gap >= 2 {
			return brand + strings.Repeat(" ", gap) + favoriteText.Render(summary)
		}
	}
	return ansi.Truncate(brand, width, "")
}
