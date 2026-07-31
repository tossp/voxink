package windows

import (
	"encoding/binary"
	"errors"
	"math"
	"strings"
	"testing"
)

func TestNormalizeViewBoundsStateTextAndLevel(t *testing.T) {
	view := normalizeView(View{
		Status:  ViewStatus(99),
		Level:   math.Inf(1),
		Partial: strings.Repeat("界", maxViewRunes+20),
		Final:   strings.Repeat("f", maxViewRunes+20),
		Error:   strings.Repeat("e", maxViewRunes+20),
	})
	if view.Status != ViewError {
		t.Fatalf("Status = %v, want ViewError", view.Status)
	}
	if view.Level != 1 {
		t.Fatalf("Level = %v, want 1", view.Level)
	}
	for name, text := range map[string]string{"partial": view.Partial, "final": view.Final, "error": view.Error} {
		if got := len([]rune(text)); got != maxViewRunes {
			t.Errorf("%s runes = %d, want %d", name, got, maxViewRunes)
		}
		if !strings.HasSuffix(text, "…") {
			t.Errorf("%s does not have truncation marker", name)
		}
	}
}

func TestCaptureIngressCopiesAndReportsOverflow(t *testing.T) {
	ingress := newCaptureIngress()
	original := []byte{1, 2, 3, 4}
	ingress.accept(original)
	original[0] = 99
	if got := <-ingress.pcm; got[0] != 1 {
		t.Fatalf("callback buffer was retained: first byte = %d", got[0])
	}

	for range cap(ingress.pcm) {
		ingress.accept([]byte{0, 0})
	}
	done := make(chan struct{})
	go func() {
		ingress.accept([]byte{0, 0})
		close(done)
	}()
	<-done
	if ingress.overflow.Load() != 1 {
		t.Fatalf("overflow count = %d, want 1", ingress.overflow.Load())
	}
	if err := <-ingress.errors; !errors.Is(err, ErrPCMOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestPCM16Level(t *testing.T) {
	pcm := make([]byte, 8)
	binary.LittleEndian.PutUint16(pcm[0:2], uint16(int16(0)))
	binary.LittleEndian.PutUint16(pcm[2:4], uint16(int16(8192)))
	binary.LittleEndian.PutUint16(pcm[4:6], 0xC000)
	binary.LittleEndian.PutUint16(pcm[6:8], uint16(int16(32767)))
	if got, want := pcm16Level(pcm), float64(32767)/32768; math.Abs(got-want) > 1e-9 {
		t.Fatalf("level = %v, want %v", got, want)
	}
	if got := pcm16Level([]byte{0}); got != 0 {
		t.Fatalf("short level = %v, want 0", got)
	}
}

func TestCaptureStateMachineIdempotence(t *testing.T) {
	state, operate, err := transitionCapture(captureStopped, actionStart)
	if err != nil || state != captureStarted || !operate {
		t.Fatalf("first Start = (%v, %v, %v)", state, operate, err)
	}
	state, operate, err = transitionCapture(state, actionStart)
	if err != nil || state != captureStarted || operate {
		t.Fatalf("second Start = (%v, %v, %v)", state, operate, err)
	}
	state, operate, err = transitionCapture(state, actionStop)
	if err != nil || state != captureStopped || !operate {
		t.Fatalf("Stop = (%v, %v, %v)", state, operate, err)
	}
	state, operate, err = transitionCapture(state, actionClose)
	if err != nil || state != captureClosed || !operate {
		t.Fatalf("Close = (%v, %v, %v)", state, operate, err)
	}
	_, operate, err = transitionCapture(state, actionStart)
	if !errors.Is(err, ErrCaptureClosed) || operate {
		t.Fatalf("Start after Close = (%v, %v)", operate, err)
	}
}

func TestToggleIngressIsNonBlocking(t *testing.T) {
	overlay := NewOverlay()
	overlay.emitToggle()
	done := make(chan struct{})
	go func() {
		overlay.emitToggle()
		close(done)
	}()
	<-done
	select {
	case <-overlay.Toggles():
	default:
		t.Fatal("first toggle was not retained")
	}
}

func TestNoCGOAvailabilityError(t *testing.T) {
	if !strings.Contains(ErrCGODisabled.Error(), "CGO_ENABLED=1") {
		t.Fatalf("ErrCGODisabled = %q", ErrCGODisabled)
	}
}
