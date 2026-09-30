package main

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

func TestTUIHistorySearchSelectionAndResponsivePreview(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	paths, db, err := initializeLibrary()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, record := range []APODRecord{
		{Date: "2024-09-27", Title: "Recent Nebula", MediaType: "image", PreviewPath: "/recent.jpg", FetchedAt: time.Now()},
		{Date: "2020-01-01", Title: "Older Galaxy", Description: "Far away", MediaType: "image", PreviewPath: "/old.jpg", FetchedAt: time.Now()},
	} {
		if err := upsertAPOD(db, record); err != nil {
			t.Fatal(err)
		}
		if err := recordWallpaperApplication(db, record.Date, "/full.jpg", time.Now()); err != nil {
			t.Fatal(err)
		}
	}
	m, err := newTUIModelFromLibrary(db, paths, "KEY", imageProtocolWezTerm, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	m.syncing = false
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	if !strings.Contains(ansi.Strip(m.renderDashboardHeader(116)), "LAST APPLIED 2020-01-01") {
		t.Fatal("persisted last-applied indicator missing")
	}
	m = typeSearchQuery(t, m, "Nebula")
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(tuiModel)
	if !m.historyMode || m.selectedRecord().Date != "2020-01-01" {
		t.Fatalf("history not in latest-use order: %q", m.selectedRecord().Date)
	}
	m = typeSearchQuery(t, m, "Galaxy")
	for _, size := range []tea.WindowSizeMsg{{Width: 120, Height: 40}, {Width: 60, Height: 20}, {Width: 35, Height: 18}, {Width: 25, Height: 10}} {
		updated, _ = m.Update(size)
		m = updated.(tuiModel)
		if m.selectedRecord().Date != "2020-01-01" || m.historyList.FilterValue() != "Galaxy" {
			t.Fatal("history search/selection lost on resize")
		}
		view := m.View().Content
		if lipgloss.Width(view) > size.Width || lipgloss.Height(view) > size.Height {
			t.Fatalf("history exceeds %dx%d", size.Width, size.Height)
		}
		if m.nativeImageWanted() {
			x, y := m.nativeImagePosition()
			if x+m.previewArea.width > size.Width || y+m.previewArea.height > size.Height-statusLineCount-verticalOuterInset {
				t.Fatal("history image outside pane")
			}
		}
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(tuiModel)
	if m.historyMode || m.recentList.FilterValue() != "Nebula" {
		t.Fatal("Recent search not restored")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(tuiModel)
	if m.historyList.FilterValue() != "Galaxy" || m.selectedRecord().Date != "2020-01-01" {
		t.Fatal("independent History search not restored")
	}
	if err := recordWallpaperApplication(db, "2024-09-27", "/full.jpg", time.Now()); err != nil {
		t.Fatal(err)
	}
	updated, _ = m.Update(wallpaperAppliedMsg{date: "2024-09-27", title: "Recent Nebula", path: "/full.jpg"})
	m = updated.(tuiModel)
	if m.lastAppliedDate != "2024-09-27" || m.historyRecords[0].Date != "2024-09-27" || m.selectedRecord().Date != "2020-01-01" {
		t.Fatal("application did not refresh history while preserving filtered selection")
	}
}

func TestTUIEmptyHistory(t *testing.T) {
	db, err := openLibrary(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	m := newTUIModel(nil, nil, "KEY")
	m.db = db
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'h', Text: "h"})
	m = updated.(tuiModel)
	if !m.historyMode || !strings.Contains(m.detail.View(), "No wallpaper history yet") || m.nativeImageWanted() {
		t.Fatal("empty History not explained")
	}
}
