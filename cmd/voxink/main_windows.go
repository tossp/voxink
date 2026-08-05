//go:build windows && cgo

// Command voxink starts the VoxInk desktop application.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/tossp/voxink/internal/app"
	"github.com/tossp/voxink/internal/credential"
	"github.com/tossp/voxink/internal/history"
	"github.com/tossp/voxink/internal/platform/windows"
	"github.com/tossp/voxink/internal/settings"
	"github.com/tossp/voxink/internal/ui"
)

func main() {
	handleCommand(showWindowsMessage)
	if buildMode != buildModeGUI {
		runCLIApplication()
		return
	}
	runGUIApplication()
}

func runCLIApplication() {
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

func runGUIApplication() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	credentialStore := windows.NewCredentialStore()
	settingsStore := settings.NewDefaultStore()
	historyStore := history.NewDefaultStore()
	events := make(chan app.RuntimeEvent, 64)
	reload := make(chan struct{}, 1)
	control := app.NewRuntimeControl()
	gui, err := ui.New(ui.Options{
		Events: events, Settings: settingsStore, Credentials: credentialStore, History: historyStore,
		Getenv: os.Getenv, Toggle: control.Toggle, UpdateHotkey: control.UpdateHotkey,
		Saved: func() { requestReload(reload) }, CaptureSupported: true,
	})
	if err != nil {
		reportStartupError(buildMode, err, os.Stderr, showWindowsMessage)
		return
	}
	go superviseRuntime(ctx, credentialStore, settingsStore, historyStore, control, events, reload)
	go func() {
		<-ctx.Done()
		sendDesktopEvent(events, app.RuntimeEvent{Action: app.ActionQuit})
	}()
	gui.Run()
}

func superviseRuntime(
	ctx context.Context,
	credentialStore credential.Store,
	settingsStore settings.Repository,
	historyStore *history.Store,
	control *app.RuntimeControl,
	events chan<- app.RuntimeEvent,
	reload <-chan struct{},
) {
	for {
		config, err := app.LoadRuntimeConfigWithSettings(os.Getenv, credentialStore, settingsStore)
		if err != nil {
			sendDesktopEvent(events, app.RuntimeEvent{Status: app.StatusFailed})
			select {
			case <-ctx.Done():
				return
			case <-reload:
				continue
			}
		}
		runtimeContext, cancelRuntime := context.WithCancel(ctx)
		done := make(chan error, 1)
		go func() {
			done <- app.RunWindowsWithOptions(runtimeContext, config, app.WindowsOptions{
				History: historyStore, RuntimeEvents: events, Control: control,
			})
		}()
		select {
		case <-ctx.Done():
			cancelRuntime()
			<-done
			return
		case <-reload:
			cancelRuntime()
			<-done
			continue
		case runErr := <-done:
			cancelRuntime()
			if runErr == nil {
				sendDesktopEvent(events, app.RuntimeEvent{Action: app.ActionQuit})
				return
			}
			sendDesktopEvent(events, app.RuntimeEvent{Status: app.StatusFailed})
			select {
			case <-ctx.Done():
				return
			case <-reload:
			}
		}
	}
}

func requestReload(reload chan<- struct{}) {
	select {
	case reload <- struct{}{}:
	default:
	}
}

func sendDesktopEvent(events chan<- app.RuntimeEvent, event app.RuntimeEvent) {
	select {
	case events <- event:
	default:
	}
}
