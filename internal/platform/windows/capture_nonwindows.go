//go:build !windows

package windows

// NewCapture reports that the WASAPI adapter is unavailable outside Windows.
func NewCapture() (*Capture, error) {
	return nil, ErrUnsupportedPlatform
}
