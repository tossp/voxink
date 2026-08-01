//go:build windows

package windows

import (
	"testing"
	"unsafe"
)

func TestNotifyIconDataABI(t *testing.T) {
	data := notifyIconData{}
	if unsafe.Sizeof(uintptr(0)) == 8 {
		if got := unsafe.Sizeof(data); got != 976 {
			t.Fatalf("notifyIconData size = %d, want 976", got)
		}
		if got := unsafe.Offsetof(data.hwnd); got != 8 {
			t.Fatalf("hwnd offset = %d, want 8", got)
		}
		if got := unsafe.Offsetof(data.icon); got != 32 {
			t.Fatalf("icon offset = %d, want 32", got)
		}
		if got := unsafe.Offsetof(data.balloonIcon); got != 968 {
			t.Fatalf("balloonIcon offset = %d, want 968", got)
		}
		return
	}
	if got := unsafe.Sizeof(data); got != 956 {
		t.Fatalf("notifyIconData size = %d, want 956", got)
	}
	if got := unsafe.Offsetof(data.hwnd); got != 4 {
		t.Fatalf("hwnd offset = %d, want 4", got)
	}
	if got := unsafe.Offsetof(data.icon); got != 20 {
		t.Fatalf("icon offset = %d, want 20", got)
	}
	if got := unsafe.Offsetof(data.balloonIcon); got != 952 {
		t.Fatalf("balloonIcon offset = %d, want 952", got)
	}
}

func TestTrayMessageMapping(t *testing.T) {
	if got := trayNotificationAction(trayLeftDoubleClick); got != trayActionToggle {
		t.Fatalf("double-click action = %v", got)
	}
	if got := trayNotificationAction(trayRightButtonUp); got != trayActionMenu {
		t.Fatalf("right-button action = %v", got)
	}
	if got := trayCommandAction(trayMenuToggleID); got != trayActionToggle {
		t.Fatalf("toggle command action = %v", got)
	}
	if got := trayCommandAction(trayMenuExitID); got != trayActionExit {
		t.Fatalf("exit command action = %v", got)
	}
}
