//go:build windows && cgo

// Command voxink starts the VoxInk desktop application.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/tossp/voxink/internal/app"
	"github.com/tossp/voxink/internal/platform/windows"
	"github.com/tossp/voxink/internal/settings"
)

func main() {
	handleCommand(showWindowsMessage)
	config, err := app.LoadRuntimeConfigWithSettings(os.Getenv, windows.NewCredentialStore(), settings.NewDefaultStore())
	if err != nil {
		reportStartupError(buildMode, err, os.Stderr, showWindowsMessage)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := app.RunWindows(ctx, config); err != nil {
		reportStartupError(buildMode, err, os.Stderr, showWindowsMessage)
		os.Exit(1)
	}
}
