//go:build windows

package app

import (
	"context"
	"fmt"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/platform/windows"
	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
)

// RunWindows constructs the stage-one Windows adapters and runs the coordinator.
func RunWindows(ctx context.Context, config RuntimeConfig) error {
	capture, err := windows.NewCapture()
	if err != nil {
		return fmt.Errorf("initialize Windows capture: %w", err)
	}

	var live asr.LiveRecognizer
	if config.volc != nil {
		client, clientErr := volcengine.NewClient(*config.volc)
		if clientErr != nil {
			_ = capture.Close()
			return fmt.Errorf("configure primary live ASR: %w", clientErr)
		}
		live = client
	}
	var batch asr.SegmentTranscriber
	if config.mimo != nil {
		transcriber, transcriberErr := mimo.NewTranscriber(*config.mimo)
		if transcriberErr != nil {
			_ = capture.Close()
			return fmt.Errorf("configure batch ASR: %w", transcriberErr)
		}
		batch = transcriber
	}
	overlay := windows.NewOverlay()
	coordinator, err := NewCoordinator(capture, windowsOverlay{overlay}, live, batch, Options{})
	if err != nil {
		_ = capture.Close()
		_ = overlay.Close()
		return err
	}
	return coordinator.Run(ctx)
}

type windowsOverlay struct {
	overlay *windows.Overlay
}

func (o windowsOverlay) Run() error               { return o.overlay.Run() }
func (o windowsOverlay) Close() error             { return o.overlay.Close() }
func (o windowsOverlay) Toggles() <-chan struct{} { return o.overlay.Toggles() }
func (o windowsOverlay) Update(view View) {
	status := windows.ViewIdle
	switch view.Status {
	case ViewListening:
		status = windows.ViewListening
	case ViewTranscribing:
		status = windows.ViewTranscribing
	case ViewError:
		status = windows.ViewError
	}
	o.overlay.Update(windows.View{
		Status: status, Level: view.Level, Partial: view.Partial, Final: view.Final, Error: view.Error,
	})
}
