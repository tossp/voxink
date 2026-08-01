package main

import (
	"os"

	"github.com/tossp/voxink/internal/selfcheck"
)

func handleSelfCheck() {
	if len(os.Args) < 2 || os.Args[1] != "self-check" {
		return
	}
	os.Exit(selfcheck.Execute(os.Args[2:], os.Stdout, os.Stderr))
}
