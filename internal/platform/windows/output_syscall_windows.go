//go:build windows

package windows

import (
	"unsafe"

	textoutput "github.com/tossp/voxink/internal/output"
	"golang.org/x/sys/windows"
)

const (
	guiThreadInfoSize = uint32(unsafe.Sizeof(guiThreadInfo{}))
	gwlStyle          = ^uintptr(15)
	esPassword        = 0x0020
	inputKeyboard     = 1
	keyEventUnicode   = 0x0004
	keyEventKeyUp     = 0x0002
	cfUnicodeText     = 13
	gmemMoveable      = 0x0002
)

var (
	procGetForegroundWindow    = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcess = user32.NewProc("GetWindowThreadProcessId")
	procGetGUIThreadInfo       = user32.NewProc("GetGUIThreadInfo")
	procGetWindowLongPtr       = user32.NewProc("GetWindowLongPtrW")
	procSendInput              = user32.NewProc("SendInput")
	procOpenClipboard          = user32.NewProc("OpenClipboard")
	procEmptyClipboard         = user32.NewProc("EmptyClipboard")
	procSetClipboardData       = user32.NewProc("SetClipboardData")
	procCloseClipboard         = user32.NewProc("CloseClipboard")
	procGlobalAlloc            = kernel32.NewProc("GlobalAlloc")
	procGlobalLock             = kernel32.NewProc("GlobalLock")
	procGlobalUnlock           = kernel32.NewProc("GlobalUnlock")
	procGlobalFree             = kernel32.NewProc("GlobalFree")
	procRtlMoveMemory          = kernel32.NewProc("RtlMoveMemory")
	procSetLastError           = kernel32.NewProc("SetLastError")
)

type guiThreadInfo struct {
	size        uint32
	flags       uint32
	active      windowHandle
	focus       windowHandle
	capture     windowHandle
	menuOwner   windowHandle
	moveSize    windowHandle
	caret       windowHandle
	caretBounds rect
}

type keyboardInput struct {
	virtualKey uint16
	scan       uint16
	flags      uint32
	time       uint32
	extraInfo  uintptr
}

type mouseInput struct {
	dx        int32
	dy        int32
	mouseData uint32
	flags     uint32
	time      uint32
	extraInfo uintptr
}

type winInput struct {
	kind uint32
	data mouseInput
}

type syscallOutputBackend struct{}

// NewOutput constructs the Win32 final-text output adapter.
func NewOutput() (textoutput.Adapter, error) {
	return newTextOutput(syscallOutputBackend{}), nil
}

func (syscallOutputBackend) foregroundWindow() windowHandle {
	hwnd, _, _ := procGetForegroundWindow.Call()
	return windowHandle(hwnd)
}

func (syscallOutputBackend) safeFocusedControl(target windowHandle) bool {
	threadID, _, _ := procGetWindowThreadProcess.Call(uintptr(target), 0)
	if threadID == 0 {
		return false
	}
	info := guiThreadInfo{size: guiThreadInfoSize}
	ok, _, _ := procGetGUIThreadInfo.Call(threadID, uintptr(unsafe.Pointer(&info)))
	if ok == 0 || info.active != target || info.focus == 0 {
		return false
	}
	procSetLastError.Call(0)
	style, _, callErr := procGetWindowLongPtr.Call(uintptr(info.focus), gwlStyle)
	if style == 0 && callErr != windows.ERROR_SUCCESS {
		return false
	}
	return style&esPassword == 0
}

func (syscallOutputBackend) sendInput(events []unicodeInput) uint32 {
	if len(events) == 0 {
		return 0
	}
	inputs := make([]winInput, len(events))
	for index, event := range events {
		flags := uint32(keyEventUnicode)
		if event.keyUp {
			flags |= keyEventKeyUp
		}
		inputs[index].kind = inputKeyboard
		keyboard := (*keyboardInput)(unsafe.Pointer(&inputs[index].data))
		*keyboard = keyboardInput{scan: event.codeUnit, flags: flags}
	}
	sent, _, _ := procSendInput.Call(uintptr(len(inputs)), uintptr(unsafe.Pointer(&inputs[0])), unsafe.Sizeof(inputs[0]))
	return uint32(sent)
}

func (syscallOutputBackend) openClipboard() bool {
	ok, _, _ := procOpenClipboard.Call(0)
	return ok != 0
}

func (syscallOutputBackend) emptyClipboard() bool {
	ok, _, _ := procEmptyClipboard.Call()
	return ok != 0
}

func (syscallOutputBackend) globalAlloc(size uintptr) globalHandle {
	handle, _, _ := procGlobalAlloc.Call(gmemMoveable, size)
	return globalHandle(handle)
}

func (syscallOutputBackend) writeGlobal(handle globalHandle, units []uint16) bool {
	pointer, _, _ := procGlobalLock.Call(uintptr(handle))
	if pointer == 0 {
		return false
	}
	procRtlMoveMemory.Call(pointer, uintptr(unsafe.Pointer(&units[0])), uintptr(len(units))*2)
	procSetLastError.Call(0)
	unlocked, _, callErr := procGlobalUnlock.Call(uintptr(handle))
	return unlocked != 0 || callErr == windows.ERROR_SUCCESS
}

func (syscallOutputBackend) setClipboardData(handle globalHandle) bool {
	result, _, _ := procSetClipboardData.Call(cfUnicodeText, uintptr(handle))
	return result != 0
}

func (syscallOutputBackend) globalFree(handle globalHandle) {
	procGlobalFree.Call(uintptr(handle))
}

func (syscallOutputBackend) closeClipboard() {
	procCloseClipboard.Call()
}
