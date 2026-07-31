//go:build windows && !cgo

package app

import (
	"context"
	"errors"
	"testing"

	"github.com/tossp/voxink/internal/platform/windows"
)

func TestRunWindowsReportsCGODisabled(t *testing.T) {
	err := RunWindows(context.Background(), RuntimeConfig{})
	if !errors.Is(err, windows.ErrCGODisabled) {
		t.Fatalf("RunWindows() error = %v, want ErrCGODisabled", err)
	}
}
