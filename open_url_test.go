package main

import (
	"runtime"
	"slices"
	"testing"
)

func TestOpenURLCommand(t *testing.T) {
	command, args := openURLCommand("https://example.com")

	var wantCommand string
	var wantArgs []string
	switch runtime.GOOS {
	case "darwin":
		wantCommand = "open"
		wantArgs = []string{"https://example.com"}
	case "windows":
		wantCommand = "rundll32"
		wantArgs = []string{"url.dll,FileProtocolHandler", "https://example.com"}
	default:
		wantCommand = "xdg-open"
		wantArgs = []string{"https://example.com"}
	}

	if command != wantCommand {
		t.Fatalf("openURLCommand() command = %q, want %q", command, wantCommand)
	}
	if !slices.Equal(args, wantArgs) {
		t.Fatalf("openURLCommand() args = %q, want %q", args, wantArgs)
	}
}

func TestOpenURLRejectsEmptyURL(t *testing.T) {
	if err := openURL(""); err == nil {
		t.Fatal("openURL() error = nil, want an error")
	}
}
