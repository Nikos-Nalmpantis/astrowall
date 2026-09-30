package main

import (
	"database/sql"
	"fmt"
	"time"
)

// A history write failure must not report that an already-applied wallpaper
// failed. Callers display the warning separately from application errors.
func applyRecordedWallpaper(db *sql.DB, date, imagePath string) (historyErr, applyErr error) {
	if err := wallpaperSetterFunc(imagePath); err != nil {
		return nil, err
	}
	return recordWallpaperApplication(db, date, imagePath, time.Now()), nil
}

func recordWallpaperApplication(db *sql.DB, date, imagePath string, appliedAt time.Time) error {
	_, err := db.Exec(`INSERT INTO wallpaper_history (apod_date, image_path, applied_at) VALUES (?, ?, ?)`, date, imagePath, appliedAt.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return fmt.Errorf("saving wallpaper history: %w", err)
	}
	return nil
}

// Keep all application events, but browse each APOD once in latest-use order.
// IDs break timestamp ties and retain application order if the clock changes.
func listWallpaperHistory(db *sql.DB) ([]APODRecord, error) {
	rows, err := db.Query(`
		SELECT a.date, a.title, a.description, a.media_type, a.url, a.hd_url,
		       a.thumbnail_url, a.copyright, a.preview_path, a.preview_error,
		       a.hd_path, a.favorite, a.fetched_at, h.applied_at
		FROM wallpaper_history h
		JOIN (SELECT MAX(id) AS id FROM wallpaper_history GROUP BY apod_date) latest ON latest.id = h.id
		JOIN apods a ON a.date = h.apod_date
		ORDER BY h.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("listing wallpaper history: %w", err)
	}
	defer rows.Close()
	var records []APODRecord
	for rows.Next() {
		var record APODRecord
		var fetchedAt, appliedAt string
		var favorite int
		if err := rows.Scan(
			&record.Date, &record.Title, &record.Description, &record.MediaType,
			&record.URL, &record.HDURL, &record.ThumbnailURL, &record.Copyright,
			&record.PreviewPath, &record.PreviewError, &record.HDPath, &favorite,
			&fetchedAt, &appliedAt,
		); err != nil {
			return nil, err
		}
		record.Favorite = favorite == 1
		record.FetchedAt, err = time.Parse(time.RFC3339, fetchedAt)
		if err != nil {
			return nil, err
		}
		record.LastAppliedAt, err = time.Parse(time.RFC3339Nano, appliedAt)
		if err != nil {
			return nil, fmt.Errorf("reading wallpaper history time: %w", err)
		}
		records = append(records, record)
	}
	return records, rows.Err()
}

func lastAppliedWallpaperDate(db *sql.DB) (string, error) {
	var date string
	err := db.QueryRow(`SELECT apod_date FROM wallpaper_history ORDER BY id DESC LIMIT 1`).Scan(&date)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return date, err
}
