//go:build windows && !cgo

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/tossp/voxink/internal/platform/windows"
)

func TestRunWindowsReportsCGODisabled(t *testing.T) {
	config, loadErr := LoadRuntimeConfig(env(map[string]string{
		envVolcAPIKey: "volc-secret", envVolcResourceID: "resource", envMiMoAPIKey: "mimo-secret",
	}))
	if loadErr != nil {
		t.Fatalf("LoadRuntimeConfig() error = %v", loadErr)
	}
	err := RunWindows(context.Background(), config)
	if !errors.Is(err, windows.ErrCGODisabled) {
		t.Fatalf("RunWindows() error = %v, want ErrCGODisabled", err)
	}
}
