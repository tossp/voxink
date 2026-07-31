package session

import (
	"testing"

	"github.com/tossp/voxink/internal/domain"
)

func TestControllerNormalStatePath(t *testing.T) {
	controller := NewController()
	assertState(t, controller, domain.SessionIdle)
	if err := controller.Start("current"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	assertState(t, controller, domain.SessionCapturing)

	if !controller.Handle(domain.Event{SessionID: "current", Kind: domain.EventStopped}) {
		t.Fatal("current EventStopped was rejected")
	}
	assertState(t, controller, domain.SessionTranscribing)
	if !controller.Handle(domain.Event{SessionID: "current", Kind: domain.EventFinal, Text: "完整结果"}) {
		t.Fatal("current EventFinal was rejected")
	}
	assertState(t, controller, domain.SessionDelivering)
	if controller.FinalText() != "完整结果" {
		t.Fatalf("FinalText() = %q", controller.FinalText())
	}
	if !controller.CompleteDelivery("current") {
		t.Fatal("CompleteDelivery() rejected current session")
	}
	assertState(t, controller, domain.SessionStopped)
}

func TestControllerErrorEntersFailedAndNeverStops(t *testing.T) {
	controller := NewController()
	if err := controller.Start("failed"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !controller.Handle(domain.Event{SessionID: "failed", Kind: domain.EventError, Message: "provider failed"}) {
		t.Fatal("EventError was rejected")
	}
	assertState(t, controller, domain.SessionFailed)
	if controller.FailureMessage() != "provider failed" {
		t.Fatalf("FailureMessage() = %q", controller.FailureMessage())
	}
	if controller.CompleteDelivery("failed") {
		t.Fatal("Failed session transitioned to Stopped")
	}
	assertState(t, controller, domain.SessionFailed)
}

func TestControllerRejectsLateSessionEvents(t *testing.T) {
	controller := NewController()
	if err := controller.Start("current"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	late := []domain.Event{
		{SessionID: "old", Kind: domain.EventStopped},
		{SessionID: "old", Kind: domain.EventFinal, Text: "stale"},
		{SessionID: "old", Kind: domain.EventError, Message: "stale failure"},
	}
	for _, event := range late {
		if controller.Handle(event) {
			t.Fatalf("late event %+v was accepted", event)
		}
	}
	assertState(t, controller, domain.SessionCapturing)
	if controller.FinalText() != "" || controller.FailureMessage() != "" {
		t.Fatalf("late event changed results: final=%q failure=%q", controller.FinalText(), controller.FailureMessage())
	}
}

func assertState(t *testing.T, controller *Controller, want domain.SessionState) {
	t.Helper()
	if got := controller.State(); got != want {
		t.Fatalf("State() = %v, want %v", got, want)
	}
}
