package main

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/reflow/wordwrap"
)

func (m *tuiModel) refreshDetail(resetScroll bool) {
	record := m.selectedRecord()
	if record.Date == "" {
		m.previewArea.height = 0
		m.detail.SetContent(m.emptyDetailMessage())
		if resetScroll {
			m.detail.GotoTop()
		}
		return
	}
	m.previewArea.height = max(0, m.detail.Height()-m.detailHeaderHeight(record))
	parts := m.detailHeader(record)
	if m.descriptionVisible() {
		if record.PreviewPath == "" {
			parts = append(parts, "", secondaryText.Render("Preview unavailable · showing description"))
		}
		if record.Copyright != "" {
			parts = append(parts, "", secondaryText.Render("Credit: "+strings.TrimSpace(record.Copyright)))
		}
		if record.Date == m.lastAppliedDate {
			parts = append(parts, "", favoriteText.Render("Last applied by Astrowall"))
		}
		if !record.LastAppliedAt.IsZero() {
			parts = append(parts, "", secondaryText.Render("Last used: "+record.LastAppliedAt.Local().Format("2006-01-02 15:04 MST")))
		}
		if record.PreviewError != "" {
			parts = append(parts, "", statusError.Render("Preview error: "+record.PreviewError))
		}
		parts = append(parts, "", accentText.Bold(true).Render("Description"), "", strings.TrimSpace(record.Description))
		// Only prose needs wrapping. Image rows and the bounded header must keep
		// their exact dimensions for native placement and ANSI letterboxing.
		m.detail.SetContent(wordwrap.String(strings.Join(parts, "\n"), max(1, m.detail.Width())))
	} else {
		if record.PreviewPath != "" && m.nativeImageMatches(record.PreviewPath) {
			parts = append(parts, m.nativeImage.placeholders)
		} else if record.PreviewPath != "" {
			result := m.ansiResult
			if m.ansiKey != m.ansiPreviewKey() {
				result = m.ansiCache[m.ansiPreviewKey()]
			}
			switch {
			case result.preview != "":
				parts = append(parts, result.preview)
			case result.err != nil:
				parts = append(parts, wordwrap.String("Preview unavailable. Press d to read the description or u to open the media.", max(1, m.detail.Width())))
			default:
				parts = append(parts, ansi.Truncate("Preparing image preview…", max(1, m.detail.Width()), "…"))
			}
		}
		m.detail.SetContent(strings.Join(parts, "\n"))
	}
	if resetScroll {
		m.detail.GotoTop()
	}
}

func (m tuiModel) detailHeaderHeight(record APODRecord) int {
	return len(m.detailHeader(record))
}

func (m tuiModel) detailHeader(record APODRecord) []string {
	width := max(1, m.detail.Width())
	title := strings.Join(strings.Fields(record.Title), " ")
	if title == "" {
		title = "Untitled APOD"
	}
	titleLines := strings.Split(wordwrap.String(title, width), "\n")
	// A long title must not consume the entire image pane.
	maxTitleLines := 2
	if m.detail.Height() < 6 {
		maxTitleLines = 1
	}
	if len(titleLines) > maxTitleLines {
		titleLines = append(titleLines[:maxTitleLines-1], strings.Join(titleLines[maxTitleLines-1:], " "))
	}
	parts := make([]string, 0, 5)
	for _, line := range titleLines {
		parts = append(parts, primaryText.Bold(true).Render(ansi.Truncate(line, width, "…")))
	}
	media := strings.ToUpper(record.MediaType)
	if media == "" {
		media = "APOD"
	}
	metadata := accentText.Render(record.Date) + secondaryText.Render(" · ") + accentText.Render(media)
	if record.Favorite {
		badge := " · ★ Favorite"
		if ansi.StringWidth(metadata)+ansi.StringWidth(badge) > width {
			badge = " · ★"
		}
		metadata += favoriteText.Render(badge)
	}
	parts = append(parts, ansi.Truncate(metadata, width, "…"))
	preview, full := "Preview —", "Full —"
	if record.PreviewPath != "" {
		preview = "Preview saved"
	}
	if record.HDPath != "" {
		full = "Full saved"
	} else if record.MediaType != "" && record.MediaType != "image" {
		full = "Full n/a"
	}
	if record.PreviewError != "" {
		if record.PreviewPath == "" {
			preview = "Preview error"
		} else {
			preview = "Preview saved (retry)"
		}
	}
	cache := preview + " · " + full
	if record.Date == m.lastAppliedDate {
		cache += " · Last applied"
	}
	if ansi.StringWidth(cache) > width {
		cache = strings.ReplaceAll(cache, "saved", "✓")
		cache = strings.ReplaceAll(cache, " (retry)", " !")
	}
	cacheStyle := secondaryText
	if record.PreviewError != "" {
		cacheStyle = statusError
	}
	parts = append(parts, cacheStyle.Render(ansi.Truncate(cache, width, "…")))
	// Attribution is abbreviated here and shown in full in description view.
	// Short detail panes reserve the remaining rows for the image instead.
	if record.Copyright != "" && !m.descriptionVisible() && m.detail.Height()-len(parts) >= 6 {
		credit := "© " + strings.Join(strings.Fields(record.Copyright), " ")
		parts = append(parts, secondaryText.Render(ansi.Truncate(credit, width, "…")))
	}
	return parts
}
