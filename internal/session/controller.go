// Package session provides stable session-state transitions and event gating.
package session

import (
	"errors"

	"github.com/tossp/voxink/internal/domain"
)

var (
	// ErrSessionActive reports Start while the controller is not idle.
	ErrSessionActive = errors.New("session controller is not idle")
	// ErrEmptySessionID reports an attempt to start an unidentified session.
	ErrEmptySessionID = errors.New("session ID must not be empty")
)

// Controller gates external events and enforces the fixed six-state lifecycle.
type Controller struct {
	id             domain.SessionID
	state          domain.SessionState
	finalText      string
	failureMessage string
}

// NewController creates an idle session controller.
func NewController() *Controller {
	return &Controller{state: domain.SessionIdle}
}

// Start begins capture for a caller-provided new session ID.
func (c *Controller) Start(id domain.SessionID) error {
	if id == "" {
		return ErrEmptySessionID
	}
	if c.state != domain.SessionIdle {
		return ErrSessionActive
	}
	c.id = id
	c.state = domain.SessionCapturing
	return nil
}

// Handle accepts only current-session events valid for the current state.
func (c *Controller) Handle(event domain.Event) bool {
	if event.SessionID != c.id || c.id == "" || c.isTerminal() {
		return false
	}
	if event.Kind == domain.EventError {
		c.failureMessage = event.Message
		c.state = domain.SessionFailed
		return true
	}

	switch c.state {
	case domain.SessionCapturing:
		switch event.Kind {
		case domain.EventPartial, domain.EventLevel:
			return true
		case domain.EventStopped:
			c.state = domain.SessionTranscribing
			return true
		}
	case domain.SessionTranscribing:
		switch event.Kind {
		case domain.EventPartial:
			return true
		case domain.EventFinal:
			c.finalText = event.Text
			c.state = domain.SessionDelivering
			return true
		}
	}
	return false
}

// CompleteDelivery marks successful delivery for the current session.
func (c *Controller) CompleteDelivery(id domain.SessionID) bool {
	if id != c.id || c.state != domain.SessionDelivering {
		return false
	}
	c.state = domain.SessionStopped
	return true
}

// State returns the current stable session state.
func (c *Controller) State() domain.SessionState {
	return c.state
}

// SessionID returns the active or terminal session identifier.
func (c *Controller) SessionID() domain.SessionID {
	return c.id
}

// FinalText returns the accepted complete final text.
func (c *Controller) FinalText() string {
	return c.finalText
}

// FailureMessage returns the non-secret message from the accepted error event.
func (c *Controller) FailureMessage() string {
	return c.failureMessage
}

func (c *Controller) isTerminal() bool {
	return c.state == domain.SessionStopped || c.state == domain.SessionFailed
}
