//go:build windows && !cgo

// Command voxink starts the VoxInk desktop application.
package main

import (
	"fmt"
	"os"

	"github.com/tossp/voxink/internal/platform/windows"
)

func main() {
	handleSelfCheck()
	fmt.Fprintln(os.Stderr, "VoxInk stopped:", windows.ErrCGODisabled)
	os.Exit(1)
}
