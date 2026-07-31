// Package windows provides the stage-one Windows capture and no-activate UI adapters.
package windows

import (
	"errors"
	"math"
	"sync"
	"sync/atomic"
	"unicode/utf8"
)

const (
	pcmQueueSize    = 32
	levelQueueSize  = 8
	errorQueueSize  = 8
	updateQueueSize = 8
	toggleQueueSize = 1
	maxViewRunes    = 512
	maxNoticeRunes  = 256
)

var (
	// ErrUnsupportedPlatform reports use of a Windows adapter on another OS.
	ErrUnsupportedPlatform = errors.New("Windows platform adapter is unavailable on this OS")
	// ErrCGODisabled reports that malgo capture requires Windows cgo.
	ErrCGODisabled = errors.New("Windows microphone capture requires CGO_ENABLED=1")
	// ErrCaptureClosed reports a capture operation after Close.
	ErrCaptureClosed = errors.New("capture is closed")
	// ErrPCMOverflow reports that bounded PCM ingress rejected a callback buffer.
	ErrPCMOverflow = errors.New("PCM ingress overflow")
	// ErrHotkeyUnavailable reports that Ctrl+Shift+Space could not be registered.
	ErrHotkeyUnavailable = errors.New("Ctrl+Shift+Space global hotkey is unavailable")
	// ErrOverlayClosed reports Run after the overlay was closed.
	ErrOverlayClosed = errors.New("overlay is closed")
	// ErrOverlayRunning reports a second concurrent Run call.
	ErrOverlayRunning = errors.New("overlay is already running")
)

// AudioFormat is a non-sensitive description of capture conversion and backend format.
type AudioFormat struct {
	CallbackFormat     string
	CallbackChannels   uint32
	CallbackSampleRate uint32
	InternalFormat     string
	InternalChannels   uint32
	InternalSampleRate uint32
}

// ViewStatus is one of the four states rendered by the stage-one overlay.
type ViewStatus uint8

const (
	// ViewIdle indicates no active capture.
	ViewIdle ViewStatus = iota
	// ViewListening indicates active microphone capture.
	ViewListening
	// ViewTranscribing indicates recognition after capture stops.
	ViewTranscribing
	// ViewError indicates a non-secret failure message.
	ViewError
)

// View is the complete bounded state rendered by the overlay.
type View struct {
	Status  ViewStatus
	Level   float64
	Partial string
	Final   string
	Error   string
	Notice  string
}

type captureIngress struct {
	pcm      chan []byte
	levels   chan float64
	errors   chan error
	overflow atomic.Uint64
}

func newCaptureIngress() *captureIngress {
	return &captureIngress{
		pcm:    make(chan []byte, pcmQueueSize),
		levels: make(chan float64, levelQueueSize),
		errors: make(chan error, errorQueueSize),
	}
}

func (i *captureIngress) accept(input []byte) {
	pcm := append([]byte(nil), input...)
	level := pcm16Level(pcm)
	select {
	case i.pcm <- pcm:
	default:
		i.overflow.Add(1)
		i.report(ErrPCMOverflow)
	}
	select {
	case i.levels <- level:
	default:
	}
}

func (i *captureIngress) report(err error) {
	select {
	case i.errors <- err:
	default:
	}
}

func pcm16Level(pcm []byte) float64 {
	if len(pcm) < 2 {
		return 0
	}
	var peak int32
	for index := 0; index+1 < len(pcm); index += 2 {
		sample := int32(int16(uint16(pcm[index]) | uint16(pcm[index+1])<<8))
		if sample < 0 {
			sample = -sample
		}
		if sample > peak {
			peak = sample
		}
	}
	return math.Min(float64(peak)/32768, 1)
}

func normalizeView(view View) View {
	if math.IsNaN(view.Level) || view.Level < 0 {
		view.Level = 0
	} else if view.Level > 1 {
		view.Level = 1
	}
	if view.Status > ViewError {
		view.Status = ViewError
	}
	view.Partial = truncateRunes(view.Partial, maxViewRunes)
	view.Final = truncateRunes(view.Final, maxViewRunes)
	view.Error = truncateRunes(view.Error, maxViewRunes)
	view.Notice = truncateRunes(view.Notice, maxNoticeRunes)
	return view
}

func truncateRunes(text string, limit int) string {
	if utf8.RuneCountInString(text) <= limit {
		return text
	}
	runes := []rune(text)
	return string(runes[:limit-1]) + "…"
}

// Overlay owns the stage-one no-activate window and its global toggle hotkey.
type Overlay struct {
	updates chan View
	toggles chan struct{}
	done    chan struct{}

	viewMu sync.RWMutex
	view   View

	running atomic.Bool
	closing atomic.Bool
	hwnd    atomic.Uintptr
}

// NewOverlay creates an idle overlay adapter. Run performs platform initialization.
func NewOverlay() *Overlay {
	return &Overlay{
		updates: make(chan View, updateQueueSize),
		toggles: make(chan struct{}, toggleQueueSize),
		done:    make(chan struct{}),
		view:    View{Status: ViewIdle},
	}
}

// Updates returns the bounded view-state input consumed while Run is active.
func (o *Overlay) Updates() chan<- View { return o.updates }

// Update non-blockingly retains a recent complete view for the UI thread.
func (o *Overlay) Update(view View) {
	view = normalizeView(view)
	select {
	case o.updates <- view:
		return
	default:
	}
	select {
	case <-o.updates:
	default:
	}
	select {
	case o.updates <- view:
	default:
	}
}

// Toggles returns non-blocking Ctrl+Shift+Space requests from WM_HOTKEY.
func (o *Overlay) Toggles() <-chan struct{} { return o.toggles }

func (o *Overlay) setView(view View) {
	o.viewMu.Lock()
	o.view = normalizeView(view)
	o.viewMu.Unlock()
}

func (o *Overlay) currentView() View {
	o.viewMu.RLock()
	defer o.viewMu.RUnlock()
	return o.view
}

func (o *Overlay) emitToggle() {
	select {
	case o.toggles <- struct{}{}:
	default:
	}
}
