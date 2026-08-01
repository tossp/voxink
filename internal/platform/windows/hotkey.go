package windows

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const (
	modifierAlt      uint32 = 0x0001
	modifierControl  uint32 = 0x0002
	modifierShift    uint32 = 0x0004
	modifierWin      uint32 = 0x0008
	modifierNoRepeat uint32 = 0x4000
	virtualKeySpace  uint32 = 0x20
)

// DefaultHotkey is the built-in stage-one toggle shortcut.
const DefaultHotkey = "Ctrl+Shift+Space"

// ErrInvalidHotkey reports a shortcut outside the fixed supported grammar.
var ErrInvalidHotkey = errors.New("invalid hotkey")

// Hotkey is a validated global shortcut and its Win32 registration values.
type Hotkey struct {
	canonical  string
	modifiers  uint32
	virtualKey uint32
}

// ParseHotkey accepts one or more Ctrl/Alt/Shift/Win modifiers followed by one
// A-Z, 0-9, F1-F24, or Space key. Matching is case-insensitive.
func ParseHotkey(raw string) (Hotkey, error) {
	parts := strings.Split(strings.TrimSpace(raw), "+")
	if len(parts) < 2 || len(parts) > 5 {
		return Hotkey{}, ErrInvalidHotkey
	}
	var modifiers uint32
	seen := make(map[string]bool, len(parts)-1)
	for _, rawModifier := range parts[:len(parts)-1] {
		modifier := strings.ToLower(strings.TrimSpace(rawModifier))
		if seen[modifier] {
			return Hotkey{}, ErrInvalidHotkey
		}
		seen[modifier] = true
		switch modifier {
		case "ctrl":
			modifiers |= modifierControl
		case "alt":
			modifiers |= modifierAlt
		case "shift":
			modifiers |= modifierShift
		case "win":
			modifiers |= modifierWin
		default:
			return Hotkey{}, ErrInvalidHotkey
		}
	}
	keyName, virtualKey, ok := parseVirtualKey(parts[len(parts)-1])
	if !ok {
		return Hotkey{}, ErrInvalidHotkey
	}
	canonical := make([]string, 0, 5)
	for _, modifier := range []struct {
		mask uint32
		name string
	}{{modifierControl, "Ctrl"}, {modifierAlt, "Alt"}, {modifierShift, "Shift"}, {modifierWin, "Win"}} {
		if modifiers&modifier.mask != 0 {
			canonical = append(canonical, modifier.name)
		}
	}
	canonical = append(canonical, keyName)
	return Hotkey{canonical: strings.Join(canonical, "+"), modifiers: modifiers, virtualKey: virtualKey}, nil
}

func parseVirtualKey(raw string) (string, uint32, bool) {
	key := strings.ToUpper(strings.TrimSpace(raw))
	if len(key) == 1 && (key[0] >= 'A' && key[0] <= 'Z' || key[0] >= '0' && key[0] <= '9') {
		return key, uint32(key[0]), true
	}
	if key == "SPACE" {
		return "Space", virtualKeySpace, true
	}
	if strings.HasPrefix(key, "F") {
		number, err := strconv.Atoi(strings.TrimPrefix(key, "F"))
		if err == nil && number >= 1 && number <= 24 {
			return fmt.Sprintf("F%d", number), uint32(0x70 + number - 1), true
		}
	}
	return "", 0, false
}

// String returns the canonical shortcut text without exposing Win32 details.
func (h Hotkey) String() string { return h.canonical }

// Modifiers returns the Win32 modifier mask including MOD_NOREPEAT.
func (h Hotkey) Modifiers() uint32 { return h.modifiers | modifierNoRepeat }

// VirtualKey returns the Win32 virtual-key code.
func (h Hotkey) VirtualKey() uint32 { return h.virtualKey }
