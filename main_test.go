package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveAPIKey(t *testing.T) {
	t.Run("flag overrides environment", func(t *testing.T) {
		t.Setenv("NASA_API_KEY", "ENV_KEY")
		if got := resolveAPIKey("FLAG_KEY"); got != "FLAG_KEY" {
			t.Fatalf("resolveAPIKey() = %q, want FLAG_KEY", got)
		}
	})

	t.Run("environment overrides demo key", func(t *testing.T) {
		t.Setenv("NASA_API_KEY", "ENV_KEY")
		if got := resolveAPIKey(""); got != "ENV_KEY" {
			t.Fatalf("resolveAPIKey() = %q, want ENV_KEY", got)
		}
	})

	t.Run("falls back to demo key", func(t *testing.T) {
		t.Setenv("NASA_API_KEY", "")
		if got := resolveAPIKey(""); got != "DEMO_KEY" {
			t.Fatalf("resolveAPIKey() = %q, want DEMO_KEY", got)
		}
	})
}

func TestRunStartupSync_NonSyncOnlyWarnsAndContinues(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, db, err := initializeLibrary()
	if err != nil {
		t.Fatalf("initializeLibrary() error: %v", err)
	}
	defer db.Close()

	originalBaseURL := apodAPIBaseURL
	apodAPIBaseURL = "http://127.0.0.1:1/planetary/apod"
	defer func() {
		apodAPIBaseURL = originalBaseURL
	}()

	var stderr bytes.Buffer
	result, err := runStartupSync(db, paths, "KEY", time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC), false, &stderr)
	if err != nil {
		t.Fatalf("runStartupSync() error: %v", err)
	}
	if result != (SyncResult{}) {
		t.Fatalf("runStartupSync() result = %#v, want zero value on warning path", result)
	}
	if !strings.Contains(stderr.String(), "Warning: could not sync local APOD library") {
		t.Fatalf("stderr = %q, want sync warning", stderr.String())
	}
}

func TestRunStartupSync_SyncOnlyReturnsError(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", filepath.Join(t.TempDir(), "data"))
	t.Setenv("XDG_CACHE_HOME", filepath.Join(t.TempDir(), "cache"))

	paths, db, err := initializeLibrary()
	if err != nil {
		t.Fatalf("initializeLibrary() error: %v", err)
	}
	defer db.Close()

	originalBaseURL := apodAPIBaseURL
	apodAPIBaseURL = "http://127.0.0.1:1/planetary/apod"
	defer func() {
		apodAPIBaseURL = originalBaseURL
	}()

	var stderr bytes.Buffer
	_, err = runStartupSync(db, paths, "KEY", time.Date(2024, 9, 27, 9, 0, 0, 0, time.UTC), true, &stderr)
	if err == nil {
		t.Fatal("runStartupSync() error = nil, want sync failure")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty buffer when returning error", stderr.String())
	}
}

func TestPrintSyncSummary(t *testing.T) {
	t.Run("up to date output", func(t *testing.T) {
		var buf bytes.Buffer
		printSyncSummary(&buf, SyncResult{AlreadyUpToDate: true})
		if got := buf.String(); got != "Local APOD library is already up to date.\n" {
			t.Fatalf("printSyncSummary() = %q", got)
		}
	})

	t.Run("sync summary output", func(t *testing.T) {
		var buf bytes.Buffer
		printSyncSummary(&buf, SyncResult{FetchedCount: 3, PreviewedCount: 2, StartDate: "2024-09-25", EndDate: "2024-09-27"})
		if got := buf.String(); got != "Synced 3 APOD items and cached 2 previews for 2024-09-25 through 2024-09-27.\n" {
			t.Fatalf("printSyncSummary() = %q", got)
		}
	})
}

func TestPrintFavoriteCycleSummary(t *testing.T) {
	var buf bytes.Buffer
	printFavoriteCycleSummary(&buf, FavoriteCycleResult{Title: "My Favorite", Date: "2024-09-27", ImagePath: "/tmp/favorite.jpg"})
	got := buf.String()
	if !strings.Contains(got, "Set favorite wallpaper to My Favorite (2024-09-27).") {
		t.Fatalf("printFavoriteCycleSummary() = %q", got)
	}
	if !strings.Contains(got, "/tmp/favorite.jpg") {
		t.Fatalf("printFavoriteCycleSummary() = %q", got)
	}
}
