package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildAPODURL(t *testing.T) {
	tests := []struct {
		name   string
		key    string
		random bool
		date   string
		want   string
	}{
		{
			name: "current",
			key:  "KEY",
			want: "https://api.nasa.gov/planetary/apod?api_key=KEY",
		},
		{
			name:   "random",
			key:    "KEY",
			random: true,
			want:   "https://api.nasa.gov/planetary/apod?api_key=KEY&count=1",
		},
		{
			name: "specific date",
			key:  "KEY",
			date: "2024-01-15",
			want: "https://api.nasa.gov/planetary/apod?api_key=KEY&date=2024-01-15",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildAPODURL(tt.key, tt.random, tt.date)
			if got != tt.want {
				t.Errorf("buildAPODURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolveImagePath_Default(t *testing.T) {
	path, err := resolveImagePath("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	home, _ := os.UserHomeDir()
	expected := filepath.Join(home, "Pictures", "apod_wallpaper.jpg")
	if path != expected {
		t.Errorf("resolveImagePath(\"\") = %q, want %q", path, expected)
	}
}

func TestResolveImagePath_Custom(t *testing.T) {
	dir := t.TempDir()
	custom := filepath.Join(dir, "sub", "wallpaper.jpg")

	path, err := resolveImagePath(custom)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if path != custom {
		t.Errorf("resolveImagePath() = %q, want %q", path, custom)
	}
	if _, err := os.Stat(filepath.Join(dir, "sub")); os.IsNotExist(err) {
		t.Error("parent directory was not created")
	}
}

func TestFetchAPOD_SingleObject(t *testing.T) {
	apod := APODResponse{
		Date:      "2024-09-27",
		Title:     "Test Nebula",
		HDURL:     "https://example.com/hd.jpg",
		URL:       "https://example.com/sd.jpg",
		MediaType: "image",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(apod)
	}))
	defer srv.Close()

	got, err := fetchAPOD(srv.URL)
	if err != nil {
		t.Fatalf("fetchAPOD() error: %v", err)
	}
	if got.Title != apod.Title {
		t.Errorf("Title = %q, want %q", got.Title, apod.Title)
	}
	if got.MediaType != "image" {
		t.Errorf("MediaType = %q, want \"image\"", got.MediaType)
	}
}

func TestFetchAPOD_Array(t *testing.T) {
	apod := APODResponse{
		Date:      "2020-03-14",
		Title:     "Random Galaxy",
		HDURL:     "https://example.com/hd.jpg",
		URL:       "https://example.com/sd.jpg",
		MediaType: "image",
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]APODResponse{apod})
	}))
	defer srv.Close()

	got, err := fetchAPOD(srv.URL)
	if err != nil {
		t.Fatalf("fetchAPOD() error: %v", err)
	}
	if got.Title != apod.Title {
		t.Errorf("Title = %q, want %q", got.Title, apod.Title)
	}
}

func TestFetchAPOD_APIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":{"code":"API_KEY_INVALID"}}`))
	}))
	defer srv.Close()

	_, err := fetchAPOD(srv.URL)
	if err == nil {
		t.Fatal("expected error for 403 response")
	}
}

func TestDownloadImage(t *testing.T) {
	content := []byte("fake image data")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(content)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")

	if err := downloadImage(srv.URL, path); err != nil {
		t.Fatalf("downloadImage() error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading downloaded file: %v", err)
	}
	if string(data) != string(content) {
		t.Errorf("file content = %q, want %q", data, content)
	}
}

func TestDownloadImage_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	dir := t.TempDir()
	path := filepath.Join(dir, "test.jpg")

	if err := downloadImage(srv.URL, path); err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestPrintDetails(t *testing.T) {
	// Smoke test: printDetails should not panic.
	apod := APODResponse{
		Date:        "2024-09-27",
		Title:       "Test Image",
		Explanation: "A test explanation.",
		MediaType:   "image",
	}
	printDetails(&apod, "/tmp/test.jpg")
}
