package app

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tossp/voxink/internal/asr"
)

func TestLiveSuccessWaitsForProtocolTerminalAndSkipsMiMo(t *testing.T) {
	live := newFakeLiveSession()
	recognizer := newFakeRecognizer(live)
	batch := newFakeTranscriber(nil)
	harness := startHarness(t, recognizer, batch)
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	live.reads <- fakeLiveRead{event: asr.LiveEvent{Utterances: []asr.LiveUtterance{{
		Text: "临时", StartMS: 0, EndMS: 100, Stable: true,
	}}}}
	waitView(t, harness.overlay, func(view View) bool { return view.Partial == "临时" }, "live partial")
	if len(finalViews(harness.overlay)) != 0 {
		t.Fatal("Stable/partial text became Final before stop and terminal")
	}

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.stopped, "capture stop")
	waitSignal(t, live.finished, "live FinishInput")
	if len(finalViews(harness.overlay)) != 0 {
		t.Fatal("capture stop became Final before protocol terminal")
	}
	live.reads <- fakeLiveRead{event: asr.LiveEvent{Text: "完整结果", ProtocolTerminal: true}}
	final := waitView(t, harness.overlay, func(view View) bool { return view.Final == "完整结果" }, "live final")
	if final.Partial != "" {
		t.Fatalf("final view retained provisional partial %q", final.Partial)
	}
	if got := batch.count.Load(); got != 0 {
		t.Fatalf("MiMo calls = %d, want 0", got)
	}
	if got := len(finalViews(harness.overlay)); got != 1 {
		t.Fatalf("Final updates = %d, want 1", got)
	}
}

func TestNewCoordinatorRequiresBothStageOneProviders(t *testing.T) {
	capture := newFakeCapture()
	overlay := newFakeOverlay()
	live := newFakeRecognizer(newFakeLiveSession())
	batch := newFakeTranscriber(nil)

	if _, err := NewCoordinator(capture, overlay, nil, batch, Options{}); !errors.Is(err, ErrMissingLiveRecognizer) {
		t.Fatalf("missing live error = %v, want %v", err, ErrMissingLiveRecognizer)
	}
	if _, err := NewCoordinator(capture, overlay, live, nil, Options{}); !errors.Is(err, ErrMissingBatchTranscriber) {
		t.Fatalf("missing batch error = %v, want %v", err, ErrMissingBatchTranscriber)
	}
}

func TestDialFailureStartsBatchBeforeCaptureStops(t *testing.T) {
	firstStarted := make(chan struct{})
	release := make(chan struct{})
	batch := newFakeTranscriber(func(ctx context.Context, _ []byte, index int) (string, error) {
		if index == 0 {
			close(firstStarted)
			select {
			case <-release:
			case <-ctx.Done():
				return "", ctx.Err()
			}
		}
		return "段", nil
	})
	recognizer := newFakeRecognizer()
	recognizer.dialErr = errors.New("local dial failure")
	harness := startHarness(t, recognizer, batch)
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start after live dial failure")
	harness.capture.pcm <- pcm(500*time.Millisecond, 1)
	harness.capture.pcm <- pcm(600*time.Millisecond, 0)
	waitSignal(t, firstStarted, "MiMo first sealed segment")
	if got := harness.capture.stopCall.Load(); got != 0 {
		t.Fatalf("capture Stop calls before batch began = %d, want 0", got)
	}
	close(release)
	harness.overlay.toggles <- struct{}{}
	waitView(t, harness.overlay, func(view View) bool { return view.Final != "" }, "batch final")
	if got := batch.count.Load(); got < 2 {
		t.Fatalf("MiMo calls = %d, want sealed segment plus flushed tail", got)
	}
}

func TestLiveFailureReplaysRetainedAndFutureSegmentsInOrder(t *testing.T) {
	live := newFakeLiveSession()
	recognizer := newFakeRecognizer(live)
	var order []byte
	batch := newFakeTranscriber(func(_ context.Context, pcm []byte, _ int) (string, error) {
		var maximum byte
		for _, value := range pcm {
			if value > maximum {
				maximum = value
			}
		}
		order = append(order, maximum)
		switch maximum {
		case 1:
			return "one", nil
		case 2:
			return "two", nil
		default:
			return "", nil
		}
	})
	harness := startHarness(t, recognizer, batch)
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	live.reads <- fakeLiveRead{event: asr.LiveEvent{Text: "old provisional"}}
	waitView(t, harness.overlay, func(view View) bool { return view.Partial == "old provisional" }, "old partial")
	harness.capture.pcm <- pcm(500*time.Millisecond, 1)
	harness.capture.pcm <- pcm(600*time.Millisecond, 0)
	<-live.sent
	<-live.sent
	live.reads <- fakeLiveRead{err: errors.New("local live read failure")}
	waitView(t, harness.overlay, func(view View) bool {
		return strings.Contains(view.Partial, "using backup")
	}, "fallback status")
	harness.capture.pcm <- pcm(500*time.Millisecond, 2)
	harness.capture.pcm <- pcm(600*time.Millisecond, 0)
	waitPCMCall(t, batch.calls, "retained MiMo segment")
	waitPCMCall(t, batch.calls, "future MiMo segment before stop")
	harness.overlay.toggles <- struct{}{}
	final := waitView(t, harness.overlay, func(view View) bool { return view.Final != "" }, "fallback final")
	if final.Final != "one two" {
		t.Fatalf("Final = %q, want %q", final.Final, "one two")
	}
	if strings.Contains(final.Final, "old provisional") {
		t.Fatalf("old provisional became final: %q", final.Final)
	}
	if !reflect.DeepEqual(order, []byte{1, 2, 0}) {
		t.Fatalf("MiMo segment order = %v, want [1 2 0]", order)
	}
}

func TestMiMoSegmentFailureEndsFailedWithoutIncompleteFinal(t *testing.T) {
	batch := newFakeTranscriber(func(context.Context, []byte, int) (string, error) {
		return "", errors.New("local batch failure")
	})
	harness := startHarness(t, newFakeRecognizer(), batch)
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	harness.capture.pcm <- pcm(500*time.Millisecond, 1)
	harness.capture.pcm <- pcm(600*time.Millisecond, 0)
	errorView := waitView(t, harness.overlay, func(view View) bool { return view.Status == ViewError }, "batch error")
	if errorView.Error != "Transcription failed" {
		t.Fatalf("Error = %q", errorView.Error)
	}
	if len(finalViews(harness.overlay)) != 0 {
		t.Fatal("failed MiMo segment produced an incomplete Final")
	}
	if harness.capture.stopCall.Load() == 0 {
		t.Fatal("capture was not stopped after batch failure")
	}
}

func TestSessionLimitAutomaticallyStopsAndFlushesTail(t *testing.T) {
	var lengths []int
	batch := newFakeTranscriber(func(_ context.Context, pcm []byte, index int) (string, error) {
		lengths = append(lengths, len(pcm))
		return string(rune('A' + index)), nil
	})
	harness := startHarness(t, newFakeRecognizer(), batch)
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	harness.capture.pcm <- pcm(time.Second, 0)
	harness.capture.pcm <- pcm(59*time.Second, 1)
	waitSignal(t, harness.capture.stopped, "automatic 60-second stop")
	waitView(t, harness.overlay, func(view View) bool { return view.Final != "" }, "limit final")
	if got := batch.count.Load(); got != 4 {
		t.Fatalf("MiMo calls = %d, want 4", got)
	}
	wantTail := len(pcm(14*time.Second, 1))
	if got := lengths[len(lengths)-1]; got != wantTail {
		t.Fatalf("flushed tail bytes = %d, want %d", got, wantTail)
	}
}

func TestToggleRestartAndCancellationCleanupHaveNoDuplicateFinal(t *testing.T) {
	first := newFakeLiveSession()
	second := newFakeLiveSession()
	third := newFakeLiveSession()
	recognizer := newFakeRecognizer(first, second, third)
	harness := startHarness(t, recognizer, newFakeTranscriber(nil))

	for index, live := range []*fakeLiveSession{first, second} {
		harness.overlay.toggles <- struct{}{}
		waitSignal(t, harness.capture.started, "capture start")
		harness.overlay.toggles <- struct{}{}
		waitSignal(t, live.finished, "live finish")
		text := string(rune('A' + index))
		live.reads <- fakeLiveRead{event: asr.LiveEvent{Text: text, ProtocolTerminal: true}}
		waitView(t, harness.overlay, func(view View) bool { return view.Final == text }, "session final")
	}
	if got := len(finalViews(harness.overlay)); got != 2 {
		t.Fatalf("Final updates = %d, want 2", got)
	}

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "third capture start")
	harness.close(t)
	if harness.capture.closeCall.Load() != 1 {
		t.Fatalf("capture Close calls = %d, want 1", harness.capture.closeCall.Load())
	}
	select {
	case <-third.closed:
	default:
		t.Fatal("active live session was not closed on cancellation")
	}
	if got := len(finalViews(harness.overlay)); got != 2 {
		t.Fatalf("cancellation added Final update; got %d", got)
	}
}

func TestNewSessionIDUsesRandomNonemptyValues(t *testing.T) {
	first, err := NewSessionID()
	if err != nil {
		t.Fatalf("NewSessionID(first) error = %v", err)
	}
	second, err := NewSessionID()
	if err != nil {
		t.Fatalf("NewSessionID(second) error = %v", err)
	}
	if len(first) != 32 || len(second) != 32 || first == second {
		t.Fatalf("session IDs = %q, %q", first, second)
	}
}
