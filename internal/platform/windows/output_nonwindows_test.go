//go:build !windows

package windows

import (
	"errors"
	"testing"
)

func TestNewOutputReportsUnsupportedPlatform(t *testing.T) {
	adapter, err := NewOutput()
	if adapter != nil || !errors.Is(err, ErrUnsupportedPlatform) {
		t.Fatalf("NewOutput() = (%v, %v), want nil ErrUnsupportedPlatform", adapter, err)
	}
}
