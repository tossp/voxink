package windows

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	textoutput "github.com/tossp/voxink/internal/output"
)

func TestTextOutputSendsCompleteUTF16ToUnchangedSafeTarget(t *testing.T) {
	backend := newFakeOutputBackend()
	adapter := newTextOutput(backend)
	session := adapter.StartSession()

	result := session.Deliver("A😀文")

	if result.Mode != textoutput.ModeInjected || result.Err != nil {
		t.Fatalf("Deliver() = %+v, want injected", result)
	}
	units := utf16.Encode([]rune("A😀文"))
	want := make([]unicodeInput, 0, len(units)*2)
	for _, unit := range units {
		want = append(want, unicodeInput{codeUnit: unit}, unicodeInput{codeUnit: unit, keyUp: true})
	}
	if !reflect.DeepEqual(backend.inputs, want) {
		t.Fatalf("SendInput events = %+v, want %+v", backend.inputs, want)
	}
	assertNoClipboard(t, backend)
}

func TestTextOutputEmptyFinalNeedsNoWin32Write(t *testing.T) {
	backend := newFakeOutputBackend()
	result := newTextOutput(backend).StartSession().Deliver("")
	if result.Mode != textoutput.ModeInjected || len(backend.inputs) != 0 {
		t.Fatalf("empty Deliver() = %+v, inputs=%v", result, backend.inputs)
	}
	assertNoClipboard(t, backend)
}

func TestTextOutputFallsBackToCompleteCopyOnly(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*fakeOutputBackend, textoutput.Session)
	}{
		{name: "foreground changed", setup: func(backend *fakeOutputBackend, _ textoutput.Session) { backend.foreground = 22 }},
		{name: "no captured target", setup: func(backend *fakeOutputBackend, _ textoutput.Session) { backend.foreground = 0 }},
		{name: "password or unsafe focus", setup: func(backend *fakeOutputBackend, _ textoutput.Session) { backend.safe = false }},
		{name: "SendInput returned zero", setup: func(backend *fakeOutputBackend, _ textoutput.Session) { backend.sendCount = 0 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newFakeOutputBackend()
			if test.name == "no captured target" {
				backend.foreground = 0
			}
			session := newTextOutput(backend).StartSession()
			test.setup(backend, session)
			result := session.Deliver("完整😀Final")
			if result.Mode != textoutput.ModeCopied || result.Err != nil {
				t.Fatalf("Deliver() = %+v, want copied", result)
			}
			want := append(utf16.Encode([]rune("完整😀Final")), 0)
			if !reflect.DeepEqual(backend.written, want) {
				t.Fatalf("clipboard UTF-16 = %v, want %v", backend.written, want)
			}
			if backend.closeCalls != 1 || backend.freeCalls != 0 {
				t.Fatalf("clipboard cleanup close=%d free=%d, want 1/0", backend.closeCalls, backend.freeCalls)
			}
		})
	}
}

func TestTextOutputPartialSendFailsWithoutCopy(t *testing.T) {
	backend := newFakeOutputBackend()
	backend.sendCount = 1
	result := newTextOutput(backend).StartSession().Deliver("partial")
	if result.Mode != textoutput.ModeFailed || !errors.Is(result.Err, textoutput.ErrPartialSend) {
		t.Fatalf("Deliver() = %+v, want partial-send failure", result)
	}
	assertNoClipboard(t, backend)
}

func TestTextOutputClipboardFailuresFreeOwnedMemory(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(*fakeOutputBackend)
		wantFree int
		wantOpen bool
	}{
		{name: "open", setup: func(backend *fakeOutputBackend) { backend.openOK = false }},
		{name: "empty", setup: func(backend *fakeOutputBackend) { backend.emptyOK = false }, wantOpen: true},
		{name: "alloc", setup: func(backend *fakeOutputBackend) { backend.allocHandle = 0 }, wantOpen: true},
		{name: "lock or copy", setup: func(backend *fakeOutputBackend) { backend.writeOK = false }, wantOpen: true, wantFree: 1},
		{name: "SetClipboardData", setup: func(backend *fakeOutputBackend) { backend.setOK = false }, wantOpen: true, wantFree: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newFakeOutputBackend()
			backend.safe = false
			test.setup(backend)
			result := newTextOutput(backend).StartSession().Deliver("copy me")
			if result.Mode != textoutput.ModeFailed || !errors.Is(result.Err, textoutput.ErrClipboardFailed) {
				t.Fatalf("Deliver() = %+v, want clipboard failure", result)
			}
			if backend.freeCalls != test.wantFree {
				t.Fatalf("GlobalFree calls = %d, want %d", backend.freeCalls, test.wantFree)
			}
			wantClose := 0
			if test.wantOpen {
				wantClose = 1
			}
			if backend.closeCalls != wantClose {
				t.Fatalf("CloseClipboard calls = %d, want %d", backend.closeCalls, wantClose)
			}
		})
	}
}

func TestTextOutputBoundsInjectionAndRejectsRepeat(t *testing.T) {
	backend := newFakeOutputBackend()
	session := newTextOutput(backend).StartSession()
	result := session.Deliver(strings.Repeat("x", maxUnicodeInputUnits+1))
	if result.Mode != textoutput.ModeCopied || len(backend.inputs) != 0 {
		t.Fatalf("oversized Deliver() = %+v, inputs=%d", result, len(backend.inputs))
	}
	result = session.Deliver("again")
	if result.Mode != textoutput.ModeFailed || !errors.Is(result.Err, textoutput.ErrAlreadyDelivered) {
		t.Fatalf("second Deliver() = %+v", result)
	}
}

func assertNoClipboard(t *testing.T, backend *fakeOutputBackend) {
	t.Helper()
	if backend.openCalls != 0 || backend.emptyCalls != 0 || backend.setCalls != 0 || backend.closeCalls != 0 {
		t.Fatalf("unexpected clipboard calls: %+v", backend)
	}
}

type fakeOutputBackend struct {
	foreground  windowHandle
	safe        bool
	sendCount   int
	inputs      []unicodeInput
	openOK      bool
	emptyOK     bool
	allocHandle globalHandle
	writeOK     bool
	setOK       bool
	written     []uint16
	openCalls   int
	emptyCalls  int
	setCalls    int
	closeCalls  int
	freeCalls   int
}

func newFakeOutputBackend() *fakeOutputBackend {
	return &fakeOutputBackend{
		foreground: 11, safe: true, sendCount: -1, openOK: true, emptyOK: true,
		allocHandle: 99, writeOK: true, setOK: true,
	}
}

func (b *fakeOutputBackend) foregroundWindow() windowHandle       { return b.foreground }
func (b *fakeOutputBackend) safeFocusedControl(windowHandle) bool { return b.safe }
func (b *fakeOutputBackend) sendInput(inputs []unicodeInput) uint32 {
	b.inputs = append([]unicodeInput(nil), inputs...)
	if b.sendCount < 0 {
		return uint32(len(inputs))
	}
	return uint32(b.sendCount)
}
func (b *fakeOutputBackend) openClipboard() bool              { b.openCalls++; return b.openOK }
func (b *fakeOutputBackend) emptyClipboard() bool             { b.emptyCalls++; return b.emptyOK }
func (b *fakeOutputBackend) globalAlloc(uintptr) globalHandle { return b.allocHandle }
func (b *fakeOutputBackend) writeGlobal(_ globalHandle, units []uint16) bool {
	b.written = append([]uint16(nil), units...)
	return b.writeOK
}
func (b *fakeOutputBackend) setClipboardData(globalHandle) bool { b.setCalls++; return b.setOK }
func (b *fakeOutputBackend) globalFree(globalHandle)            { b.freeCalls++ }
func (b *fakeOutputBackend) closeClipboard()                    { b.closeCalls++ }
