package main

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// Dated requests can use local metadata offline; random requests ask NASA to
// choose an APOD before reusing its full-image cache.
func resolveWallpaperAPOD(db *sql.DB, apiKey string, random bool, date string, now time.Time) (APODResponse, error) {
	if !random {
		if date == "" {
			date = now.UTC().Format("2006-01-02")
		}
		record, err := recordByDate(db, date)
		if err == nil {
			return apodResponseFromRecord(record), nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return APODResponse{}, err
		}
	}
	apod, err := fetchAPOD(buildAPODURL(apiKey, random, date))
	if err != nil {
		return APODResponse{}, err
	}
	err = upsertAPOD(db, APODRecord{
		Date: apod.Date, Title: apod.Title, Description: apod.Explanation,
		MediaType: apod.MediaType, URL: apod.URL, HDURL: apod.HDURL,
		ThumbnailURL: apod.ThumbnailURL, Copyright: apod.Copyright, FetchedAt: now,
	})
	return apod, err
}

func ensureHDImageCached(db *sql.DB, paths AppPaths, record APODRecord, apiKey string) (string, error) {
	storedRecord, err := recordByDate(db, record.Date)
	if err == nil {
		record = storedRecord
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	if record.MediaType != "" && record.MediaType != "image" {
		return "", fmt.Errorf("%s is a %s, not an image", record.Date, record.MediaType)
	}
	if record.HDPath != "" {
		cached, err := usableCachedImage(record.HDPath)
		if err != nil {
			return "", err
		}
		if cached {
			return record.HDPath, nil
		}
	}
	if record.MediaType == "" || preferredMediaURL(record) == "" {
		apod, err := fetchAPOD(buildAPODURL(apiKey, false, record.Date))
		if err != nil {
			return "", err
		}
		record.MediaType, record.URL, record.HDURL = apod.MediaType, apod.URL, apod.HDURL
	}
	if record.MediaType != "image" {
		return "", fmt.Errorf("%s is a %s, not an image", record.Date, record.MediaType)
	}
	imageURL := preferredMediaURL(record)
	if imageURL == "" {
		return "", fmt.Errorf("no downloadable image URL for %s", record.Date)
	}
	fullPath := filepath.Join(paths.FullDir, record.Date+fileExtensionFromURL(imageURL))
	cached, err := usableCachedImage(fullPath)
	if err != nil {
		return "", err
	}
	if !cached {
		if err := downloadImage(imageURL, fullPath); err != nil {
			return "", err
		}
	}
	if err := updateHDPath(db, record.Date, fullPath); err != nil {
		return "", err
	}
	return fullPath, nil
}

func usableCachedImage(path string) (bool, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, localImageError{err: fmt.Errorf("checking image cache: %w", err)}
	}
	if !info.Mode().IsRegular() {
		return false, localImageError{err: fmt.Errorf("image cache path is not a regular file: %s", path)}
	}
	return info.Size() > 0, nil
}

// Install the cached image at the CLI output path only after the copy succeeds.
func copyImageAtomic(sourcePath, targetPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("opening cached image: %w", err)
	}
	defer source.Close()
	sourceInfo, err := source.Stat()
	if err != nil {
		return err
	}
	if targetInfo, err := os.Stat(targetPath); err == nil && os.SameFile(sourceInfo, targetInfo) {
		return nil
	}
	temp, err := os.CreateTemp(filepath.Dir(targetPath), ".astrowall-copy-*")
	if err != nil {
		return fmt.Errorf("creating temporary image: %w", err)
	}
	defer os.Remove(temp.Name())
	if _, err := io.Copy(temp, source); err != nil {
		temp.Close()
		return fmt.Errorf("copying cached image: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("closing image: %w", err)
	}
	if err := os.Rename(temp.Name(), targetPath); err != nil {
		return fmt.Errorf("installing image: %w", err)
	}
	return nil
}
