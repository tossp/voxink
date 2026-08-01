//go:build windows && cgo

// Command voxink starts the VoxInk desktop application.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/tossp/voxink/internal/app"
)

func main() {
	handleSelfCheck()
	config, err := app.LoadRuntimeConfig(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "VoxInk configuration error:", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	if err := app.RunWindows(ctx, config); err != nil {
		fmt.Fprintln(os.Stderr, "VoxInk stopped:", err)
		os.Exit(1)
	}
}
