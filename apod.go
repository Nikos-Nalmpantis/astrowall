package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"time"
)

var httpClient = &http.Client{
	Timeout: 30 * time.Second,
}

const userAgent = "astrowall/1.0"

var apodAPIBaseURL = "https://api.nasa.gov/planetary/apod"

func httpGet(url string) (*http.Response, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	return httpClient.Do(req)
}

// APODResponse represents the JSON response from NASA's APOD API.
type APODResponse struct {
	Copyright      string `json:"copyright"`
	Date           string `json:"date"`
	Explanation    string `json:"explanation"`
	HDURL          string `json:"hdurl"`
	MediaType      string `json:"media_type"`
	ServiceVersion string `json:"service_version"`
	ThumbnailURL   string `json:"thumbnail_url"`
	Title          string `json:"title"`
	URL            string `json:"url"`
}

func buildAPODURL(apiKey string, random bool, date string) string {
	base := apodAPIBaseURL + "?api_key=" + apiKey
	if random {
		return base + "&count=1"
	}
	if date != "" {
		return base + "&date=" + date
	}
	return base
}

func buildAPODRangeURL(apiKey, startDate, endDate string) string {
	values := url.Values{}
	values.Set("api_key", apiKey)
	values.Set("start_date", startDate)
	values.Set("end_date", endDate)
	values.Set("thumbs", "true")
	return apodAPIBaseURL + "?" + values.Encode()
}

func fetchAPOD(url string) (APODResponse, error) {
	resp, err := httpGet(url)
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

func fetchAPODRange(url string) ([]APODResponse, error) {
	resp, err := httpGet(url)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned %d: %s", resp.StatusCode, string(body))
	}

	var arr []APODResponse
	if err := json.Unmarshal(body, &arr); err == nil {
		return arr, nil
	}

	var apod APODResponse
	if err := json.Unmarshal(body, &apod); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}
	return []APODResponse{apod}, nil
}

func downloadImage(url, path string) error {
	resp, err := httpGet(url)
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

func preferredPreviewURL(apod APODResponse) string {
	if apod.MediaType == "image" && apod.URL != "" {
		return apod.URL
	}
	if apod.ThumbnailURL != "" {
		return apod.ThumbnailURL
	}
	if apod.URL != "" {
		return apod.URL
	}
	return ""
}

func fileExtensionFromURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ".jpg"
	}

	ext := path.Ext(parsed.Path)
	if ext == "" {
		return ".jpg"
	}
	return ext
}
