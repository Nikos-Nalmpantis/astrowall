package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestWallpaperCacheLifecycle(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	paths, db, err := initializeLibrary()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	apiRequests, imageRequests := 0, 0
	var apod APODResponse
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/image.jpg" {
			imageRequests++
			w.Write([]byte("complete full image"))
			return
		}
		apiRequests++
		if r.URL.Query().Get("count") == "1" {
			json.NewEncoder(w).Encode([]APODResponse{apod})
		} else {
			json.NewEncoder(w).Encode(apod)
		}
	}))
	defer server.Close()
	apod = APODResponse{Date: "2024-09-27", Title: "Nebula", MediaType: "image", HDURL: server.URL + "/image.jpg"}
	oldURL := apodAPIBaseURL
	apodAPIBaseURL = server.URL + "/apod"
	t.Cleanup(func() { apodAPIBaseURL = oldURL })
	now := time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC)

	got, err := resolveWallpaperAPOD(db, "KEY", false, apod.Date, now)
	if err != nil || got.Title != apod.Title {
		t.Fatalf("resolveWallpaperAPOD() = %#v, %v", got, err)
	}
	cachePath, err := ensureHDImageCached(db, paths, APODRecord{Date: got.Date}, "KEY")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(t.TempDir(), "wallpaper.jpg")
	if err := copyImageAtomic(cachePath, output); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "complete full image" {
		t.Fatalf("output = %q, %v", data, err)
	}
	stored, err := recordByDate(db, got.Date)
	if err != nil || stored.HDPath != cachePath {
		t.Fatalf("stored cache = %q, %v", stored.HDPath, err)
	}
	// A downloaded file can outlive an unsuccessful database-path update.
	if err := updateHDPath(db, got.Date, ""); err != nil {
		t.Fatal(err)
	}
	if recovered, err := ensureHDImageCached(db, paths, APODRecord{Date: got.Date}, "KEY"); err != nil || recovered != cachePath {
		t.Fatalf("unlinked cache recovery = %q, %v", recovered, err)
	}
	// A random APOD still comes from NASA, but its image should be reused.
	got, err = resolveWallpaperAPOD(db, "KEY", true, "", now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ensureHDImageCached(db, paths, APODRecord{Date: got.Date}, "KEY"); err != nil {
		t.Fatal(err)
	}
	if apiRequests != 2 || imageRequests != 1 {
		t.Fatalf("API/image requests = %d/%d, want 2/1", apiRequests, imageRequests)
	}

	server.Close()
	for _, date := range []string{apod.Date, ""} {
		got, err := resolveWallpaperAPOD(db, "KEY", false, date, now)
		if err != nil {
			t.Fatalf("offline metadata for %q: %v", date, err)
		}
		if _, err := ensureHDImageCached(db, paths, APODRecord{Date: got.Date}, "KEY"); err != nil {
			t.Fatalf("offline image: %v", err)
		}
	}
	// Copying onto the same cache path must not truncate the source.
	if err := copyImageAtomic(cachePath, cachePath); err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(cachePath)
	if err != nil || string(data) != "complete full image" {
		t.Fatalf("cache after same-file copy = %q, %v", data, err)
	}
}

func TestHDCacheFailedDownloadCanBeRetried(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))
	paths, db, err := initializeLibrary()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.Header().Set("Content-Length", "100")
			w.Write([]byte("partial"))
			return
		}
		w.Write([]byte("complete"))
	}))
	defer server.Close()
	record := APODRecord{Date: "2024-09-27", MediaType: "image", URL: server.URL + "/image.jpg", FetchedAt: time.Now()}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureHDImageCached(db, paths, record, "KEY"); err == nil {
		t.Fatal("expected interrupted download to fail")
	}
	files, err := os.ReadDir(paths.FullDir)
	if err != nil || len(files) != 0 {
		t.Fatalf("files after interrupted download = %v, %v", files, err)
	}
	stored, err := recordByDate(db, record.Date)
	if err != nil || stored.HDPath != "" {
		t.Fatalf("failed download recorded as cached: %#v, %v", stored, err)
	}
	path, err := ensureHDImageCached(db, paths, record, "KEY")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil || string(data) != "complete" || attempts != 2 {
		t.Fatalf("retry = %q, %v (%d attempts)", data, err, attempts)
	}
}

func TestHDCacheRepairsEmptyFileAndRejectsVideos(t *testing.T) {
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
		w.Write([]byte("repaired"))
	}))
	defer server.Close()
	path := filepath.Join(paths.FullDir, "2024-09-27.jpg")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	record := APODRecord{Date: "2024-09-27", MediaType: "image", URL: server.URL + "/image.jpg", HDPath: path, FetchedAt: time.Now()}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatal(err)
	}
	if got, err := ensureHDImageCached(db, paths, record, "KEY"); err != nil || got != path || requests != 1 {
		t.Fatalf("empty cache repair = %q, %v, requests %d", got, err, requests)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if got, err := ensureHDImageCached(db, paths, record, "KEY"); err != nil || got != path || requests != 2 {
		t.Fatalf("missing cache repair = %q, %v, requests %d", got, err, requests)
	}
	if _, err := db.Exec(`UPDATE apods SET media_type = 'video' WHERE date = ?`, record.Date); err != nil {
		t.Fatal(err)
	}
	if _, err := ensureHDImageCached(db, paths, record, "KEY"); err == nil {
		t.Fatal("cached video should not be applied as wallpaper")
	}
}

func TestCopyImageAtomicPreservesOutputOnReadFailure(t *testing.T) {
	dir := t.TempDir()
	output := filepath.Join(dir, "wallpaper.jpg")
	if err := os.WriteFile(output, []byte("original wallpaper"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Reading a directory as image bytes fails after opening it.
	if err := copyImageAtomic(dir, output); err == nil {
		t.Fatal("expected source read failure")
	}
	data, err := os.ReadFile(output)
	if err != nil || string(data) != "original wallpaper" {
		t.Fatalf("destination after failed copy = %q, %v", data, err)
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 1 {
		t.Fatalf("temporary copies leaked: %v, %v", files, err)
	}
}
