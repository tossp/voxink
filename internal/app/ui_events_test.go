package app

import (
	"errors"
	"testing"
	"time"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/domain"
	"github.com/tossp/voxink/internal/history"
	"github.com/tossp/voxink/internal/output"
)

func TestCoordinatorPublishesRuntimeStatesAndHistory(t *testing.T) {
	live := newFakeLiveSession()
	events := make(chan RuntimeEvent, 16)
	store := &fakeHistoryStore{}
	now := time.Date(2026, 8, 5, 12, 30, 0, 0, time.FixedZone("test", 8*60*60))
	harness := startHarnessWithOptions(t, newFakeRecognizer(live), newFakeTranscriber(nil), Options{
		History: store, RuntimeEvents: events, Now: func() time.Time { return now },
	})
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	harness.overlay.toggles <- struct{}{}
	waitSignal(t, live.finished, "live finish")
	live.reads <- fakeLiveRead{event: asr.LiveEvent{Text: "final-canary", ProtocolTerminal: true}}
	waitView(t, harness.overlay, func(view View) bool { return view.Final != "" }, "final")

	got := drainRuntimeEvents(events)
	wantStates := []RuntimeStatus{StatusIdle, StatusCapturing, StatusTranscribing, StatusDelivering, StatusStopped}
	var states []RuntimeStatus
	var entry *history.Entry
	for _, event := range got {
		if event.Status != "" {
			states = append(states, event.Status)
		}
		if event.History != nil {
			entry = event.History
		}
	}
	if len(states) != len(wantStates) {
		t.Fatalf("runtime states = %v, want %v", states, wantStates)
	}
	for index := range wantStates {
		if states[index] != wantStates[index] {
			t.Fatalf("runtime states = %v, want %v", states, wantStates)
		}
	}
	if entry == nil || entry.Provider != domain.ProviderVolcengineV3 || entry.Mode != history.ModeInjected || entry.Final != "final-canary" || !entry.Time.Equal(now.UTC()) {
		t.Fatalf("runtime history event = %+v", entry)
	}
	if len(store.entries) != 1 || store.entries[0] != *entry {
		t.Fatalf("persisted entries = %+v", store.entries)
	}
}

func TestCoordinatorPublishesFailedOutputHistoryWithoutLeakingStoreError(t *testing.T) {
	live := newFakeLiveSession()
	events := make(chan RuntimeEvent, 16)
	store := &fakeHistoryStore{err: errors.New("raw storage error canary")}
	harness := startHarnessWithOptions(t, newFakeRecognizer(live), newFakeTranscriber(nil), Options{
		Output: newFakeOutputAdapter(output.Result{Mode: output.ModeFailed}), History: store, RuntimeEvents: events,
	})
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	harness.overlay.toggles <- struct{}{}
	waitSignal(t, live.finished, "live finish")
	live.reads <- fakeLiveRead{event: asr.LiveEvent{Text: "final", ProtocolTerminal: true}}
	waitView(t, harness.overlay, func(view View) bool { return view.Status == ViewError }, "output failure")

	var failed *history.Entry
	for _, event := range drainRuntimeEvents(events) {
		if event.History != nil {
			failed = event.History
		}
	}
	if failed == nil || failed.Mode != history.ModeFailed || failed.Final != "final" {
		t.Fatalf("failed history = %+v", failed)
	}
}

type fakeHistoryStore struct {
	entries []history.Entry
	err     error
}

func (s *fakeHistoryStore) Append(entry history.Entry) error {
	s.entries = append(s.entries, entry)
	return s.err
}

func drainRuntimeEvents(events <-chan RuntimeEvent) []RuntimeEvent {
	var result []RuntimeEvent
	for {
		select {
		case event := <-events:
			result = append(result, event)
		default:
			return result
		}
	}
}
