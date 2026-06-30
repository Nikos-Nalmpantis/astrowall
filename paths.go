package main

import (
	"fmt"
	"os"
	"path/filepath"
)

const appDirName = "astrowall"

type AppPaths struct {
	DataDir    string
	CacheDir   string
	PreviewDir string
	FullDir    string
	DBPath     string
}

func resolveAppPaths() (AppPaths, error) {
	dataRoot, err := resolveDataRoot()
	if err != nil {
		return AppPaths{}, err
	}

	cacheRoot, err := resolveCacheRoot()
	if err != nil {
		return AppPaths{}, err
	}

	paths := AppPaths{
		DataDir:    filepath.Join(dataRoot, appDirName),
		CacheDir:   filepath.Join(cacheRoot, appDirName),
		PreviewDir: filepath.Join(cacheRoot, appDirName, "previews"),
		FullDir:    filepath.Join(cacheRoot, appDirName, "full"),
	}
	paths.DBPath = filepath.Join(paths.DataDir, "astrowall.db")

	for _, dir := range []string{paths.DataDir, paths.CacheDir, paths.PreviewDir, paths.FullDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return AppPaths{}, fmt.Errorf("creating directory %s: %w", dir, err)
		}
	}

	return paths, nil
}

func resolveDataRoot() (string, error) {
	if value := os.Getenv("XDG_DATA_HOME"); value != "" {
		return value, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".local", "share"), nil
}

func resolveCacheRoot() (string, error) {
	if value := os.Getenv("XDG_CACHE_HOME"); value != "" {
		return value, nil
	}

	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolving home directory: %w", err)
	}
	return filepath.Join(home, ".cache"), nil
}
