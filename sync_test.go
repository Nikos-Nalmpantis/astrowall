package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestResolveAppPaths_UsesXDGDirectories(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, err := resolveAppPaths()
	if err != nil {
		t.Fatalf("resolveAppPaths() error: %v", err)
	}

	for _, dir := range []string{paths.DataDir, paths.CacheDir, paths.PreviewDir, paths.FullDir} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Fatalf("stat %s: %v", dir, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", dir)
		}
	}

	if filepath.Base(paths.DBPath) != "astrowall.db" {
		t.Fatalf("DBPath = %q, want filename astrowall.db", paths.DBPath)
	}
}

func TestOpenLibrary_InitializesSchemaAndLatestDate(t *testing.T) {
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

	if _, err := os.Stat(paths.DBPath); err != nil {
		t.Fatalf("stat DBPath %q: %v", paths.DBPath, err)
	}

	latest, err := latestStoredDate(db)
	if err != nil {
		t.Fatalf("latestStoredDate() error: %v", err)
	}
	if latest != "" {
		t.Fatalf("latestStoredDate() = %q, want empty string", latest)
	}

	count, err := apodCount(db)
	if err != nil {
		t.Fatalf("apodCount() error: %v", err)
	}
	if count != 0 {
		t.Fatalf("apodCount = %d, want 0", count)
	}

	record := APODRecord{
		Date:        "2024-09-27",
		Title:       "Schema Test",
		Description: "Ensures schema exists.",
		MediaType:   "image",
		URL:         "https://example.com/preview.jpg",
		HDURL:       "https://example.com/full.jpg",
		FetchedAt:   time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC),
	}
	if err := upsertAPOD(db, record); err != nil {
		t.Fatalf("upsertAPOD() error: %v", err)
	}

	latest, err = latestStoredDate(db)
	if err != nil {
		t.Fatalf("latestStoredDate() after insert error: %v", err)
	}
	if latest != "2024-09-27" {
		t.Fatalf("latestStoredDate() = %q, want 2024-09-27", latest)
	}
}

func TestListRecentAPODs_ReturnsDescendingDates(t *testing.T) {
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
		{
			Date:        "2024-09-25",
			Title:       "Third",
			Description: "Third item.",
			MediaType:   "image",
			URL:         "https://example.com/third.jpg",
			FetchedAt:   time.Date(2024, 9, 25, 8, 0, 0, 0, time.UTC),
		},
		{
			Date:        "2024-09-27",
			Title:       "First",
			Description: "Newest item.",
			MediaType:   "image",
			URL:         "https://example.com/first.jpg",
			FetchedAt:   time.Date(2024, 9, 27, 8, 0, 0, 0, time.UTC),
			Favorite:    true,
		},
		{
			Date:        "2024-09-26",
			Title:       "Second",
			Description: "Middle item.",
			MediaType:   "video",
			URL:         "https://example.com/second",
			FetchedAt:   time.Date(2024, 9, 26, 8, 0, 0, 0, time.UTC),
		},
	} {
		if err := upsertAPOD(db, record); err != nil {
			t.Fatalf("upsertAPOD(%s) error: %v", record.Date, err)
		}
	}

	records, err := listRecentAPODs(db, 2)
	if err != nil {
		t.Fatalf("listRecentAPODs() error: %v", err)
	}
	if len(records) != 2 {
		t.Fatalf("len(records) = %d, want 2", len(records))
	}
	if records[0].Date != "2024-09-27" || records[1].Date != "2024-09-26" {
		t.Fatalf("dates = [%s, %s], want [2024-09-27, 2024-09-26]", records[0].Date, records[1].Date)
	}
	if !records[0].Favorite {
		t.Fatal("records[0].Favorite = false, want true")
	}
}

func TestDetermineSyncRange(t *testing.T) {
	now := time.Date(2024, 9, 27, 14, 0, 0, 0, time.UTC)

	t.Run("first run backfills 30 days", func(t *testing.T) {
		start, end, shouldSync, err := determineSyncRange(now, "")
		if err != nil {
			t.Fatalf("determineSyncRange() error: %v", err)
		}
		if !shouldSync {
			t.Fatal("shouldSync = false, want true")
		}
		if start != "2024-08-29" || end != "2024-09-27" {
			t.Fatalf("range = %s..%s, want 2024-08-29..2024-09-27", start, end)
		}
	})

	t.Run("up to date skips sync", func(t *testing.T) {
		start, end, shouldSync, err := determineSyncRange(now, "2024-09-27")
		if err != nil {
			t.Fatalf("determineSyncRange() error: %v", err)
		}
		if shouldSync {
			t.Fatal("shouldSync = true, want false")
		}
		if start != "2024-09-28" || end != "2024-09-27" {
			t.Fatalf("range = %s..%s, want 2024-09-28..2024-09-27", start, end)
		}
	})

	t.Run("missing dates resumes after latest", func(t *testing.T) {
		start, end, shouldSync, err := determineSyncRange(now, "2024-09-25")
		if err != nil {
			t.Fatalf("determineSyncRange() error: %v", err)
		}
		if !shouldSync {
			t.Fatal("shouldSync = false, want true")
		}
		if start != "2024-09-26" || end != "2024-09-27" {
			t.Fatalf("range = %s..%s, want 2024-09-26..2024-09-27", start, end)
		}
	})
}

func TestSyncAPODArchive_PersistsMetadataAndPreviews(t *testing.T) {
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

	_, err = db.Exec(`INSERT INTO apods (date, title, description, media_type, url, hd_url, fetched_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		"2024-09-25", "Existing", "Cached already", "image", "https://example.com/existing.jpg", "", time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		t.Fatalf("seed insert: %v", err)
	}

	apodCalls := 0
	var lastQuery url.Values
	previewBytes := []byte("preview-data")

	items := []APODResponse{
		{
			Date:        "2024-09-26",
			Title:       "Nebula",
			Explanation: "Clouds in space.",
			MediaType:   "image",
			URL:         "http://example.invalid/replace/preview-1.jpg",
			HDURL:       "http://example.invalid/replace/full-1.jpg",
		},
		{
			Date:         "2024-09-27",
			Title:        "Video Day",
			Explanation:  "A video with a thumbnail.",
			MediaType:    "video",
			URL:          "https://youtube.example/watch?v=123",
			ThumbnailURL: "http://example.invalid/replace/video-thumb.png",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/planetary/apod":
			apodCalls++
			lastQuery = r.URL.Query()
			json.NewEncoder(w).Encode(items)
		case "/assets/preview-1.jpg", "/assets/video-thumb.png":
			w.Write(previewBytes)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	items[0].URL = server.URL + "/assets/preview-1.jpg"
	items[0].HDURL = server.URL + "/assets/full-1.jpg"
	items[1].ThumbnailURL = server.URL + "/assets/video-thumb.png"

	originalBaseURL := apodAPIBaseURL
	apodAPIBaseURL = server.URL + "/planetary/apod"
	defer func() {
		apodAPIBaseURL = originalBaseURL
	}()

	now := time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC)
	result, err := syncAPODArchive(db, paths, "KEY", now)
	if err != nil {
		t.Fatalf("syncAPODArchive() error: %v", err)
	}

	if result.FetchedCount != 2 {
		t.Fatalf("FetchedCount = %d, want 2", result.FetchedCount)
	}
	if result.PreviewedCount != 2 {
		t.Fatalf("PreviewedCount = %d, want 2", result.PreviewedCount)
	}
	if got := lastQuery.Get("start_date"); got != "2024-09-26" {
		t.Fatalf("start_date = %q, want 2024-09-26", got)
	}
	if got := lastQuery.Get("end_date"); got != "2024-09-27" {
		t.Fatalf("end_date = %q, want 2024-09-27", got)
	}
	if got := lastQuery.Get("thumbs"); got != "true" {
		t.Fatalf("thumbs = %q, want true", got)
	}

	count, err := apodCount(db)
	if err != nil {
		t.Fatalf("apodCount() error: %v", err)
	}
	if count != 3 {
		t.Fatalf("apodCount = %d, want 3", count)
	}

	record, err := recordByDate(db, "2024-09-27")
	if err != nil {
		t.Fatalf("recordByDate() error: %v", err)
	}
	if record.MediaType != "video" {
		t.Fatalf("record.MediaType = %q, want video", record.MediaType)
	}
	if record.PreviewPath == "" {
		t.Fatal("record.PreviewPath is empty")
	}
	data, err := os.ReadFile(record.PreviewPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", record.PreviewPath, err)
	}
	if string(data) != string(previewBytes) {
		t.Fatalf("preview bytes = %q, want %q", data, previewBytes)
	}

	result, err = syncAPODArchive(db, paths, "KEY", now)
	if err != nil {
		t.Fatalf("second syncAPODArchive() error: %v", err)
	}
	if !result.AlreadyUpToDate {
		t.Fatal("AlreadyUpToDate = false, want true")
	}
	if apodCalls != 1 {
		t.Fatalf("APOD API calls = %d, want 1", apodCalls)
	}
}
