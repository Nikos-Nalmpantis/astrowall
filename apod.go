package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

// APODResponse represents the JSON response from NASA's APOD API.
type APODResponse struct {
	Date           string `json:"date"`
	Explanation    string `json:"explanation"`
	HDURL          string `json:"hdurl"`
	MediaType      string `json:"media_type"`
	ServiceVersion string `json:"service_version"`
	Title          string `json:"title"`
	URL            string `json:"url"`
}

func buildAPODURL(apiKey string, random bool, date string) string {
	base := "https://api.nasa.gov/planetary/apod?api_key=" + apiKey
	if random {
		return base + "&count=1"
	}
	if date != "" {
		return base + "&date=" + date
	}
	return base
}

func fetchAPOD(url string) (APODResponse, error) {
	resp, err := http.Get(url)
	if err != nil {
		return APODResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return APODResponse{}, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return APODResponse{}, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	// Random mode returns a JSON array; normal mode returns a single object.
	var arr []APODResponse
	if err := json.Unmarshal(body, &arr); err == nil && len(arr) > 0 {
		return arr[0], nil
	}

	var apod APODResponse
	if err := json.Unmarshal(body, &apod); err != nil {
		return APODResponse{}, fmt.Errorf("parsing response: %w", err)
	}
	return apod, nil
}

func downloadImage(url, path string) error {
	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned %d", resp.StatusCode)
	}

	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating file: %w", err)
	}
	defer file.Close()

	if _, err := io.Copy(file, resp.Body); err != nil {
		return fmt.Errorf("saving image: %w", err)
	}
	return nil
}

func resolveImagePath(output string) (string, error) {
	if output != "" {
		dir := filepath.Dir(output)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return "", fmt.Errorf("creating directory %s: %w", dir, err)
		}
		return output, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "apod_wallpaper.jpg", nil
	}

	picturesDir := filepath.Join(home, "Pictures")
	if err := os.MkdirAll(picturesDir, 0o755); err != nil {
		return filepath.Join(home, "apod_wallpaper.jpg"), nil
	}

	return filepath.Join(picturesDir, "apod_wallpaper.jpg"), nil
}
