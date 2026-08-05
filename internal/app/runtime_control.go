package app

import (
	"sync"

	platformwindows "github.com/tossp/voxink/internal/platform/windows"
)

type runtimeController interface {
	RequestToggle()
	UpdateHotkey(platformwindows.Hotkey) error
}

// RuntimeControl safely forwards UI commands to the current Windows runtime.
type RuntimeControl struct {
	mu       sync.RWMutex
	updateMu sync.Mutex
	target   runtimeController
	pending  *platformwindows.Hotkey
}

// NewRuntimeControl creates a detached command bridge.
func NewRuntimeControl() *RuntimeControl { return &RuntimeControl{} }

// Toggle requests start or stop when a runtime is attached.
func (c *RuntimeControl) Toggle() {
	c.mu.RLock()
	target := c.target
	c.mu.RUnlock()
	if target != nil {
		target.RequestToggle()
	}
}

// UpdateHotkey validates and applies a shortcut, or retains it for the next runtime.
func (c *RuntimeControl) UpdateHotkey(raw string) error {
	c.updateMu.Lock()
	defer c.updateMu.Unlock()
	hotkey, err := platformwindows.ParseHotkey(raw)
	if err != nil {
		return platformwindows.ErrInvalidHotkey
	}
	c.mu.Lock()
	target := c.target
	if target == nil {
		c.pending = &hotkey
		c.mu.Unlock()
		return nil
	}
	c.mu.Unlock()
	if err := target.UpdateHotkey(hotkey); err != nil {
		return err
	}
	c.mu.Lock()
	c.pending = nil
	c.mu.Unlock()
	return nil
}

func (c *RuntimeControl) attach(target runtimeController) error {
	c.updateMu.Lock()
	defer c.updateMu.Unlock()
	c.mu.Lock()
	c.target = target
	pending := c.pending
	c.mu.Unlock()
	if pending != nil {
		if err := target.UpdateHotkey(*pending); err != nil {
			c.mu.Lock()
			if c.target == target {
				c.target = nil
			}
			c.mu.Unlock()
			return err
		}
		c.mu.Lock()
		c.pending = nil
		c.mu.Unlock()
	}
	return nil
}

func (c *RuntimeControl) detach(target runtimeController) {
	c.mu.Lock()
	if c.target == target {
		c.target = nil
	}
	c.mu.Unlock()
}
