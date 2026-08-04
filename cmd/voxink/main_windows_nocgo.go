//go:build windows && !cgo

// Command voxink starts the VoxInk desktop application.
package main

import (
	"os"

	"github.com/tossp/voxink/internal/platform/windows"
)

func main() {
	handleCommand(showWindowsMessage)
	reportStartupError(buildMode, windows.ErrCGODisabled, os.Stderr, showWindowsMessage)
	os.Exit(1)
}
