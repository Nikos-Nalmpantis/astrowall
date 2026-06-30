package main

import (
	"database/sql"
	"fmt"
)

const lastCycledFavoriteKey = "last_cycled_favorite_date"

type FavoriteCycleResult struct {
	Date      string
	Title     string
	ImagePath string
}

var wallpaperSetterFunc = setWallpaper
var ensureHDImageCachedFunc = ensureHDImageCached

func cycleFavoriteWallpaper(db *sql.DB, paths AppPaths, apiKey string) (FavoriteCycleResult, error) {
	favorites, err := listFavoriteAPODs(db)
	if err != nil {
		return FavoriteCycleResult{}, err
	}
	if len(favorites) == 0 {
		return FavoriteCycleResult{}, fmt.Errorf("no favorite wallpapers available")
	}

	lastDate, err := getStateValue(db, lastCycledFavoriteKey)
	if err != nil {
		return FavoriteCycleResult{}, err
	}
	next := nextFavoriteRecord(favorites, lastDate)

	imagePath, err := ensureHDImageCachedFunc(db, paths, next, apiKey)
	if err != nil {
		return FavoriteCycleResult{}, err
	}
	if err := wallpaperSetterFunc(imagePath); err != nil {
		return FavoriteCycleResult{}, err
	}
	if err := setStateValue(db, lastCycledFavoriteKey, next.Date); err != nil {
		return FavoriteCycleResult{}, err
	}

	return FavoriteCycleResult{Date: next.Date, Title: next.Title, ImagePath: imagePath}, nil
}

func nextFavoriteRecord(records []APODRecord, lastDate string) APODRecord {
	if len(records) == 0 {
		return APODRecord{}
	}
	if lastDate == "" {
		return records[0]
	}
	for i := range records {
		if records[i].Date == lastDate {
			return records[(i+1)%len(records)]
		}
	}
	return records[0]
}
