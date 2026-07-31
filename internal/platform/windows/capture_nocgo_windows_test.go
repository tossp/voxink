//go:build windows && !cgo

package windows

import (
	"errors"
	"testing"
)

func TestNewCaptureReportsCGODisabled(t *testing.T) {
	capture, err := NewCapture()
	if capture != nil {
		t.Fatal("NewCapture returned a capture without cgo")
	}
	if !errors.Is(err, ErrCGODisabled) {
		t.Fatalf("NewCapture error = %v, want ErrCGODisabled", err)
	}
}
