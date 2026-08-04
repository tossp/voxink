package main

import (
	"fmt"
	"io"
	"os"

	"github.com/tossp/voxink/internal/credential"
	"github.com/tossp/voxink/internal/platform/windows"
	"github.com/tossp/voxink/internal/selfcheck"
	"github.com/tossp/voxink/internal/settings"
	"github.com/tossp/voxink/internal/smoke"
)

const (
	buildModeCLI = "cli"
	buildModeGUI = "gui"

	guiCLIUsageMessage     = "请使用voxink-cli.exe"
	guiStartupErrorMessage = "VoxInk无法启动。请使用voxink-cli.exe运行self-check。"
	cliStartupErrorMessage = "VoxInk could not start. Run voxink-cli.exe self-check for safe diagnostics."
)

// buildMode is set to gui or cli by release build linker flags.
var buildMode = buildModeCLI

type messagePresenter func(string)

func handleCommand(presentMessage messagePresenter) {
	handled, code := dispatchCommandForMode(buildMode, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, presentMessage)
	if !handled {
		return
	}
	os.Exit(code)
}

func dispatchCommandForMode(mode string, args []string, stdin io.Reader, stdout, stderr io.Writer, presentMessage messagePresenter) (bool, int) {
	if mode == buildModeGUI && len(args) > 0 {
		if presentMessage != nil {
			presentMessage(guiCLIUsageMessage)
		}
		return true, 2
	}
	return dispatchCommand(args, stdin, stdout, stderr)
}

func reportStartupError(mode string, _ error, stderr io.Writer, presentMessage messagePresenter) {
	if mode == buildModeGUI {
		if presentMessage != nil {
			presentMessage(guiStartupErrorMessage)
		}
		return
	}
	fmt.Fprintln(stderr, cliStartupErrorMessage)
}

func dispatchCommand(args []string, stdin io.Reader, stdout, stderr io.Writer) (bool, int) {
	return dispatchCommandWithStores(args, stdin, stdout, stderr, windows.NewCredentialStore(), settings.NewDefaultStore(), os.Getenv)
}

func dispatchCommandWithStore(args []string, stdin io.Reader, stdout, stderr io.Writer, store credential.Store) (bool, int) {
	return dispatchCommandWithStores(args, stdin, stdout, stderr, store, settings.NewDefaultStore(), os.Getenv)
}

func dispatchCommandWithStores(args []string, stdin io.Reader, stdout, stderr io.Writer, credentialStore credential.Store, settingsStore settings.Repository, getenv func(string) string) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "self-check":
		return true, selfcheck.Execute(args[1:], stdout, stderr)
	case "smoke":
		return true, smoke.ExecuteWithSettings(args[1:], stdout, stderr, credentialStore, settingsStore)
	case "config":
		if len(args) > 1 && args[1] == "settings" {
			return true, settings.Execute(args[2:], stdout, stderr, settingsStore, getenv)
		}
		if len(args) == 1 || args[1] != "credential" {
			return true, credential.Execute(nil, stdout, stderr, credentialStore, nil)
		}
		readValue := func() ([]byte, error) { return windows.ReadCredentialValue(stdin) }
		return true, credential.Execute(args[2:], stdout, stderr, credentialStore, readValue)
	default:
		return false, 0
	}
}
