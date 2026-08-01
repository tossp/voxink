// Package output defines provider- and platform-neutral final-text delivery.
package output

import "errors"

var (
	// ErrAlreadyDelivered reports a second delivery attempt for one output session.
	ErrAlreadyDelivered = errors.New("final text was already delivered")
	// ErrPartialSend reports that only part of the Unicode input sequence was sent.
	ErrPartialSend = errors.New("Unicode input was partially sent")
	// ErrClipboardFailed reports that Copy Only could not retain the complete final text.
	ErrClipboardFailed = errors.New("clipboard copy failed")
	// ErrInvalidText reports final text that cannot be represented safely.
	ErrInvalidText = errors.New("final text cannot be delivered safely")
)

// Mode describes the fixed result of one final-text delivery attempt.
type Mode uint8

const (
	// ModeFailed indicates that neither safe injection nor Copy Only completed.
	ModeFailed Mode = iota
	// ModeInjected indicates that the complete final text was sent as Unicode input.
	ModeInjected
	// ModeCopied indicates Copy Only; the user must paste manually.
	ModeCopied
)

// Result is the non-sensitive outcome of final-text delivery.
type Result struct {
	Mode Mode
	Err  error
}

// Session captures the output target at recognition-session start.
type Session interface {
	Deliver(text string) Result
}

// Adapter prepares one output session for each recognition session.
type Adapter interface {
	StartSession() Session
}
