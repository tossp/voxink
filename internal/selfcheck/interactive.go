package selfcheck

import "time"

func runInteractive(timeout time.Duration, deps dependencies) []Check {
	overlay := deps.newInteractive()
	overlay.ShowNotice(selfCheckNotice)
	runDone := make(chan error, 1)
	go func() { runDone <- overlay.Run() }()

	status, code := StatusPass, CodeOK
	toggleReceived := false
	runExited := false
	timer := time.NewTimer(timeout)
	select {
	case <-overlay.Toggles():
		toggleReceived = true
	case err := <-runDone:
		runExited = true
		status, code = StatusFail, platformCode(err, CodeOverlayUnavailable)
	case <-timer.C:
		status, code = StatusFail, CodeToggleTimeout
	}
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	if err := overlay.Close(); err != nil && status != StatusFail {
		status, code = StatusFail, CodeOverlayUnavailable
	}
	if !runExited {
		cleanupTimer := time.NewTimer(time.Second)
		select {
		case <-runDone:
		case <-cleanupTimer.C:
			if status != StatusFail {
				status, code = StatusFail, CodeOverlayUnavailable
			}
		}
		if !cleanupTimer.Stop() {
			select {
			case <-cleanupTimer.C:
			default:
			}
		}
	}

	focusStatus, focusCode := StatusManual, CodeFocusConfirmationRequired
	if code == CodePlatformUnsupported || code == CodeCGODisabled || code == CodeOverlayUnavailable || code == CodeHotkeyUnavailable {
		focusStatus, focusCode = StatusSkipped, CodeFocusNotTested
	}
	return []Check{
		{Name: "interactive_hotkey", Status: status, Code: code, Metrics: Metrics{ToggleReceived: boolean(toggleReceived)}},
		{Name: "overlay_no_activate", Status: focusStatus, Code: focusCode},
	}
}
