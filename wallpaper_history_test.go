package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWallpaperHistoryPersistsAndOrdersRepeatedApplications(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	db, err := openLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, date := range []string{"2024-09-27", "2024-09-26"} {
		if err := upsertAPOD(db, APODRecord{Date: date, Title: date, MediaType: "image", FetchedAt: time.Now()}); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC()
	for _, date := range []string{"2024-09-27", "2024-09-26", "2024-09-27"} {
		if err := recordWallpaperApplication(db, date, "/wallpaper.jpg", now); err != nil {
			t.Fatal(err)
		}
	}
	db.Close()
	db, err = openLibrary(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	records, err := listWallpaperHistory(db)
	if err != nil || len(records) != 2 || records[0].Date != "2024-09-27" || records[1].Date != "2024-09-26" || !records[0].LastAppliedAt.Equal(now) {
		t.Fatalf("history = %#v, %v", records, err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wallpaper_history`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("events = %d, %v", count, err)
	}
	date, err := lastAppliedWallpaperDate(db)
	if err != nil || date != records[0].Date {
		t.Fatalf("last applied = %q, %v", date, err)
	}
}

func TestRecordedWallpaperExcludesFailuresAndReportsHistoryWarning(t *testing.T) {
	db, err := openLibrary(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	record := APODRecord{Date: "2024-09-27", MediaType: "image", FetchedAt: time.Now()}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	oldSetter := wallpaperSetterFunc
	t.Cleanup(func() { wallpaperSetterFunc = oldSetter })
	wallpaperSetterFunc = func(string) error { return errors.New("desktop failed") }
	historyErr, applyErr := applyRecordedWallpaper(db, record.Date, "/image.jpg")
	if applyErr == nil || historyErr != nil {
		t.Fatalf("failed application = %v, %v", historyErr, applyErr)
	}
	if date, err := lastAppliedWallpaperDate(db); err != nil || date != "" {
		t.Fatalf("failed application recorded: %q, %v", date, err)
	}
	wallpaperSetterFunc = func(string) error { return nil }
	historyErr, applyErr = applyRecordedWallpaper(db, record.Date, "/image.jpg")
	if historyErr != nil || applyErr != nil {
		t.Fatalf("success = %v, %v", historyErr, applyErr)
	}
	if _, err := db.Exec(`DROP TABLE wallpaper_history`); err != nil {
		t.Fatal(err)
	}
	historyErr, applyErr = applyRecordedWallpaper(db, record.Date, "/image.jpg")
	if historyErr == nil || applyErr != nil {
		t.Fatalf("history write failure must be a warning: %v, %v", historyErr, applyErr)
	}
}

func TestTUIWallpaperCommandReappliesCacheAndRecordsHistory(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	paths, db, err := initializeLibrary()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	path := filepath.Join(paths.FullDir, "2024-09-27.jpg")
	if err := os.WriteFile(path, []byte("cached"), 0o644); err != nil {
		t.Fatal(err)
	}
	record := APODRecord{Date: "2024-09-27", Title: "Nebula", MediaType: "image", HDPath: path, FetchedAt: time.Now()}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	oldSetter, oldURL := wallpaperSetterFunc, apodAPIBaseURL
	t.Cleanup(func() { wallpaperSetterFunc, apodAPIBaseURL = oldSetter, oldURL })
	apodAPIBaseURL = "http://127.0.0.1:1"
	applied := ""
	wallpaperSetterFunc = func(path string) error { applied = path; return nil }
	msg := applyWallpaperCmd(db, paths, record, "KEY")().(wallpaperAppliedMsg)
	if msg.err != nil || msg.historyErr != nil || applied != path {
		t.Fatalf("reapply = %#v, path %q", msg, applied)
	}
	history, err := listWallpaperHistory(db)
	if err != nil || len(history) != 1 || history[0].Date != record.Date {
		t.Fatalf("reapply history = %#v, %v", history, err)
	}
}

func TestHistoryReapplicationRecoversMissingCache(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	paths, db, err := initializeLibrary()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.Write([]byte("recovered image"))
	}))
	defer server.Close()
	record := APODRecord{Date: "2024-09-27", Title: "Nebula", MediaType: "image", URL: server.URL + "/image.jpg", HDPath: filepath.Join(paths.FullDir, "missing.jpg"), FetchedAt: time.Now()}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	if err := recordWallpaperApplication(db, record.Date, record.HDPath, time.Now()); err != nil {
		t.Fatal(err)
	}
	history, err := listWallpaperHistory(db)
	if err != nil {
		t.Fatal(err)
	}
	oldSetter := wallpaperSetterFunc
	t.Cleanup(func() { wallpaperSetterFunc = oldSetter })
	wallpaperSetterFunc = func(path string) error {
		data, err := os.ReadFile(path)
		if err != nil || string(data) != "recovered image" {
			t.Fatalf("recovered image = %q, %v", data, err)
		}
		return nil
	}
	msg := applyWallpaperCmd(db, paths, history[0], "KEY")().(wallpaperAppliedMsg)
	if msg.err != nil || msg.historyErr != nil || requests != 1 {
		t.Fatalf("recovered application = %#v, requests %d", msg, requests)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM wallpaper_history`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("reapplication events = %d, %v", count, err)
	}
}
