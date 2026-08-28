package main

import (
	"fmt"
	"os/exec"
	"runtime"
)

var openURLFunc = openURL

func openURL(url string) error {
	if url == "" {
		return fmt.Errorf("url is empty")
	}

	command, args := openURLCommand(url)
	if command == "" {
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}

	if err := exec.Command(command, args...).Run(); err != nil {
		return fmt.Errorf("opening URL %s: %w", url, err)
	}
	return nil
}

func openURLCommand(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}
