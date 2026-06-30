package main

import (
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestNextFavoriteRecordCyclesDeterministically(t *testing.T) {
	records := []APODRecord{
		{Date: "2024-09-27", Title: "First"},
		{Date: "2024-09-26", Title: "Second"},
		{Date: "2024-09-25", Title: "Third"},
	}

	if got := nextFavoriteRecord(records, ""); got.Date != "2024-09-27" {
		t.Fatalf("nextFavoriteRecord(empty) = %s, want 2024-09-27", got.Date)
	}
	if got := nextFavoriteRecord(records, "2024-09-27"); got.Date != "2024-09-26" {
		t.Fatalf("nextFavoriteRecord(first) = %s, want 2024-09-26", got.Date)
	}
	if got := nextFavoriteRecord(records, "2024-09-25"); got.Date != "2024-09-27" {
		t.Fatalf("nextFavoriteRecord(last) = %s, want 2024-09-27", got.Date)
	}
}

func TestCycleFavoriteWallpaperAdvancesStoredState(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, db, err := initializeLibrary()
	if err != nil {
		t.Fatalf("initializeLibrary() error: %v", err)
	}
	defer db.Close()

	for _, record := range []APODRecord{
		{Date: "2024-09-27", Title: "First", Description: "Favorite one.", MediaType: "image", URL: "https://example.com/1.jpg", Favorite: true, HDPath: "/tmp/favorite-1.jpg", FetchedAt: time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC)},
		{Date: "2024-09-26", Title: "Second", Description: "Favorite two.", MediaType: "image", URL: "https://example.com/2.jpg", Favorite: true, HDPath: "/tmp/favorite-2.jpg", FetchedAt: time.Date(2024, 9, 26, 9, 0, 0, 0, time.UTC)},
	} {
		if err := upsertAPOD(db, record); err != nil {
			t.Fatalf("upsertAPOD(%s) error: %v", record.Date, err)
		}
	}

	oldEnsure := ensureHDImageCachedFunc
	oldWallpaper := wallpaperSetterFunc
	defer func() {
		ensureHDImageCachedFunc = oldEnsure
		wallpaperSetterFunc = oldWallpaper
	}()

	var applied []string
	ensureHDImageCachedFunc = func(_ *sql.DB, _ AppPaths, record APODRecord, _ string) (string, error) {
		return record.HDPath, nil
	}
	wallpaperSetterFunc = func(path string) error {
		applied = append(applied, path)
		return nil
	}

	first, err := cycleFavoriteWallpaper(db, paths, "KEY")
	if err != nil {
		t.Fatalf("cycleFavoriteWallpaper() first error: %v", err)
	}
	if first.Date != "2024-09-27" {
		t.Fatalf("first.Date = %s, want 2024-09-27", first.Date)
	}

	second, err := cycleFavoriteWallpaper(db, paths, "KEY")
	if err != nil {
		t.Fatalf("cycleFavoriteWallpaper() second error: %v", err)
	}
	if second.Date != "2024-09-26" {
		t.Fatalf("second.Date = %s, want 2024-09-26", second.Date)
	}

	if len(applied) != 2 || applied[0] != "/tmp/favorite-1.jpg" || applied[1] != "/tmp/favorite-2.jpg" {
		t.Fatalf("applied paths = %#v", applied)
	}

	lastDate, err := getStateValue(db, lastCycledFavoriteKey)
	if err != nil {
		t.Fatalf("getStateValue() error: %v", err)
	}
	if lastDate != "2024-09-26" {
		t.Fatalf("lastDate = %s, want 2024-09-26", lastDate)
	}
}
