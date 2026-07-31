//go:build windows

package windows

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wsPopup         = 0x80000000
	wsExTransparent = 0x00000020
	wsExToolWindow  = 0x00000080
	wsExLayered     = 0x00080000
	wsExNoActivate  = 0x08000000

	wmDestroy       = 0x0002
	wmPaint         = 0x000F
	wmClose         = 0x0010
	wmHotkey        = 0x0312
	wmMouseActivate = 0x0021
	wmAppUpdate     = 0x8001

	maNoActivate     = 3
	swShowNoActivate = 4
	swpNoActivate    = 0x0010

	modControl  = 0x0002
	modShift    = 0x0004
	modNoRepeat = 0x4000
	vkSpace     = 0x20
	hotkeyID    = 1

	lwaAlpha    = 0x00000002
	dtLeft      = 0x00000000
	dtWordBreak = 0x00000010
	dtNoPrefix  = 0x00000800
	transparent = 1

	windowWidth  = 520
	windowHeight = 140
)

var (
	user32                   = windows.NewLazySystemDLL("user32.dll")
	gdi32                    = windows.NewLazySystemDLL("gdi32.dll")
	kernel32                 = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClassEx      = user32.NewProc("RegisterClassExW")
	procCreateWindowEx       = user32.NewProc("CreateWindowExW")
	procDefWindowProc        = user32.NewProc("DefWindowProcW")
	procDestroyWindow        = user32.NewProc("DestroyWindow")
	procIsWindow             = user32.NewProc("IsWindow")
	procShowWindow           = user32.NewProc("ShowWindow")
	procSetWindowPos         = user32.NewProc("SetWindowPos")
	procRegisterHotKey       = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey     = user32.NewProc("UnregisterHotKey")
	procGetMessage           = user32.NewProc("GetMessageW")
	procTranslateMessage     = user32.NewProc("TranslateMessage")
	procDispatchMessage      = user32.NewProc("DispatchMessageW")
	procPostMessage          = user32.NewProc("PostMessageW")
	procPostQuitMessage      = user32.NewProc("PostQuitMessage")
	procBeginPaint           = user32.NewProc("BeginPaint")
	procEndPaint             = user32.NewProc("EndPaint")
	procGetClientRect        = user32.NewProc("GetClientRect")
	procFillRect             = user32.NewProc("FillRect")
	procDrawText             = user32.NewProc("DrawTextW")
	procInvalidateRect       = user32.NewProc("InvalidateRect")
	procSetLayeredAttributes = user32.NewProc("SetLayeredWindowAttributes")
	procGetModuleHandle      = kernel32.NewProc("GetModuleHandleW")
	procCreateSolidBrush     = gdi32.NewProc("CreateSolidBrush")
	procDeleteObject         = gdi32.NewProc("DeleteObject")
	procSetBkMode            = gdi32.NewProc("SetBkMode")
	procSetTextColor         = gdi32.NewProc("SetTextColor")

	overlayWindows sync.Map
	windowProc     = syscall.NewCallback(overlayWindowProc)
)

type point struct{ x, y int32 }

type message struct {
	hwnd    windows.Handle
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	point   point
	private uint32
}

type rect struct{ left, top, right, bottom int32 }

type paintStruct struct {
	hdc       windows.Handle
	erase     int32
	paint     rect
	restore   int32
	incUpdate int32
	reserved  [32]byte
}

type windowClass struct {
	size        uint32
	style       uint32
	wndProc     uintptr
	classExtra  int32
	windowExtra int32
	instance    windows.Handle
	icon        windows.Handle
	cursor      windows.Handle
	background  windows.Handle
	menuName    *uint16
	className   *uint16
	iconSmall   windows.Handle
}

// Run creates the window and hotkey and owns GetMessage/DispatchMessage on one OS thread.
func (o *Overlay) Run() error {
	if o.closing.Load() {
		return ErrOverlayClosed
	}
	if !o.running.CompareAndSwap(false, true) {
		return ErrOverlayRunning
	}
	defer o.running.Store(false)
	defer o.closing.Store(true)
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hwnd, err := o.createWindow()
	if err != nil {
		return err
	}
	o.hwnd.Store(uintptr(hwnd))
	overlayWindows.Store(hwnd, o)
	defer func() {
		overlayWindows.Delete(hwnd)
		o.hwnd.Store(0)
		// Closing done guarantees pumpUpdates exits.
		close(o.done)
	}()
	if o.closing.Load() {
		procDestroyWindow.Call(uintptr(hwnd))
		return ErrOverlayClosed
	}

	registered, _, registerErr := procRegisterHotKey.Call(uintptr(hwnd), hotkeyID, modControl|modShift|modNoRepeat, vkSpace)
	if registered == 0 {
		procDestroyWindow.Call(uintptr(hwnd))
		return fmt.Errorf("%w: %v", ErrHotkeyUnavailable, registerErr)
	}
	defer func() {
		procUnregisterHotKey.Call(uintptr(hwnd), hotkeyID)
		if exists, _, _ := procIsWindow.Call(uintptr(hwnd)); exists != 0 {
			procDestroyWindow.Call(uintptr(hwnd))
		}
	}()

	go o.pumpUpdates(hwnd)
	procShowWindow.Call(uintptr(hwnd), swShowNoActivate)
	if positioned, _, positionErr := procSetWindowPos.Call(uintptr(hwnd), ^uintptr(0), 24, 24, windowWidth, windowHeight, swpNoActivate); positioned == 0 {
		procDestroyWindow.Call(uintptr(hwnd))
		return fmt.Errorf("position no-activate overlay: %w", positionErr)
	}

	var msg message
	for {
		result, _, messageErr := procGetMessage.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			return fmt.Errorf("GetMessageW: %w", messageErr)
		}
		if result == 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

// Close posts WM_CLOSE without activating or taking foreground focus.
func (o *Overlay) Close() error {
	if !o.closing.CompareAndSwap(false, true) {
		return nil
	}
	if hwnd := o.hwnd.Load(); hwnd != 0 {
		if posted, _, err := procPostMessage.Call(hwnd, wmClose, 0, 0); posted == 0 {
			return fmt.Errorf("post overlay close: %w", err)
		}
	}
	return nil
}

func (o *Overlay) createWindow() (windows.Handle, error) {
	className, _ := windows.UTF16PtrFromString("VoxInkStageOneOverlay")
	title, _ := windows.UTF16PtrFromString("VoxInk")
	instance, _, instanceErr := procGetModuleHandle.Call(0)
	if instance == 0 {
		return 0, fmt.Errorf("GetModuleHandleW: %w", instanceErr)
	}
	class := windowClass{
		size:      uint32(unsafe.Sizeof(windowClass{})),
		wndProc:   windowProc,
		instance:  windows.Handle(instance),
		className: className,
	}
	if atom, _, classErr := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&class))); atom == 0 && classErr != windows.ERROR_CLASS_ALREADY_EXISTS {
		return 0, fmt.Errorf("RegisterClassExW: %w", classErr)
	}
	exStyle := uintptr(wsExNoActivate | wsExToolWindow | wsExLayered | wsExTransparent)
	hwnd, _, createErr := procCreateWindowEx.Call(exStyle, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup, 24, 24, windowWidth, windowHeight, 0, 0, instance, 0)
	if hwnd == 0 {
		return 0, fmt.Errorf("CreateWindowExW: %w", createErr)
	}
	if layered, _, layeredErr := procSetLayeredAttributes.Call(hwnd, 0, 232, lwaAlpha); layered == 0 {
		procDestroyWindow.Call(hwnd)
		return 0, fmt.Errorf("SetLayeredWindowAttributes: %w", layeredErr)
	}
	return windows.Handle(hwnd), nil
}

func (o *Overlay) pumpUpdates(hwnd windows.Handle) {
	for {
		select {
		case view := <-o.updates:
			o.setView(view)
			procPostMessage.Call(uintptr(hwnd), wmAppUpdate, 0, 0)
		case <-o.done:
			return
		}
	}
}

func overlayWindowProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	value, _ := overlayWindows.Load(windows.Handle(hwnd))
	overlay, _ := value.(*Overlay)
	switch message {
	case wmMouseActivate:
		return maNoActivate
	case wmHotkey:
		if overlay != nil && wParam == hotkeyID {
			overlay.emitToggle()
		}
		return 0
	case wmAppUpdate:
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmPaint:
		if overlay != nil {
			overlay.paint(windows.Handle(hwnd))
			return 0
		}
	case wmClose:
		procUnregisterHotKey.Call(hwnd, hotkeyID)
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProc.Call(hwnd, uintptr(message), wParam, lParam)
	return result
}

func (o *Overlay) paint(hwnd windows.Handle) {
	var paint paintStruct
	hdc, _, _ := procBeginPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&paint)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&paint)))

	var bounds rect
	procGetClientRect.Call(uintptr(hwnd), uintptr(unsafe.Pointer(&bounds)))
	background, _, _ := procCreateSolidBrush.Call(rgb(28, 30, 36))
	if background != 0 {
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&bounds)), background)
		procDeleteObject.Call(background)
	}

	view := o.currentView()
	levelBounds := rect{left: 16, top: 104, right: 16 + int32(float64(windowWidth-32)*view.Level), bottom: 122}
	levelBrush, _, _ := procCreateSolidBrush.Call(rgb(72, 180, 112))
	if levelBrush != 0 {
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&levelBounds)), levelBrush)
		procDeleteObject.Call(levelBrush)
	}

	procSetBkMode.Call(hdc, transparent)
	procSetTextColor.Call(hdc, rgb(238, 240, 244))
	text := viewText(view)
	textPtr, _ := windows.UTF16PtrFromString(text)
	textBounds := rect{left: 16, top: 14, right: windowWidth - 16, bottom: 98}
	procDrawText.Call(hdc, uintptr(unsafe.Pointer(textPtr)), ^uintptr(0), uintptr(unsafe.Pointer(&textBounds)), dtLeft|dtWordBreak|dtNoPrefix)
}

func viewText(view View) string {
	status := "Idle"
	switch view.Status {
	case ViewListening:
		status = "Listening"
	case ViewTranscribing:
		status = "Transcribing"
	case ViewError:
		status = "Error"
	}
	text := view.Partial
	if view.Final != "" {
		text = view.Final
	}
	if view.Error != "" {
		text = view.Error
	}
	return status + "\r\n" + text
}

func rgb(red, green, blue byte) uintptr {
	return uintptr(red) | uintptr(green)<<8 | uintptr(blue)<<16
}
