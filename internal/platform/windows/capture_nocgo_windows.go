//go:build windows && !cgo

package windows

// NewCapture reports that the malgo WASAPI adapter requires cgo on Windows.
func NewCapture() (*Capture, error) {
	return nil, ErrCGODisabled
}
