//go:build darwin

package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
)

func setWallpaper(imagePath string) error {
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}

	script := fmt.Sprintf(
		`tell application "System Events" to set picture of every desktop to "%s"`,
		absPath,
	)
	if err := exec.Command("osascript", "-e", script).Run(); err != nil {
		return fmt.Errorf("osascript: %w", err)
	}
	return nil
}
