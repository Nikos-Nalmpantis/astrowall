package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type APODRecord struct {
	Date         string
	Title        string
	Description  string
	MediaType    string
	URL          string
	HDURL        string
	ThumbnailURL string
	Copyright    string
	PreviewPath  string
	HDPath       string
	Favorite     bool
	FetchedAt    time.Time
}

func openLibrary(dbPath string) (*sql.DB, error) {
	dsn := dbPath + "?_journal_mode=WAL&_busy_timeout=5000&_synchronous=NORMAL&_foreign_keys=ON"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening sqlite database: %w", err)
	}

	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)

	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("connecting to sqlite database: %w", err)
	}

	if err := initLibrary(db); err != nil {
		db.Close()
		return nil, err
	}

	return db, nil
}

func initLibrary(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS apods (
	date TEXT PRIMARY KEY,
	title TEXT NOT NULL,
	description TEXT NOT NULL,
	media_type TEXT NOT NULL,
	url TEXT NOT NULL,
	hd_url TEXT NOT NULL DEFAULT '',
	thumbnail_url TEXT NOT NULL DEFAULT '',
	copyright TEXT NOT NULL DEFAULT '',
	preview_path TEXT NOT NULL DEFAULT '',
	hd_path TEXT NOT NULL DEFAULT '',
	favorite INTEGER NOT NULL DEFAULT 0,
	fetched_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS app_state (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_apods_fetched_at ON apods(fetched_at DESC);
CREATE INDEX IF NOT EXISTS idx_apods_favorite ON apods(favorite, date DESC);
`

	if _, err := db.Exec(schema); err != nil {
		return fmt.Errorf("initializing sqlite schema: %w", err)
	}
	return nil
}

func latestStoredDate(db *sql.DB) (string, error) {
	var latest sql.NullString
	if err := db.QueryRow(`SELECT MAX(date) FROM apods`).Scan(&latest); err != nil {
		return "", fmt.Errorf("querying latest stored date: %w", err)
	}
	if !latest.Valid {
		return "", nil
	}
	return latest.String, nil
}

func upsertAPOD(db *sql.DB, record APODRecord) error {
	const query = `
INSERT INTO apods (
	date, title, description, media_type, url, hd_url, thumbnail_url, copyright,
	preview_path, hd_path, favorite, fetched_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(date) DO UPDATE SET
	title = excluded.title,
	description = excluded.description,
	media_type = excluded.media_type,
	url = excluded.url,
	hd_url = excluded.hd_url,
	thumbnail_url = excluded.thumbnail_url,
	copyright = excluded.copyright,
	fetched_at = excluded.fetched_at
`

	_, err := db.Exec(
		query,
		record.Date,
		record.Title,
		record.Description,
		record.MediaType,
		record.URL,
		record.HDURL,
		record.ThumbnailURL,
		record.Copyright,
		record.PreviewPath,
		record.HDPath,
		boolToInt(record.Favorite),
		record.FetchedAt.UTC().Format(time.RFC3339),
	)
	if err != nil {
		return fmt.Errorf("upserting APOD %s: %w", record.Date, err)
	}
	return nil
}

func updatePreviewPath(db *sql.DB, date, previewPath string) error {
	_, err := db.Exec(`UPDATE apods SET preview_path = ? WHERE date = ?`, previewPath, date)
	if err != nil {
		return fmt.Errorf("updating preview path for %s: %w", date, err)
	}
	return nil
}

func updateHDPath(db *sql.DB, date, hdPath string) error {
	_, err := db.Exec(`UPDATE apods SET hd_path = ? WHERE date = ?`, hdPath, date)
	if err != nil {
		return fmt.Errorf("updating HD path for %s: %w", date, err)
	}
	return nil
}

func toggleFavorite(db *sql.DB, date string) (bool, error) {
	_, err := db.Exec(`UPDATE apods SET favorite = CASE favorite WHEN 0 THEN 1 ELSE 0 END WHERE date = ?`, date)
	if err != nil {
		return false, fmt.Errorf("toggling favorite for %s: %w", date, err)
	}

	var favorite int
	if err := db.QueryRow(`SELECT favorite FROM apods WHERE date = ?`, date).Scan(&favorite); err != nil {
		return false, fmt.Errorf("querying favorite state for %s: %w", date, err)
	}
	return favorite == 1, nil
}

func apodCount(db *sql.DB) (int, error) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM apods`).Scan(&count); err != nil {
		return 0, fmt.Errorf("counting APOD rows: %w", err)
	}
	return count, nil
}

func recordByDate(db *sql.DB, date string) (APODRecord, error) {
	var record APODRecord
	var fetchedAt string
	var favorite int

	err := db.QueryRow(`
		SELECT date, title, description, media_type, url, hd_url, thumbnail_url, copyright,
		       preview_path, hd_path, favorite, fetched_at
		FROM apods
		WHERE date = ?
	`, date).Scan(
		&record.Date,
		&record.Title,
		&record.Description,
		&record.MediaType,
		&record.URL,
		&record.HDURL,
		&record.ThumbnailURL,
		&record.Copyright,
		&record.PreviewPath,
		&record.HDPath,
		&favorite,
		&fetchedAt,
	)
	if err != nil {
		return APODRecord{}, fmt.Errorf("querying APOD %s: %w", date, err)
	}

	parsed, err := time.Parse(time.RFC3339, fetchedAt)
	if err != nil {
		return APODRecord{}, fmt.Errorf("parsing fetched_at for %s: %w", date, err)
	}
	record.FetchedAt = parsed
	record.Favorite = favorite == 1
	return record, nil
}

func listRecentAPODs(db *sql.DB, limit int) ([]APODRecord, error) {
	if limit <= 0 {
		limit = 30
	}

	rows, err := db.Query(`
		SELECT date, title, description, media_type, url, hd_url, thumbnail_url, copyright,
		       preview_path, hd_path, favorite, fetched_at
		FROM apods
		ORDER BY date DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, fmt.Errorf("listing recent APODs: %w", err)
	}
	defer rows.Close()

	var records []APODRecord
	for rows.Next() {
		var record APODRecord
		var fetchedAt string
		var favorite int

		if err := rows.Scan(
			&record.Date,
			&record.Title,
			&record.Description,
			&record.MediaType,
			&record.URL,
			&record.HDURL,
			&record.ThumbnailURL,
			&record.Copyright,
			&record.PreviewPath,
			&record.HDPath,
			&favorite,
			&fetchedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning recent APODs: %w", err)
		}

		parsed, err := time.Parse(time.RFC3339, fetchedAt)
		if err != nil {
			return nil, fmt.Errorf("parsing fetched_at for %s: %w", record.Date, err)
		}

		record.FetchedAt = parsed
		record.Favorite = favorite == 1
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating recent APODs: %w", err)
	}

	return records, nil
}

func listFavoriteAPODs(db *sql.DB) ([]APODRecord, error) {
	rows, err := db.Query(`
		SELECT date, title, description, media_type, url, hd_url, thumbnail_url, copyright,
		       preview_path, hd_path, favorite, fetched_at
		FROM apods
		WHERE favorite = 1
		ORDER BY date DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("listing favorite APODs: %w", err)
	}
	defer rows.Close()

	var records []APODRecord
	for rows.Next() {
		var record APODRecord
		var fetchedAt string
		var favorite int

		if err := rows.Scan(
			&record.Date,
			&record.Title,
			&record.Description,
			&record.MediaType,
			&record.URL,
			&record.HDURL,
			&record.ThumbnailURL,
			&record.Copyright,
			&record.PreviewPath,
			&record.HDPath,
			&favorite,
			&fetchedAt,
		); err != nil {
			return nil, fmt.Errorf("scanning favorite APODs: %w", err)
		}

		parsed, err := time.Parse(time.RFC3339, fetchedAt)
		if err != nil {
			return nil, fmt.Errorf("parsing favorite fetched_at for %s: %w", record.Date, err)
		}

		record.FetchedAt = parsed
		record.Favorite = favorite == 1
		records = append(records, record)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating favorite APODs: %w", err)
	}

	return records, nil
}

func getStateValue(db *sql.DB, key string) (string, error) {
	var value sql.NullString
	err := db.QueryRow(`SELECT value FROM app_state WHERE key = ?`, key).Scan(&value)
	if err != nil {
		if err == sql.ErrNoRows {
			return "", nil
		}
		return "", fmt.Errorf("querying app state %s: %w", key, err)
	}
	if !value.Valid {
		return "", nil
	}
	return value.String, nil
}

func setStateValue(db *sql.DB, key, value string) error {
	_, err := db.Exec(`
		INSERT INTO app_state (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value
	`, key, value)
	if err != nil {
		return fmt.Errorf("saving app state %s: %w", key, err)
	}
	return nil
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
