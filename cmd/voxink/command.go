package main

import (
	"io"
	"os"

	"github.com/tossp/voxink/internal/selfcheck"
	"github.com/tossp/voxink/internal/smoke"
)

func handleCommand() {
	handled, code := dispatchCommand(os.Args[1:], os.Stdout, os.Stderr)
	if !handled {
		return
	}
	os.Exit(code)
}

func dispatchCommand(args []string, stdout, stderr io.Writer) (bool, int) {
	if len(args) == 0 {
		return false, 0
	}
	switch args[0] {
	case "self-check":
		return true, selfcheck.Execute(args[1:], stdout, stderr)
	case "smoke":
		return true, smoke.Execute(args[1:], stdout, stderr)
	default:
		return false, 0
	}
}
