//go:build !windows

package windows

// Run reports that the native overlay is unavailable outside Windows.
func (o *Overlay) Run() error { return ErrUnsupportedPlatform }

// Close marks the non-Windows overlay closed.
func (o *Overlay) Close() error {
	o.closing.Store(true)
	return nil
}
