package windows

import "testing"

func TestParseHotkeyCanonicalAndWin32Mapping(t *testing.T) {
	tests := []struct {
		input     string
		canonical string
		modifiers uint32
		key       uint32
	}{
		{"shift+ctrl+space", "Ctrl+Shift+Space", modifierControl | modifierShift | modifierNoRepeat, virtualKeySpace},
		{"Alt+Win+A", "Alt+Win+A", modifierAlt | modifierWin | modifierNoRepeat, 'A'},
		{"Ctrl+F24", "Ctrl+F24", modifierControl | modifierNoRepeat, 0x87},
		{"Ctrl+9", "Ctrl+9", modifierControl | modifierNoRepeat, '9'},
	}
	for _, test := range tests {
		hotkey, err := ParseHotkey(test.input)
		if err != nil {
			t.Fatalf("ParseHotkey(%q) error = %v", test.input, err)
		}
		if hotkey.String() != test.canonical || hotkey.Modifiers() != test.modifiers || hotkey.VirtualKey() != test.key {
			t.Errorf("ParseHotkey(%q) = %q %#x %#x", test.input, hotkey.String(), hotkey.Modifiers(), hotkey.VirtualKey())
		}
	}
}

func TestParseHotkeyRejectsUnsupportedForms(t *testing.T) {
	for _, input := range []string{"", "Space", "Ctrl+", "Ctrl+Ctrl+A", "Control+A", "Windows+A", "Ctrl+F25", "Ctrl+Escape", "Meta+A"} {
		if _, err := ParseHotkey(input); err == nil {
			t.Errorf("ParseHotkey(%q) error = nil", input)
		}
	}
}

func TestNewOverlayWithHotkeyRetainsRuntimeRegistrationValues(t *testing.T) {
	hotkey, err := ParseHotkey("Alt+Win+F4")
	if err != nil {
		t.Fatal(err)
	}
	overlay := NewOverlayWithHotkey(hotkey)
	if overlay.hotkey.String() != "Alt+Win+F4" || overlay.hotkey.Modifiers() != modifierAlt|modifierWin|modifierNoRepeat || overlay.hotkey.VirtualKey() != 0x73 {
		t.Fatalf("overlay hotkey = %q %#x %#x", overlay.hotkey.String(), overlay.hotkey.Modifiers(), overlay.hotkey.VirtualKey())
	}
	if overlay.trayOn {
		t.Fatal("self-check overlay unexpectedly enabled tray controls")
	}
	if runtimeOverlay := NewRuntimeOverlayWithHotkey(hotkey); !runtimeOverlay.trayOn {
		t.Fatal("runtime overlay did not enable tray controls")
	}
}
