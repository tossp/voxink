package app

import (
	"context"
	"reflect"
	"testing"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/audio"
	"github.com/tossp/voxink/internal/diagnostic"
	"github.com/tossp/voxink/internal/domain"
	"github.com/tossp/voxink/internal/output"
	"github.com/tossp/voxink/internal/session"
)

func TestCoordinatorPartialNeverReachesOutputAndFinalIsDeliveredOnce(t *testing.T) {
	live := newFakeLiveSession()
	adapter := newFakeOutputAdapter(output.Result{Mode: output.ModeInjected})
	harness := startHarnessWithOptions(t, newFakeRecognizer(live), newFakeTranscriber(nil), Options{Output: adapter})
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	live.reads <- fakeLiveRead{event: asr.LiveEvent{Text: "partial only"}}
	waitView(t, harness.overlay, func(view View) bool { return view.Partial == "partial only" }, "partial")
	if starts, texts := adapter.snapshot(); starts != 1 || len(texts) != 0 {
		t.Fatalf("before Final output starts=%d texts=%q, want 1/0", starts, texts)
	}

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, live.finished, "live finish")
	live.reads <- fakeLiveRead{event: asr.LiveEvent{Text: "complete final", ProtocolTerminal: true}}
	view := waitView(t, harness.overlay, func(view View) bool { return view.Final != "" }, "output result")
	if view.Final != OutputInjectedMessage {
		t.Fatalf("Final status = %q, want %q", view.Final, OutputInjectedMessage)
	}
	if starts, texts := adapter.snapshot(); starts != 1 || !reflect.DeepEqual(texts, []string{"complete final"}) {
		t.Fatalf("output starts=%d texts=%q", starts, texts)
	}
}

func TestCoordinatorShowsCopyOnlyAndFailsOnOutputFailure(t *testing.T) {
	tests := []struct {
		name       string
		result     output.Result
		wantStatus ViewStatus
		wantFinal  string
		wantError  string
		wantState  domain.SessionState
	}{
		{name: "copied", result: output.Result{Mode: output.ModeCopied}, wantStatus: ViewIdle, wantFinal: OutputCopiedMessage, wantState: domain.SessionStopped},
		{name: "failed", result: output.Result{Mode: output.ModeFailed, Err: output.ErrClipboardFailed}, wantStatus: ViewError, wantError: "Output failed", wantState: domain.SessionFailed},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			live := newFakeLiveSession()
			adapter := newFakeOutputAdapter(test.result)
			harness := startHarnessWithOptions(t, newFakeRecognizer(live), newFakeTranscriber(nil), Options{Output: adapter})
			defer harness.close(t)

			harness.overlay.toggles <- struct{}{}
			waitSignal(t, harness.capture.started, "capture start")
			harness.overlay.toggles <- struct{}{}
			waitSignal(t, live.finished, "live finish")
			live.reads <- fakeLiveRead{event: asr.LiveEvent{Text: "secret final", ProtocolTerminal: true}}
			view := waitView(t, harness.overlay, func(view View) bool { return view.Status == test.wantStatus && (view.Final != "" || view.Error != "") }, "delivery state")
			if view.Final != test.wantFinal || view.Error != test.wantError {
				t.Fatalf("view = %+v, want final=%q error=%q", view, test.wantFinal, test.wantError)
			}
			if harness.coordinator.controller.State() != test.wantState {
				t.Fatalf("controller state = %v, want %v", harness.coordinator.controller.State(), test.wantState)
			}
			if _, texts := adapter.snapshot(); !reflect.DeepEqual(texts, []string{"secret final"}) {
				t.Fatalf("output texts = %q", texts)
			}
		})
	}
}

func TestCoordinatorRejectsDuplicateStaleAndCanceledOutput(t *testing.T) {
	adapter := newFakeOutputAdapter(output.Result{Mode: output.ModeInjected})
	controller := session.NewController()
	if err := controller.Start("current"); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if !controller.Handle(domain.Event{SessionID: "current", Kind: domain.EventStopped}) {
		t.Fatal("EventStopped rejected")
	}
	ctx, cancel := context.WithCancel(context.Background())
	current := &activeSession{
		id: "current", ctx: ctx, cancel: cancel, segmenter: audio.NewProgressiveSegmenter(),
		output: adapter.StartSession(), liveHealthy: true,
	}
	coordinator := &Coordinator{
		active: current, controller: controller, capture: newFakeCapture(), overlay: newFakeOverlay(),
		diagnostics: diagnostic.NoopSink(),
	}

	coordinator.handleWorker(workerEvent{kind: workerLiveEvent, sessionID: "stale", live: asr.LiveEvent{Text: "stale", ProtocolTerminal: true}})
	coordinator.handleWorker(workerEvent{kind: workerLiveEvent, sessionID: "current", live: asr.LiveEvent{Text: "partial"}})
	if _, texts := adapter.snapshot(); len(texts) != 0 {
		t.Fatalf("stale/partial output texts = %q", texts)
	}

	coordinator.finalize(current, "final")
	coordinator.finalize(current, "duplicate")
	if _, texts := adapter.snapshot(); !reflect.DeepEqual(texts, []string{"final"}) {
		t.Fatalf("duplicate output texts = %q", texts)
	}

	ctx2, cancel2 := context.WithCancel(context.Background())
	canceled := &activeSession{id: "canceled", ctx: ctx2, cancel: cancel2, segmenter: audio.NewProgressiveSegmenter(), output: adapter.StartSession()}
	coordinator.active = canceled
	coordinator.cleanupSession(canceled)
	coordinator.handleWorker(workerEvent{kind: workerLiveEvent, sessionID: "canceled", live: asr.LiveEvent{Text: "late", ProtocolTerminal: true}})
	if _, texts := adapter.snapshot(); !reflect.DeepEqual(texts, []string{"final"}) {
		t.Fatalf("canceled output texts = %q", texts)
	}
}
