// Package domain contains stable, implementation-independent VoxInk concepts.
package domain

// SessionID identifies one capture and transcription attempt.
// Events must carry this value so that a later session can reject stale events.
type SessionID string

// SessionState describes the lifecycle of a voice input session.
type SessionState uint8

const (
	// SessionIdle has no active voice input session.
	SessionIdle SessionState = iota
	// SessionCapturing accepts audio from the future capture component.
	SessionCapturing
	// SessionTranscribing waits for or receives provider recognition results.
	SessionTranscribing
	// SessionDelivering applies a final result using an output strategy.
	SessionDelivering
	// SessionStopped is a normally ended session.
	SessionStopped
	// SessionFailed is a terminal session that ended with an error.
	SessionFailed
)

// EventKind identifies the stable UI-facing event categories.
type EventKind uint8

const (
	// EventPartial carries provisional recognition text.
	EventPartial EventKind = iota
	// EventFinal carries final recognition text.
	EventFinal
	// EventStopped indicates that capture has stopped.
	EventStopped
	// EventError carries a non-secret diagnostic message.
	EventError
	// EventLevel carries an input-level measurement.
	EventLevel
)

// Event is a provider-neutral notification for a single session.
// Text and Message are intentionally plain fields; transport and UI behavior are
// not implemented by this scaffold.
type Event struct {
	SessionID SessionID
	Kind      EventKind
	Text      string
	Message   string
	Level     float64
}
