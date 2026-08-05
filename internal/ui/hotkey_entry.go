//go:build (windows && cgo) || fyne_gui

package ui

import (
	"fmt"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/widget"
)

type hotkeyEntry struct {
	widget.Entry
	capturing bool
	modifiers func() fyne.KeyModifier
	onFocus   func(bool)
}

func newHotkeyEntry(modifiers func() fyne.KeyModifier, onFocus func(bool)) *hotkeyEntry {
	entry := &hotkeyEntry{modifiers: modifiers, onFocus: onFocus}
	entry.ExtendBaseWidget(entry)
	return entry
}

func (e *hotkeyEntry) FocusGained() {
	e.Entry.FocusGained()
	if e.onFocus != nil {
		e.onFocus(true)
	}
}

func (e *hotkeyEntry) FocusLost() {
	e.capturing = false
	e.Entry.FocusLost()
	if e.onFocus != nil {
		e.onFocus(false)
	}
}

func (e *hotkeyEntry) TypedKey(event *fyne.KeyEvent) {
	if !e.capturing {
		e.Entry.TypedKey(event)
		return
	}
	modifiers := fyne.KeyModifier(0)
	if e.modifiers != nil {
		modifiers = e.modifiers()
	}
	if value := capturedHotkey(modifiers, event.Name); value != "" {
		e.SetText(value)
		e.capturing = false
	}
}

func capturedHotkey(modifiers fyne.KeyModifier, key fyne.KeyName) string {
	keyName := string(key)
	if key == fyne.KeySpace {
		keyName = "Space"
	}
	if !supportedCaptureKey(keyName) {
		return ""
	}
	parts := make([]string, 0, 5)
	if modifiers&fyne.KeyModifierControl != 0 {
		parts = append(parts, "Ctrl")
	}
	if modifiers&fyne.KeyModifierAlt != 0 {
		parts = append(parts, "Alt")
	}
	if modifiers&fyne.KeyModifierShift != 0 {
		parts = append(parts, "Shift")
	}
	if modifiers&fyne.KeyModifierSuper != 0 {
		parts = append(parts, "Win")
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(append(parts, keyName), "+")
}

func supportedCaptureKey(key string) bool {
	if len(key) == 1 {
		return key[0] >= 'A' && key[0] <= 'Z' || key[0] >= '0' && key[0] <= '9'
	}
	if key == "Space" {
		return true
	}
	if len(key) >= 2 && key[0] == 'F' {
		var number int
		if _, err := fmt.Sscanf(key, "F%d", &number); err == nil {
			return number >= 1 && number <= 24
		}
	}
	return false
}
