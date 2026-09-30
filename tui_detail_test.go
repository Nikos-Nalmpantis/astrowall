package main

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestDetailMetadataStates(t *testing.T) {
	for _, tc := range []struct {
		name   string
		record APODRecord
		want   []string
	}{
		{"uncached", APODRecord{MediaType: "image"}, []string{"IMAGE", "Preview —", "Full —"}},
		{"cached favorite", APODRecord{MediaType: "image", Favorite: true, PreviewPath: "/preview.jpg", HDPath: "/full.jpg"}, []string{"★ Favorite", "Preview saved", "Full saved"}},
		{"failed preview", APODRecord{MediaType: "image", PreviewError: "download failed"}, []string{"Preview error", "Full —"}},
		{"failed refresh keeps cache", APODRecord{MediaType: "image", PreviewPath: "/preview.jpg", PreviewError: "download failed"}, []string{"Preview saved (retry)"}},
		{"video", APODRecord{MediaType: "video", PreviewPath: "/thumbnail.jpg"}, []string{"VIDEO", "Preview saved", "Full n/a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.record.Date, tc.record.Title = "2024-09-27", "Nebula"
			m := newTUIModel([]APODRecord{tc.record}, nil, "KEY")
			updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
			m = updated.(tuiModel)
			header := ansi.Strip(strings.Join(m.detailHeader(tc.record), "\n"))
			for _, want := range tc.want {
				if !strings.Contains(header, want) {
					t.Errorf("header %q does not contain %q", header, want)
				}
			}
		})
	}
}

func TestPolishedDetailHeaderBoundsAndNativePlacement(t *testing.T) {
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 40}, {Width: 80, Height: 24}, {Width: 60, Height: 20}, {Width: 49, Height: 24}, {Width: 35, Height: 18}} {
		record := APODRecord{
			Date: "2024-09-27", Title: strings.Repeat("星雲 Nebula ", 30), MediaType: "image", Favorite: true,
			PreviewPath: "/preview.jpg", HDPath: "/full.jpg", Copyright: strings.Repeat("Astronomer ", 20),
			PreviewError: strings.Repeat("Failed refresh ", 20),
		}
		m := newTUIModel([]APODRecord{record}, nil, "KEY")
		m.imageProtocol = imageProtocolWezTerm
		updated, _ := m.Update(size)
		m = updated.(tuiModel)
		header := m.detailHeader(record)
		if len(header) > 5 {
			t.Fatalf("%dx%d: unbounded header height %d", size.Width, size.Height, len(header))
		}
		for _, line := range header {
			if ansi.StringWidth(line) > m.detail.Width() {
				t.Fatalf("%dx%d: header line exceeds width: %q", size.Width, size.Height, line)
			}
		}
		if m.previewArea.height < 2 {
			t.Fatalf("%dx%d: long metadata crowded out preview (%d rows)", size.Width, size.Height, m.previewArea.height)
		}
		m.nativeImage = nativeImage{protocol: imageProtocolWezTerm, path: record.PreviewPath, width: m.previewArea.width, height: m.previewArea.height, placeholders: "IMAGE ROW"}
		m.refreshDetail(false)
		lines := strings.Split(ansi.Strip(m.detail.View()), "\n")
		if len(lines) <= len(header) || !strings.HasPrefix(lines[len(header)], "IMAGE ROW") {
			t.Fatalf("%dx%d: image row does not follow measured header: %q", size.Width, size.Height, lines)
		}
		_, y := m.nativeImagePosition()
		contentTop := headerLineCount + verticalOuterInset + m.detailStyle.GetBorderTopSize()
		if m.stackedLayout() {
			_, frameHeight := m.listStyle.GetFrameSize()
			contentTop += m.activeList().Height() + frameHeight
		}
		if y != contentTop+len(header) || y+m.previewArea.height != contentTop+m.detail.Height() {
			t.Fatalf("%dx%d: image y=%d rows=%d, detail top=%d rows=%d", size.Width, size.Height, y, m.previewArea.height, contentTop, m.detail.Height())
		}
		if view := m.View().Content; lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
			t.Fatalf("%dx%d: dashboard bounds exceeded", size.Width, size.Height)
		}
	}
}

func TestDetailAttributionAndPreviewErrorRemainReadableInDescription(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", Title: "Nebula", MediaType: "image", PreviewPath: "/preview.jpg", Copyright: "Alice and Bob Observatory", PreviewError: "refresh failed: connection interrupted", Description: "A nebula in the night sky."}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	if !strings.Contains(ansi.Strip(m.detail.View()), "© Alice and Bob Observatory") {
		t.Fatal("image view attribution missing")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Text: "d", Code: 'd'})
	m = updated.(tuiModel)
	view := ansi.Strip(m.detail.View())
	for _, want := range []string{"Credit: " + record.Copyright, "Preview error: " + record.PreviewError, record.Description} {
		if !strings.Contains(view, want) {
			t.Errorf("description missing %q: %q", want, view)
		}
	}
	updated, _ = m.Update(tea.WindowSizeMsg{Width: 35, Height: 18})
	m = updated.(tuiModel)
	m.showDescription = false
	m.refreshDetail(true)
	if strings.Contains(strings.Join(m.detailHeader(record), "\n"), "©") {
		t.Fatal("short pane should reserve image space instead of adding attribution")
	}
}

func TestFullImageCacheFeedbackUpdatesDetailWithoutShiftingPreview(t *testing.T) {
	record := APODRecord{Date: "2024-09-27", Title: "Nebula", MediaType: "image", PreviewPath: "/preview.jpg"}
	m := newTUIModel([]APODRecord{record}, nil, "KEY")
	m.imageProtocol = imageProtocolWezTerm
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	_, oldY := m.nativeImagePosition()
	updated, _ = m.Update(wallpaperAppliedMsg{date: record.Date, title: record.Title, path: "/full.jpg"})
	m = updated.(tuiModel)
	_, newY := m.nativeImagePosition()
	if oldY != newY || !strings.Contains(ansi.Strip(m.detail.View()), "Full saved") {
		t.Fatalf("cache feedback shifted image or is missing: y=%d/%d, %q", oldY, newY, m.detail.View())
	}
}
