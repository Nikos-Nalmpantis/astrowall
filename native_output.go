package main

import (
	"bytes"
	"io"
	"os"
	"sync"

	"github.com/charmbracelet/x/ansi"
)

type nativeImageOutput struct {
	writer    io.Writer
	mu        sync.Mutex
	placement string
}

func newNativeImageOutput(writer io.Writer) *nativeImageOutput {
	return &nativeImageOutput{writer: writer}
}

func (o *nativeImageOutput) setPlacement(placement string) {
	o.mu.Lock()
	o.placement = placement
	o.mu.Unlock()
}

func (o *nativeImageOutput) Write(data []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	written, err := o.writer.Write(data)
	if err != nil {
		return written, err
	}
	if written != len(data) {
		return written, io.ErrShortWrite
	}
	if o.placement != "" && !bytes.Contains(data, []byte(ansi.ResetModeAltScreenSaveCursor)) && !bytes.Contains(data, []byte(ansi.ResetModeAltScreen)) {
		if _, err := io.WriteString(o.writer, o.placement); err != nil {
			return written, err
		}
	}
	return written, nil
}

func (o *nativeImageOutput) Read(data []byte) (int, error) {
	reader, ok := o.writer.(io.Reader)
	if !ok {
		return 0, io.EOF
	}
	return reader.Read(data)
}

func (o *nativeImageOutput) Close() error {
	// The wrapper borrows the program output; Bubble Tea must not close stdout.
	return nil
}

func (o *nativeImageOutput) Fd() uintptr {
	file, ok := o.writer.(*os.File)
	if !ok {
		return ^uintptr(0)
	}
	return file.Fd()
}
