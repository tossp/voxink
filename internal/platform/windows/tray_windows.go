//go:build windows

package windows

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nimDelete = 0x00000002

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004

	idiApplication = 32512
	mfString       = 0x00000000

	tpmRightButton = 0x0002
	tpmNonotify    = 0x0080
	tpmReturnCmd   = 0x0100
)

var (
	shell32             = windows.NewLazySystemDLL("shell32.dll")
	procShellNotifyIcon = shell32.NewProc("Shell_NotifyIconW")
	procLoadIcon        = user32.NewProc("LoadIconW")
	procCreatePopupMenu = user32.NewProc("CreatePopupMenu")
	procAppendMenu      = user32.NewProc("AppendMenuW")
	procTrackPopupMenu  = user32.NewProc("TrackPopupMenu")
	procDestroyMenu     = user32.NewProc("DestroyMenu")
	procGetCursorPos    = user32.NewProc("GetCursorPos")
	procSetForeground   = user32.NewProc("SetForegroundWindow")
)

type notifyIconData struct {
	size        uint32
	hwnd        windows.Handle
	id          uint32
	flags       uint32
	callback    uint32
	icon        windows.Handle
	tip         [128]uint16
	state       uint32
	stateMask   uint32
	info        [256]uint16
	timeout     uint32
	infoTitle   [64]uint16
	infoFlags   uint32
	guidItem    [16]byte
	balloonIcon windows.Handle
}

type nativeTrayBackend struct {
	data notifyIconData
}

func (b *nativeTrayBackend) Add(hwnd uintptr, tooltip string) error {
	icon, _, iconErr := procLoadIcon.Call(0, idiApplication)
	if icon == 0 {
		return fmt.Errorf("LoadIconW: %w", iconErr)
	}
	b.data = notifyIconData{
		size:     uint32(unsafe.Sizeof(notifyIconData{})),
		hwnd:     windows.Handle(hwnd),
		id:       trayIconID,
		flags:    nifMessage | nifIcon | nifTip,
		callback: wmTrayCallback,
		icon:     windows.Handle(icon),
	}
	setTrayTip(&b.data.tip, tooltip)
	if result, _, err := procShellNotifyIcon.Call(nimAdd, uintptr(unsafe.Pointer(&b.data))); result == 0 {
		return fmt.Errorf("Shell_NotifyIconW(NIM_ADD): %w", err)
	}
	return nil
}

func (b *nativeTrayBackend) Update(tooltip string) error {
	b.data.flags = nifTip
	setTrayTip(&b.data.tip, tooltip)
	if result, _, err := procShellNotifyIcon.Call(nimModify, uintptr(unsafe.Pointer(&b.data))); result == 0 {
		return fmt.Errorf("Shell_NotifyIconW(NIM_MODIFY): %w", err)
	}
	return nil
}

func (b *nativeTrayBackend) Popup(hwnd uintptr, toggleText string) (uint32, error) {
	return popupTrayMenu(nativeTrayMenuAPI{}, hwnd, toggleText)
}

func (b *nativeTrayBackend) Delete() error {
	if result, _, err := procShellNotifyIcon.Call(nimDelete, uintptr(unsafe.Pointer(&b.data))); result == 0 {
		return fmt.Errorf("Shell_NotifyIconW(NIM_DELETE): %w", err)
	}
	return nil
}

func setTrayTip(target *[128]uint16, text string) {
	clear(target[:])
	encoded, _ := windows.UTF16FromString(text)
	copy(target[:], encoded)
}

type nativeTrayMenuAPI struct{}

func (nativeTrayMenuAPI) Create() (uintptr, error) {
	menu, _, err := procCreatePopupMenu.Call()
	if menu == 0 {
		return 0, fmt.Errorf("CreatePopupMenu: %w", err)
	}
	return menu, nil
}

func (nativeTrayMenuAPI) Append(menu uintptr, id uint32, text string) error {
	textPtr, err := windows.UTF16PtrFromString(text)
	if err != nil {
		return err
	}
	if result, _, callErr := procAppendMenu.Call(menu, mfString, uintptr(id), uintptr(unsafe.Pointer(textPtr))); result == 0 {
		return fmt.Errorf("AppendMenuW: %w", callErr)
	}
	return nil
}

func (nativeTrayMenuAPI) Track(menu, hwnd uintptr) (uint32, error) {
	var cursor point
	if result, _, err := procGetCursorPos.Call(uintptr(unsafe.Pointer(&cursor))); result == 0 {
		return 0, fmt.Errorf("GetCursorPos: %w", err)
	}
	procSetForeground.Call(hwnd)
	defer procPostMessage.Call(hwnd, 0, 0, 0)
	command, _, err := procTrackPopupMenu.Call(
		menu, tpmRightButton|tpmNonotify|tpmReturnCmd,
		uintptr(int(cursor.x)), uintptr(int(cursor.y)), 0, hwnd, 0,
	)
	if command == 0 && err != windows.ERROR_SUCCESS {
		return 0, fmt.Errorf("TrackPopupMenu: %w", err)
	}
	return uint32(command), nil
}

func (nativeTrayMenuAPI) Destroy(menu uintptr) error {
	if result, _, err := procDestroyMenu.Call(menu); result == 0 {
		return fmt.Errorf("DestroyMenu: %w", err)
	}
	return nil
}
