package main

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type SyncResult struct {
	FetchedCount    int
	PreviewedCount  int
	StartDate       string
	EndDate         string
	AlreadyUpToDate bool
}

func syncAPODArchive(db *sql.DB, paths AppPaths, apiKey string, now time.Time) (SyncResult, error) {
	latestDate, err := latestStoredDate(db)
	if err != nil {
		return SyncResult{}, err
	}

	startDate, endDate, shouldSync, err := determineSyncRange(now, latestDate)
	if err != nil {
		return SyncResult{}, err
	}
	if !shouldSync {
		return SyncResult{StartDate: startDate, EndDate: endDate, AlreadyUpToDate: true}, nil
	}

	items, err := fetchAPODRange(buildAPODRangeURL(apiKey, startDate, endDate))
	if err != nil {
		return SyncResult{}, err
	}

	sort.Slice(items, func(i, j int) bool {
		return items[i].Date < items[j].Date
	})

	result := SyncResult{StartDate: startDate, EndDate: endDate, FetchedCount: len(items)}
	for _, item := range items {
		record := APODRecord{
			Date:         item.Date,
			Title:        item.Title,
			Description:  item.Explanation,
			MediaType:    item.MediaType,
			URL:          item.URL,
			HDURL:        item.HDURL,
			ThumbnailURL: item.ThumbnailURL,
			Copyright:    item.Copyright,
			FetchedAt:    now.UTC(),
		}
		if err := upsertAPOD(db, record); err != nil {
			return SyncResult{}, err
		}

		previewURL := preferredPreviewURL(item)
		if previewURL == "" {
			continue
		}

		previewPath := filepath.Join(paths.PreviewDir, item.Date+fileExtensionFromURL(previewURL))
		if _, err := os.Stat(previewPath); err != nil {
			if !os.IsNotExist(err) {
				return SyncResult{}, fmt.Errorf("checking preview cache for %s: %w", item.Date, err)
			}
			if err := downloadImage(previewURL, previewPath); err != nil {
				return SyncResult{}, fmt.Errorf("downloading preview for %s: %w", item.Date, err)
			}
			result.PreviewedCount++
		}

		if err := updatePreviewPath(db, item.Date, previewPath); err != nil {
			return SyncResult{}, err
		}
	}

	return result, nil
}

func determineSyncRange(now time.Time, latestDate string) (startDate, endDate string, shouldSync bool, err error) {
	today := now.UTC().Format("2006-01-02")
	endDate = today

	if latestDate == "" {
		return now.UTC().AddDate(0, 0, -29).Format("2006-01-02"), endDate, true, nil
	}

	latest, err := time.Parse("2006-01-02", latestDate)
	if err != nil {
		return "", "", false, fmt.Errorf("parsing latest stored date %q: %w", latestDate, err)
	}

	next := latest.AddDate(0, 0, 1)
	startDate = next.Format("2006-01-02")
	if startDate > endDate {
		return startDate, endDate, false, nil
	}
	return startDate, endDate, true, nil
}
