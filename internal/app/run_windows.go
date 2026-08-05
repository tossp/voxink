//go:build windows

package app

import (
	"context"
	"fmt"

	"github.com/tossp/voxink/internal/platform/windows"
	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
)

// WindowsOptions provides desktop UI bridges without coupling app to Fyne.
type WindowsOptions struct {
	History       HistoryStore
	RuntimeEvents chan<- RuntimeEvent
	Control       *RuntimeControl
}

// RunWindows constructs the stage-one Windows adapters and runs the coordinator.
func RunWindows(ctx context.Context, config RuntimeConfig) error {
	return RunWindowsWithOptions(ctx, config, WindowsOptions{})
}

// RunWindowsWithOptions constructs and runs the Windows adapters with UI bridges.
func RunWindowsWithOptions(ctx context.Context, config RuntimeConfig, options WindowsOptions) error {
	if err := config.validateStageOne(); err != nil {
		return err
	}
	capture, err := windows.NewCapture()
	if err != nil {
		return fmt.Errorf("initialize Windows capture: %w", err)
	}

	client, clientErr := volcengine.NewClient(*config.volc)
	if clientErr != nil {
		_ = capture.Close()
		return fmt.Errorf("configure primary live ASR: %w", clientErr)
	}
	transcriber, transcriberErr := mimo.NewTranscriber(*config.mimo)
	if transcriberErr != nil {
		_ = capture.Close()
		return fmt.Errorf("configure backup batch ASR: %w", transcriberErr)
	}
	textOutput, outputErr := windows.NewOutput()
	if outputErr != nil {
		_ = capture.Close()
		return fmt.Errorf("initialize Windows output: %w", outputErr)
	}
	overlay := windows.NewRuntimeOverlayWithHotkey(config.hotkey)
	if options.Control != nil {
		if err := options.Control.attach(overlay); err != nil {
			_ = capture.Close()
			_ = overlay.Close()
			return err
		}
		defer options.Control.detach(overlay)
	}
	forwardContext, cancelForward := context.WithCancel(ctx)
	defer cancelForward()
	go forwardWindowAction(forwardContext, overlay.OpenMainRequests(), options.RuntimeEvents, ActionOpenMain)
	go forwardWindowAction(forwardContext, overlay.OpenSettingsRequests(), options.RuntimeEvents, ActionOpenSettings)
	coordinator, err := NewCoordinator(capture, windowsOverlay{overlay}, client, transcriber, Options{
		Output: textOutput, History: options.History, RuntimeEvents: options.RuntimeEvents,
	})
	if err != nil {
		_ = capture.Close()
		_ = overlay.Close()
		return err
	}
	return coordinator.Run(ctx)
}

func forwardWindowAction(ctx context.Context, source <-chan struct{}, target chan<- RuntimeEvent, action RuntimeAction) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-source:
			sendRuntimeEvent(target, RuntimeEvent{Action: action})
		}
	}
}

type windowsOverlay struct {
	overlay *windows.Overlay
}

func (o windowsOverlay) Run() error               { return o.overlay.Run() }
func (o windowsOverlay) Close() error             { return o.overlay.Close() }
func (o windowsOverlay) Toggles() <-chan struct{} { return o.overlay.Toggles() }
func (o windowsOverlay) Exits() <-chan struct{}   { return o.overlay.Exits() }
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
		Status: status, Level: view.Level, Partial: view.Partial, Final: view.Final, Error: view.Error, Notice: view.Notice,
	})
}
