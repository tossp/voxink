package main

import (
	"io"
	"os"

	"github.com/tossp/voxink/internal/credential"
	"github.com/tossp/voxink/internal/platform/windows"
	"github.com/tossp/voxink/internal/selfcheck"
	"github.com/tossp/voxink/internal/settings"
	"github.com/tossp/voxink/internal/smoke"
)

func handleCommand() {
	handled, code := dispatchCommand(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	if !handled {
		return
	}
	os.Exit(code)
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
