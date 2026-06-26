//go:build windows

package main

import (
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

func setWallpaper(imagePath string) error {
	absPath, err := filepath.Abs(imagePath)
	if err != nil {
		return fmt.Errorf("resolving absolute path: %w", err)
	}

	pathPtr, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return fmt.Errorf("encoding path: %w", err)
	}

	user32 := syscall.NewLazyDLL("user32.dll")
	proc := user32.NewProc("SystemParametersInfoW")

	const (
		spiSetDeskWallpaper = 0x0014
		spifUpdateINIFile   = 0x01
		spifSendChange      = 0x02
	)

	ret, _, callErr := proc.Call(
		spiSetDeskWallpaper,
		0,
		uintptr(unsafe.Pointer(pathPtr)),
		spifUpdateINIFile|spifSendChange,
	)
	if ret == 0 {
		return fmt.Errorf("SystemParametersInfoW: %v", callErr)
	}
	return nil
}
