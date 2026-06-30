package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
)

func TestNewTUIModel_SelectsNewestRecord(t *testing.T) {
	records := []APODRecord{
		{Date: "2024-09-27", Title: "Newest", Description: "Latest item.", MediaType: "image", URL: "https://example.com/1.jpg"},
		{Date: "2024-09-26", Title: "Older", Description: "Older item.", MediaType: "image", URL: "https://example.com/2.jpg"},
	}

	m := newTUIModel(records, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	selected := m.selectedRecord()
	if selected.Date != "2024-09-27" {
		t.Fatalf("selected.Date = %q, want 2024-09-27", selected.Date)
	}
	if !strings.Contains(m.detail.View(), "Latest item.") {
		t.Fatalf("detail view = %q, want description", m.detail.View())
	}
}

func TestTUIModelWindowResizeSetsReady(t *testing.T) {
	m := newTUIModel(nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(tuiModel)
	if !model.ready {
		t.Fatal("model.ready = false, want true")
	}
	if model.list.Width() == 0 {
		t.Fatal("list width = 0, want resized list")
	}
	if model.detail.Width() == 0 || model.detail.Height() == 0 {
		t.Fatal("detail viewport not resized")
	}
}

func TestRunTUIModelUsesRecentRecords(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	if err := upsertAPOD(db, APODRecord{
		Date:        "2024-09-27",
		Title:       "Stored",
		Description: "Stored item.",
		MediaType:   "image",
		URL:         "https://example.com/stored.jpg",
		FetchedAt:   time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}

	records, err := listRecentAPODs(db, 30)
	if err != nil {
		t.Fatalf("listRecentAPODs() error: %v", err)
	}
	model := newTUIModel(records, "KEY")
	updated, _ := model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model = updated.(tuiModel)
	if got := model.selectedRecord().Title; got != "Stored" {
		t.Fatalf("selected title = %q, want Stored", got)
	}
}

func TestEnsureHDImageCached_ReusesExistingFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	hdPath := filepath.Join(paths.FullDir, "2024-09-27.jpg")
	if err := os.WriteFile(hdPath, []byte("cached-full"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	record := APODRecord{Date: "2024-09-27", Title: "Cached", HDPath: hdPath}
	got, err := ensureHDImageCached(db, paths, record, "KEY")
	if err != nil {
		t.Fatalf("ensureHDImageCached() error: %v", err)
	}
	if got != hdPath {
		t.Fatalf("ensureHDImageCached() = %q, want %q", got, hdPath)
	}
}

func TestEnsureHDImageCached_UsesStoredHDPathFromDatabase(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	hdPath := filepath.Join(paths.FullDir, "2024-09-27.jpg")
	if err := os.WriteFile(hdPath, []byte("cached-full"), 0o644); err != nil {
		t.Fatalf("WriteFile() error: %v", err)
	}

	if err := upsertAPOD(db, APODRecord{
		Date:        "2024-09-27",
		Title:       "Stored",
		Description: "Stored item.",
		MediaType:   "image",
		URL:         "https://example.com/stored.jpg",
		HDPath:      hdPath,
		FetchedAt:   time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}

	originalBaseURL := apodAPIBaseURL
	apodAPIBaseURL = "http://127.0.0.1:1/planetary/apod"
	defer func() {
		apodAPIBaseURL = originalBaseURL
	}()

	staleRecord := APODRecord{Date: "2024-09-27", Title: "Stored"}
	got, err := ensureHDImageCached(db, paths, staleRecord, "KEY")
	if err != nil {
		t.Fatalf("ensureHDImageCached() error: %v", err)
	}
	if got != hdPath {
		t.Fatalf("ensureHDImageCached() = %q, want %q", got, hdPath)
	}
}

func TestToggleFavoriteCmdUpdatesState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}
	db, err := openLibrary(paths.DBPath)
	if err != nil {
		t.Fatalf("openLibrary() error: %v", err)
	}
	defer db.Close()

	if err := upsertAPOD(db, APODRecord{
		Date:        "2024-09-27",
		Title:       "Favorite me",
		Description: "Stored item.",
		MediaType:   "image",
		URL:         "https://example.com/stored.jpg",
		FetchedAt:   time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}

	records, err := listRecentAPODs(db, 30)
	if err != nil {
		t.Fatalf("listRecentAPODs() error: %v", err)
	}
	model := newTUIModel(records, "KEY")
	model.db = db
	model.paths = paths

	msg := toggleFavoriteCmd(db, "2024-09-27")().(favoriteToggledMsg)
	if msg.err != nil {
		t.Fatalf("toggleFavoriteCmd() error: %v", msg.err)
	}
	if !msg.favorite {
		t.Fatal("msg.favorite = false, want true")
	}

	updatedModel, _ := model.Update(msg)
	model = updatedModel.(tuiModel)
	if !model.selectedRecord().Favorite {
		t.Fatal("selectedRecord().Favorite = false, want true")
	}
}
