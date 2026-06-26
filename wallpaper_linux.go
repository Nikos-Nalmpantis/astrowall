//go:build linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func setWallpaper(imagePath string) error {
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}

	de := os.Getenv("XDG_CURRENT_DESKTOP")

	// Some DEs report compound values like "ubuntu:GNOME"
	for _, part := range strings.Split(de, ":") {
		switch part {
		case "GNOME", "Unity", "Pantheon", "Budgie":
			return setWallpaperGNOME(absPath)
		case "KDE":
			return setWallpaperKDE(absPath)
		case "Hyprland":
			return setWallpaperHyprland(absPath)
		case "sway":
			return setWallpaperSway(absPath)
		case "XFCE":
			return setWallpaperXFCE(absPath)
		case "X-Cinnamon":
			return setWallpaperCinnamon(absPath)
		case "MATE":
			return setWallpaperMATE(absPath)
		}
	}

	// Fallback: try GNOME gsettings since many DEs are GNOME-based.
	if err := setWallpaperGNOME(absPath); err == nil {
		return nil
	}

	return fmt.Errorf("unsupported desktop environment: %q (set XDG_CURRENT_DESKTOP if not detected)", de)
}

func setWallpaperGNOME(absPath string) error {
	uri := "file://" + absPath
	for _, key := range []string{"picture-uri", "picture-uri-dark"} {
		if err := exec.Command("gsettings", "set", "org.gnome.desktop.background", key, uri).Run(); err != nil {
			return fmt.Errorf("gsettings set %s: %w", key, err)
		}
	}
	return nil
}

func setWallpaperKDE(absPath string) error {
	if err := exec.Command("plasma-apply-wallpaperimage", absPath).Run(); err != nil {
		return fmt.Errorf("plasma-apply-wallpaperimage: %w", err)
	}
	return nil
}

func setWallpaperHyprland(absPath string) error {
	if err := exec.Command("swww", "img", absPath).Run(); err != nil {
		return fmt.Errorf("swww img: %w", err)
	}
	return nil
}

func setWallpaperSway(absPath string) error {
	if err := exec.Command("swaymsg", "output", "*", "bg", absPath, "fill").Run(); err != nil {
		return fmt.Errorf("swaymsg: %w", err)
	}
	return nil
}

func setWallpaperXFCE(absPath string) error {
	// Discover all backdrop last-image properties and set them all.
	out, err := exec.Command("xfconf-query", "-c", "xfce4-desktop", "-l").Output()
	if err != nil {
		return fmt.Errorf("xfconf-query list: %w", err)
	}

	set := false
	for _, line := range strings.Split(string(out), "\n") {
		prop := strings.TrimSpace(line)
		if strings.HasSuffix(prop, "/last-image") {
			if err := exec.Command("xfconf-query", "-c", "xfce4-desktop", "-p", prop, "-s", absPath).Run(); err == nil {
				set = true
			}
		}
	}
	if !set {
		return fmt.Errorf("xfconf-query: no backdrop properties found")
	}
	return nil
}

func setWallpaperCinnamon(absPath string) error {
	uri := "file://" + absPath
	if err := exec.Command("gsettings", "set", "org.cinnamon.desktop.background", "picture-uri", uri).Run(); err != nil {
		return fmt.Errorf("gsettings cinnamon: %w", err)
	}
	return nil
}

func setWallpaperMATE(absPath string) error {
	if err := exec.Command("gsettings", "set", "org.mate.background", "picture-filename", absPath).Run(); err != nil {
		return fmt.Errorf("gsettings mate: %w", err)
	}
	return nil
}
