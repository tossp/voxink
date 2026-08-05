package app

import (
	"errors"
	"testing"

	platformwindows "github.com/tossp/voxink/internal/platform/windows"
)

func TestRuntimeControlForwardsToggleAndValidatedHotkey(t *testing.T) {
	control := NewRuntimeControl()
	target := &fakeRuntimeController{}
	control.Toggle()
	if err := control.UpdateHotkey("Shift+Ctrl+Space"); err != nil {
		t.Fatalf("detached UpdateHotkey() error = %v", err)
	}
	if err := control.attach(target); err != nil {
		t.Fatalf("attach() error = %v", err)
	}
	control.Toggle()
	if target.toggles != 1 || target.hotkey.String() != "Ctrl+Shift+Space" {
		t.Fatalf("target toggle/hotkey = %d/%q", target.toggles, target.hotkey)
	}
	if err := control.UpdateHotkey("Win+F8"); err != nil || target.hotkey.String() != "Win+F8" {
		t.Fatalf("attached UpdateHotkey() = %q, %v", target.hotkey, err)
	}
	control.detach(target)
	control.Toggle()
	if target.toggles != 1 {
		t.Fatalf("detached toggle count = %d", target.toggles)
	}
}

func TestRuntimeControlRejectsInvalidAndPreservesApplyFailure(t *testing.T) {
	control := NewRuntimeControl()
	if err := control.UpdateHotkey("Space"); !errors.Is(err, platformwindows.ErrInvalidHotkey) {
		t.Fatalf("invalid UpdateHotkey() error = %v", err)
	}
	target := &fakeRuntimeController{err: platformwindows.ErrHotkeyUnavailable}
	if err := control.attach(target); err != nil {
		t.Fatalf("attach() error = %v", err)
	}
	if err := control.UpdateHotkey("Ctrl+F9"); !errors.Is(err, platformwindows.ErrHotkeyUnavailable) {
		t.Fatalf("failed UpdateHotkey() error = %v", err)
	}
	control.Toggle()
	if target.toggles != 1 {
		t.Fatalf("failed update unexpectedly detached target; toggles = %d", target.toggles)
	}

	pendingControl := NewRuntimeControl()
	if err := pendingControl.UpdateHotkey("Ctrl+F10"); err != nil {
		t.Fatal(err)
	}
	if err := pendingControl.attach(target); !errors.Is(err, platformwindows.ErrHotkeyUnavailable) {
		t.Fatalf("failed pending attach error = %v", err)
	}
	pendingControl.Toggle()
	if target.toggles != 1 {
		t.Fatalf("failed attach retained target; toggles = %d", target.toggles)
	}
}

type fakeRuntimeController struct {
	toggles int
	hotkey  platformwindows.Hotkey
	err     error
}

func (c *fakeRuntimeController) RequestToggle() { c.toggles++ }
func (c *fakeRuntimeController) UpdateHotkey(hotkey platformwindows.Hotkey) error {
	if c.err == nil {
		c.hotkey = hotkey
	}
	return c.err
}
