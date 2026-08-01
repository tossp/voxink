package windows

import (
	"strings"
	"sync"
	"unicode/utf16"

	textoutput "github.com/tossp/voxink/internal/output"
)

const maxUnicodeInputUnits = 16384

type windowHandle uintptr
type globalHandle uintptr

type unicodeInput struct {
	codeUnit uint16
	keyUp    bool
}

type outputBackend interface {
	foregroundWindow() windowHandle
	safeFocusedControl(windowHandle) bool
	sendInput([]unicodeInput) uint32
	openClipboard() bool
	emptyClipboard() bool
	globalAlloc(uintptr) globalHandle
	writeGlobal(globalHandle, []uint16) bool
	setClipboardData(globalHandle) bool
	globalFree(globalHandle)
	closeClipboard()
}

// TextOutput captures and verifies a Windows foreground target before delivery.
type TextOutput struct {
	backend outputBackend
}

func newTextOutput(backend outputBackend) *TextOutput {
	return &TextOutput{backend: backend}
}

// StartSession captures the current foreground window without retaining its metadata elsewhere.
func (o *TextOutput) StartSession() textoutput.Session {
	return &outputSession{backend: o.backend, target: o.backend.foregroundWindow()}
}

type outputSession struct {
	mu        sync.Mutex
	backend   outputBackend
	target    windowHandle
	delivered bool
}

func (s *outputSession) Deliver(text string) textoutput.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.delivered {
		return textoutput.Result{Mode: textoutput.ModeFailed, Err: textoutput.ErrAlreadyDelivered}
	}
	s.delivered = true
	if strings.IndexByte(text, 0) >= 0 {
		return textoutput.Result{Mode: textoutput.ModeFailed, Err: textoutput.ErrInvalidText}
	}

	units := utf16.Encode([]rune(text))
	if len(units) == 0 {
		return textoutput.Result{Mode: textoutput.ModeInjected}
	}
	if s.target == 0 || s.backend.foregroundWindow() != s.target ||
		len(units) > maxUnicodeInputUnits || !s.backend.safeFocusedControl(s.target) ||
		s.backend.foregroundWindow() != s.target {
		return s.copyOnly(units)
	}

	inputs := make([]unicodeInput, 0, len(units)*2)
	for _, unit := range units {
		inputs = append(inputs, unicodeInput{codeUnit: unit}, unicodeInput{codeUnit: unit, keyUp: true})
	}
	sent := s.backend.sendInput(inputs)
	if sent == uint32(len(inputs)) {
		return textoutput.Result{Mode: textoutput.ModeInjected}
	}
	if sent > 0 {
		return textoutput.Result{Mode: textoutput.ModeFailed, Err: textoutput.ErrPartialSend}
	}
	return s.copyOnly(units)
}

func (s *outputSession) copyOnly(units []uint16) textoutput.Result {
	if !s.backend.openClipboard() {
		return textoutput.Result{Mode: textoutput.ModeFailed, Err: textoutput.ErrClipboardFailed}
	}
	defer s.backend.closeClipboard()
	if !s.backend.emptyClipboard() {
		return textoutput.Result{Mode: textoutput.ModeFailed, Err: textoutput.ErrClipboardFailed}
	}

	terminated := append(append([]uint16(nil), units...), 0)
	handle := s.backend.globalAlloc(uintptr(len(terminated)) * 2)
	if handle == 0 {
		return textoutput.Result{Mode: textoutput.ModeFailed, Err: textoutput.ErrClipboardFailed}
	}
	owned := true
	defer func() {
		if owned {
			s.backend.globalFree(handle)
		}
	}()
	if !s.backend.writeGlobal(handle, terminated) || !s.backend.setClipboardData(handle) {
		return textoutput.Result{Mode: textoutput.ModeFailed, Err: textoutput.ErrClipboardFailed}
	}
	owned = false
	return textoutput.Result{Mode: textoutput.ModeCopied}
}
