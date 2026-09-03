package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type SyncResult struct {
	FetchedCount       int
	PreviewedCount     int
	PreviewFailedCount int
	StartDate          string
	EndDate            string
	AlreadyUpToDate    bool
}

type itemSyncResult struct {
	Previewed    bool
	PreviewError string
}

type archiveSyncPlan struct {
	Items           []APODResponse
	StartDate       string
	EndDate         string
	AlreadyUpToDate bool
}

func prepareArchiveSync(db *sql.DB, apiKey string, now time.Time) (archiveSyncPlan, error) {
	return prepareArchiveSyncContext(context.Background(), db, apiKey, now)
}

func prepareArchiveSyncContext(ctx context.Context, db *sql.DB, apiKey string, now time.Time) (archiveSyncPlan, error) {
	latestDate, err := latestStoredDate(db)
	if err != nil {
		return archiveSyncPlan{}, err
	}

	startDate, endDate, shouldSync, err := determineSyncRange(now, latestDate)
	if err != nil {
		return archiveSyncPlan{}, err
	}
	plan := archiveSyncPlan{StartDate: startDate, EndDate: endDate}
	failedPreviews, err := listAPODsWithPreviewErrors(db)
	if err != nil {
		return archiveSyncPlan{}, err
	}
	for _, record := range failedPreviews {
		plan.Items = append(plan.Items, APODResponse{
			Date: record.Date, Title: record.Title, Explanation: record.Description,
			MediaType: record.MediaType, URL: record.URL, HDURL: record.HDURL,
			ThumbnailURL: record.ThumbnailURL, Copyright: record.Copyright,
		})
	}
	if shouldSync {
		items, err := fetchAPODRangeContext(ctx, buildAPODRangeURL(apiKey, startDate, endDate))
		if err != nil {
			return archiveSyncPlan{}, err
		}
		plan.Items = append(plan.Items, items...)
	}
	plan.AlreadyUpToDate = len(plan.Items) == 0
	sort.Slice(plan.Items, func(i, j int) bool {
		return plan.Items[i].Date < plan.Items[j].Date
	})
	if len(plan.Items) > 0 {
		plan.StartDate = plan.Items[0].Date
		plan.EndDate = plan.Items[len(plan.Items)-1].Date
	}
	return plan, nil
}

func syncAPODArchive(db *sql.DB, paths AppPaths, apiKey string, now time.Time) (SyncResult, error) {
	plan, err := prepareArchiveSync(db, apiKey, now)
	if err != nil {
		return SyncResult{}, err
	}
	if plan.AlreadyUpToDate {
		return SyncResult{StartDate: plan.StartDate, EndDate: plan.EndDate, AlreadyUpToDate: true}, nil
	}

	result := SyncResult{StartDate: plan.StartDate, EndDate: plan.EndDate, FetchedCount: len(plan.Items)}
	for _, item := range plan.Items {
		itemResult, err := syncAPODItem(db, paths, item, now)
		if err != nil {
			return SyncResult{}, err
		}
		if itemResult.Previewed {
			result.PreviewedCount++
		}
		if itemResult.PreviewError != "" {
			result.PreviewFailedCount++
		}
	}

	return result, nil
}

func syncAPODItem(db *sql.DB, paths AppPaths, item APODResponse, now time.Time) (itemSyncResult, error) {
	return syncAPODItemContext(context.Background(), db, paths, item, now)
}

func syncAPODItemContext(ctx context.Context, db *sql.DB, paths AppPaths, item APODResponse, now time.Time) (itemSyncResult, error) {
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
	previewURL := preferredPreviewURL(item)
	if previewURL == "" {
		return itemSyncResult{}, upsertAPOD(db, record)
	}

	result := itemSyncResult{}
	previewPath := filepath.Join(paths.PreviewDir, item.Date+fileExtensionFromURL(previewURL))
	if _, err := os.Stat(previewPath); err != nil {
		if !os.IsNotExist(err) {
			return itemSyncResult{}, fmt.Errorf("checking preview cache for %s: %w", item.Date, err)
		} else if err := downloadImageAtomicContext(ctx, previewURL, previewPath); err != nil {
			if ctx.Err() != nil {
				return itemSyncResult{}, fmt.Errorf("downloading preview for %s: %w", item.Date, err)
			}
			var localErr localImageError
			if errors.As(err, &localErr) {
				return itemSyncResult{}, fmt.Errorf("downloading preview for %s: %w", item.Date, err)
			}
			result.PreviewError = fmt.Sprintf("downloading preview: %v", err)
		} else {
			result.Previewed = true
		}
	}

	if result.PreviewError == "" {
		record.PreviewPath = previewPath
	} else {
		record.PreviewError = result.PreviewError
	}
	return result, upsertAPOD(db, record)
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
