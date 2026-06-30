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

	m := newTUIModel(records, nil, "KEY")
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
	m := newTUIModel(nil, nil, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	model := updated.(tuiModel)
	if !model.ready {
		t.Fatal("model.ready = false, want true")
	}
	if model.recentList.Width() == 0 || model.favoriteList.Width() == 0 {
		t.Fatal("list width = 0, want resized lists")
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
	model := newTUIModel(records, nil, "KEY")
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
	model := newTUIModel(records, nil, "KEY")
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

func TestTUIModelTabSwitchesToFavoritesPane(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: "Recent item.", MediaType: "image", URL: "https://example.com/recent.jpg"}}
	favorites := []APODRecord{{Date: "2024-08-01", Title: "Favorite", Description: "Favorite item.", MediaType: "image", URL: "https://example.com/favorite.jpg", Favorite: true}}

	m := newTUIModel(recent, favorites, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)
	updated, _ = m.Update(tea.KeyPressMsg{})
	_ = updated

	updated, _ = m.Update(tea.KeyPressMsg{Text: "tab"})
	m = updated.(tuiModel)
	if m.activePane != favoritesPane {
		t.Fatalf("activePane = %v, want favoritesPane", m.activePane)
	}
	if m.favoriteList.Title != "Favorites • active" {
		t.Fatalf("favoriteList.Title = %q, want Favorites • active", m.favoriteList.Title)
	}
	if m.recentList.Title != "Recent APODs" {
		t.Fatalf("recentList.Title = %q, want Recent APODs", m.recentList.Title)
	}
	if got := m.selectedRecord().Title; got != "Favorite" {
		t.Fatalf("selected title = %q, want Favorite", got)
	}
}

func TestTUIModelTabSwitchesPaneByKeyCode(t *testing.T) {
	recent := []APODRecord{{Date: "2024-09-27", Title: "Recent", Description: "Recent item.", MediaType: "image", URL: "https://example.com/recent.jpg"}}
	favorites := []APODRecord{{Date: "2024-08-01", Title: "Favorite", Description: "Favorite item.", MediaType: "image", URL: "https://example.com/favorite.jpg", Favorite: true}}

	m := newTUIModel(recent, favorites, "KEY")
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = updated.(tuiModel)

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m = updated.(tuiModel)
	if m.activePane != favoritesPane {
		t.Fatalf("activePane after KeyTab = %v, want favoritesPane", m.activePane)
	}

	updated, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift})
	m = updated.(tuiModel)
	if m.activePane != recentPane {
		t.Fatalf("activePane after Shift+Tab = %v, want recentPane", m.activePane)
	}
	if m.recentList.Title != "Recent APODs • active" {
		t.Fatalf("recentList.Title = %q, want Recent APODs • active", m.recentList.Title)
	}
}

func TestListFavoriteAPODsReturnsPersistentFavorites(t *testing.T) {
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

	for _, record := range []APODRecord{
		{Date: "2024-09-27", Title: "Recent Favorite", Description: "Recent favorite.", MediaType: "image", URL: "https://example.com/recent.jpg", Favorite: true, FetchedAt: time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC)},
		{Date: "2024-07-01", Title: "Old Favorite", Description: "Older favorite.", MediaType: "image", URL: "https://example.com/old.jpg", Favorite: true, FetchedAt: time.Date(2024, 7, 1, 9, 0, 0, 0, time.UTC)},
		{Date: "2024-09-26", Title: "Not Favorite", Description: "Normal item.", MediaType: "image", URL: "https://example.com/normal.jpg", Favorite: false, FetchedAt: time.Date(2024, 9, 26, 9, 0, 0, 0, time.UTC)},
	} {
		if err := upsertAPOD(db, record); err != nil {
			t.Fatalf("upsertAPOD(%s) error: %v", record.Date, err)
		}
	}

	favorites, err := listFavoriteAPODs(db)
	if err != nil {
		t.Fatalf("listFavoriteAPODs() error: %v", err)
	}
	if len(favorites) != 2 {
		t.Fatalf("len(favorites) = %d, want 2", len(favorites))
	}
	if favorites[0].Date != "2024-09-27" || favorites[1].Date != "2024-07-01" {
		t.Fatalf("favorite dates = [%s, %s], want [2024-09-27, 2024-07-01]", favorites[0].Date, favorites[1].Date)
	}
}
