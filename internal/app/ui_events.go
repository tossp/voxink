package app

import "github.com/tossp/voxink/internal/history"

// RuntimeStatus is the provider-neutral state shown by the desktop UI.
type RuntimeStatus string

const (
	StatusIdle         RuntimeStatus = "Idle"
	StatusCapturing    RuntimeStatus = "Capturing"
	StatusTranscribing RuntimeStatus = "Transcribing"
	StatusDelivering   RuntimeStatus = "Delivering"
	StatusStopped      RuntimeStatus = "Stopped"
	StatusFailed       RuntimeStatus = "Failed"
)

// RuntimeAction requests a desktop window action without importing a UI package.
type RuntimeAction uint8

const (
	ActionNone RuntimeAction = iota
	ActionOpenMain
	ActionOpenSettings
	ActionQuit
)

// RuntimeEvent carries one state, history, or window event to a UI channel.
type RuntimeEvent struct {
	Status  RuntimeStatus
	History *history.Entry
	Action  RuntimeAction
}

func sendRuntimeEvent(target chan<- RuntimeEvent, event RuntimeEvent) {
	if target == nil {
		return
	}
	select {
	case target <- event:
	default:
	}
}
