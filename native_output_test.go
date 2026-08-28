package main

import (
	"bytes"
	"io"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestNativeImageOutputPlacesImageAfterEveryFrameWrite(t *testing.T) {
	var buffer bytes.Buffer
	output := newNativeImageOutput(&buffer)
	output.setPlacement("<placement>")

	if _, err := output.Write([]byte("frame-one")); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if _, err := output.Write([]byte("frame-two")); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if got := buffer.String(); got != "frame-one<placement>frame-two<placement>" {
		t.Fatalf("output = %q", got)
	}
}

func TestNativeImageOutputDoesNotPlaceAfterAltScreenExit(t *testing.T) {
	var buffer bytes.Buffer
	output := newNativeImageOutput(&buffer)
	output.setPlacement("<placement>")

	if _, err := output.Write([]byte(ansi.ResetModeAltScreenSaveCursor)); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if got := buffer.String(); got != ansi.ResetModeAltScreenSaveCursor {
		t.Fatalf("output = %q", got)
	}
}

type shortWriter struct{}

func (shortWriter) Write(data []byte) (int, error) {
	return len(data) - 1, nil
}

func TestNativeImageOutputPropagatesShortWrites(t *testing.T) {
	output := newNativeImageOutput(shortWriter{})
	output.setPlacement("<placement>")
	if _, err := output.Write([]byte("frame")); err != io.ErrShortWrite {
		t.Fatalf("Write() error = %v, want io.ErrShortWrite", err)
	}
}

func TestNativeImageOutputStopsPlacementImmediately(t *testing.T) {
	var buffer bytes.Buffer
	output := newNativeImageOutput(&buffer)
	output.setPlacement("<placement>")
	output.setPlacement("")

	if _, err := output.Write([]byte("frame")); err != nil {
		t.Fatalf("Write() error: %v", err)
	}
	if got := buffer.String(); got != "frame" {
		t.Fatalf("output = %q", got)
	}
}
