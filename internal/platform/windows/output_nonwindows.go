//go:build !windows

package windows

import textoutput "github.com/tossp/voxink/internal/output"

// NewOutput reports that Win32 final-text output is unavailable.
func NewOutput() (textoutput.Adapter, error) {
	return nil, ErrUnsupportedPlatform
}
