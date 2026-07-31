package windows

import (
	"sync"
	"sync/atomic"
)

type captureAction uint8

const (
	actionStart captureAction = iota
	actionStop
	actionClose
)

type captureCommand struct {
	action captureAction
	reply  chan error
}

// Capture is a WASAPI microphone adapter with bounded callback outputs.
type Capture struct {
	ingress   *captureIngress
	commands  chan captureCommand
	format    atomic.Pointer[AudioFormat]
	commandMu sync.Mutex
	closed    bool
}

func newCapture() *Capture {
	return &Capture{
		ingress:  newCaptureIngress(),
		commands: make(chan captureCommand),
	}
}

// PCM returns copied little-endian PCM16 callback buffers.
func (c *Capture) PCM() <-chan []byte { return c.ingress.pcm }

// Levels returns normalized peak levels in the range [0, 1].
func (c *Capture) Levels() <-chan float64 { return c.ingress.levels }

// Errors returns bounded asynchronous capture errors, including ingress overflow.
func (c *Capture) Errors() <-chan error { return c.ingress.errors }

// OverflowCount returns the monotonic number of PCM callback buffers rejected by ingress.
func (c *Capture) OverflowCount() uint64 { return c.ingress.overflow.Load() }

// Format returns callback and backend-negotiated format diagnostics when initialized.
func (c *Capture) Format() (AudioFormat, bool) {
	format := c.format.Load()
	if format == nil {
		return AudioFormat{}, false
	}
	return *format, true
}

// Start starts or idempotently keeps microphone capture active.
func (c *Capture) Start() error { return c.command(actionStart) }

// Stop stops or idempotently keeps microphone capture inactive.
func (c *Capture) Stop() error { return c.command(actionStop) }

// Close stops capture and releases all malgo resources. It is idempotent.
func (c *Capture) Close() error { return c.command(actionClose) }

func (c *Capture) command(action captureAction) error {
	c.commandMu.Lock()
	defer c.commandMu.Unlock()
	if c.closed {
		if action == actionClose || action == actionStop {
			return nil
		}
		return ErrCaptureClosed
	}
	reply := make(chan error, 1)
	c.commands <- captureCommand{action: action, reply: reply}
	err := <-reply
	if action == actionClose {
		c.closed = true
	}
	return err
}

type captureState uint8

const (
	captureStopped captureState = iota
	captureStarted
	captureClosed
)

func transitionCapture(state captureState, action captureAction) (captureState, bool, error) {
	switch action {
	case actionStart:
		if state == captureClosed {
			return state, false, ErrCaptureClosed
		}
		return captureStarted, state != captureStarted, nil
	case actionStop:
		if state == captureClosed {
			return state, false, nil
		}
		return captureStopped, state == captureStarted, nil
	case actionClose:
		if state == captureClosed {
			return state, false, nil
		}
		return captureClosed, true, nil
	default:
		return state, false, nil
	}
}
