package app

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/audio"
	"github.com/tossp/voxink/internal/diagnostic"
	"github.com/tossp/voxink/internal/domain"
	platformwindows "github.com/tossp/voxink/internal/platform/windows"
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
	final := waitView(t, harness.overlay, func(view View) bool { return view.Final == OutputInjectedMessage }, "live final")
	if final.Partial != "" {
		t.Fatalf("final view retained provisional partial %q", final.Partial)
	}
	if got := batch.count.Load(); got != 0 {
		t.Fatalf("MiMo calls = %d, want 0", got)
	}
	if got := len(finalViews(harness.overlay)); got != 1 {
		t.Fatalf("Final updates = %d, want 1", got)
	}
	if _, texts := harness.output.snapshot(); !reflect.DeepEqual(texts, []string{"完整结果"}) {
		t.Fatalf("output texts = %q, want complete Final", texts)
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

func TestActiveOverlayViewsContainFixedPrivacyNotice(t *testing.T) {
	live := newFakeLiveSession()
	harness := startHarness(t, newFakeRecognizer(live), newFakeTranscriber(nil))
	defer harness.close(t)

	idle := waitView(t, harness.overlay, func(view View) bool { return view.Status == ViewIdle }, "idle")
	if idle.Notice != "" {
		t.Fatalf("idle Notice = %q, want empty", idle.Notice)
	}
	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	listening := waitView(t, harness.overlay, func(view View) bool { return view.Status == ViewListening }, "listening")
	if listening.Notice != AudioPrivacyNotice {
		t.Fatalf("listening Notice = %q", listening.Notice)
	}
	harness.overlay.toggles <- struct{}{}
	waitSignal(t, live.finished, "live finish")
	transcribing := waitView(t, harness.overlay, func(view View) bool { return view.Status == ViewTranscribing }, "transcribing")
	if transcribing.Notice != AudioPrivacyNotice {
		t.Fatalf("transcribing Notice = %q", transcribing.Notice)
	}
	live.reads <- fakeLiveRead{event: asr.LiveEvent{ProtocolTerminal: true}}
}

func TestDiagnosticLiveCompleteSequenceAndSessionID(t *testing.T) {
	sink, err := diagnostic.NewMemorySink(16)
	if err != nil {
		t.Fatalf("NewMemorySink() error = %v", err)
	}
	live := newFakeLiveSession()
	harness := startHarnessWithOptions(t, newFakeRecognizer(live), newFakeTranscriber(nil), Options{DiagnosticSink: sink})
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	harness.overlay.toggles <- struct{}{}
	waitSignal(t, live.finished, "live finish")
	live.reads <- fakeLiveRead{event: asr.LiveEvent{Text: "不进入诊断的正文", ProtocolTerminal: true}}
	waitView(t, harness.overlay, func(view View) bool { return view.Final != "" }, "live final")

	assertDiagnosticSequence(t, sink.Snapshot(), "session-1", []diagnostic.Kind{
		diagnostic.KindSessionStarted,
		diagnostic.KindCaptureStopped,
		diagnostic.KindSessionCompleted,
	})
}

func TestDiagnosticFallbackCompleteSequenceAndSessionID(t *testing.T) {
	sink, err := diagnostic.NewMemorySink(16)
	if err != nil {
		t.Fatalf("NewMemorySink() error = %v", err)
	}
	recognizer := newFakeRecognizer()
	recognizer.dialErr = errors.New("dial failed")
	harness := startHarnessWithOptions(t, recognizer, newFakeTranscriber(func(context.Context, []byte, int) (string, error) {
		return "备用结果", nil
	}), Options{DiagnosticSink: sink})
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	harness.capture.pcm <- pcm(500*time.Millisecond, 1)
	harness.capture.pcm <- pcm(600*time.Millisecond, 0)
	waitPCMCall(t, harness.coordinator.batch.(*fakeTranscriber).calls, "fallback segment")
	harness.overlay.toggles <- struct{}{}
	waitView(t, harness.overlay, func(view View) bool { return view.Final != "" }, "fallback final")

	snapshot := sink.Snapshot()
	assertDiagnosticSequence(t, snapshot, "session-1", []diagnostic.Kind{
		diagnostic.KindSessionStarted,
		diagnostic.KindLiveFallback,
		diagnostic.KindCaptureStopped,
		diagnostic.KindSessionCompleted,
	})
	if snapshot.Events[1].AsrVendor() != asr.VendorMiMo || snapshot.Events[1].Code() != diagnostic.CodeLiveDialFailed {
		t.Fatalf("fallback metadata = (%q, %q)", snapshot.Events[1].AsrVendor(), snapshot.Events[1].Code())
	}
}

func TestCoordinatorDiagnosticsDoNotRetainInjectedSecretsAudioOrText(t *testing.T) {
	sink, err := diagnostic.NewMemorySink(16)
	if err != nil {
		t.Fatalf("NewMemorySink() error = %v", err)
	}
	canaries := []string{
		"canary-api-key-123", "Authorization: Bearer canary-token",
		"PCM=00ff01", "UklGRkJBU0U2NA==", "这是识别正文", "https://secret.example/asr?key=canary",
	}
	injected := errors.New(strings.Join(canaries, " | "))
	recognizer := newFakeRecognizer()
	recognizer.dialErr = injected
	harness := startHarnessWithOptions(t, recognizer, newFakeTranscriber(func(context.Context, []byte, int) (string, error) {
		return "", injected
	}), Options{DiagnosticSink: sink})
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	harness.capture.pcm <- pcm(500*time.Millisecond, 1)
	harness.capture.pcm <- pcm(600*time.Millisecond, 0)
	waitView(t, harness.overlay, func(view View) bool { return view.Status == ViewError }, "batch error")

	snapshot := sink.Snapshot()
	assertDiagnosticSequence(t, snapshot, "session-1", []diagnostic.Kind{
		diagnostic.KindSessionStarted,
		diagnostic.KindLiveFallback,
		diagnostic.KindSessionFailed,
	})
	serialized := fmt.Sprintf("%+v", snapshot)
	for _, canary := range canaries {
		if strings.Contains(serialized, canary) {
			t.Fatalf("diagnostic snapshot leaked %q: %s", canary, serialized)
		}
	}
}

func TestCaptureOverflowProducesCategorizedFaultAndFailure(t *testing.T) {
	sink, err := diagnostic.NewMemorySink(16)
	if err != nil {
		t.Fatalf("NewMemorySink() error = %v", err)
	}
	harness := startHarnessWithOptions(t, newFakeRecognizer(newFakeLiveSession()), newFakeTranscriber(nil), Options{DiagnosticSink: sink})
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	harness.capture.errors <- fmt.Errorf("wrapped callback detail: %w", platformwindows.ErrPCMOverflow)
	waitView(t, harness.overlay, func(view View) bool { return view.Status == ViewError }, "capture overflow")

	snapshot := sink.Snapshot()
	assertDiagnosticSequence(t, snapshot, "session-1", []diagnostic.Kind{
		diagnostic.KindSessionStarted,
		diagnostic.KindCaptureFault,
		diagnostic.KindSessionFailed,
	})
	for _, event := range snapshot.Events[1:] {
		if event.Code() != diagnostic.CodeCaptureOverflow {
			t.Fatalf("capture event code = %q", event.Code())
		}
	}
}

func TestCaptureInternalErrorUsesFixedCodeWithoutRawMessage(t *testing.T) {
	sink, err := diagnostic.NewMemorySink(8)
	if err != nil {
		t.Fatalf("NewMemorySink() error = %v", err)
	}
	harness := startHarnessWithOptions(t, newFakeRecognizer(newFakeLiveSession()), newFakeTranscriber(nil), Options{DiagnosticSink: sink})
	defer harness.close(t)

	harness.overlay.toggles <- struct{}{}
	waitSignal(t, harness.capture.started, "capture start")
	harness.capture.errors <- errors.New("internal canary Authorization PCM UklGRg== 识别正文")
	errorView := waitView(t, harness.overlay, func(view View) bool { return view.Status == ViewError }, "capture internal error")
	if errorView.Notice != "" {
		t.Fatalf("error Notice = %q, want empty", errorView.Notice)
	}
	snapshot := sink.Snapshot()
	assertDiagnosticSequence(t, snapshot, "session-1", []diagnostic.Kind{
		diagnostic.KindSessionStarted,
		diagnostic.KindCaptureFault,
		diagnostic.KindSessionFailed,
	})
	for _, event := range snapshot.Events[1:] {
		if event.Code() != diagnostic.CodeCaptureInternal {
			t.Fatalf("capture event code = %q", event.Code())
		}
	}
	if strings.Contains(fmt.Sprintf("%+v", snapshot), "internal canary") {
		t.Fatalf("diagnostic snapshot retained raw capture error: %+v", snapshot)
	}
}

func TestCleanupSessionReleasesAudioReferences(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	current := &activeSession{
		ctx: ctx, cancel: cancel, segmenter: audio.NewProgressiveSegmenter(), accepted: 42,
		retained: [][]byte{{1, 2, 3}}, liveJobs: make(chan liveJob, 1), batchJobs: make(chan []byte, 1),
	}
	current.liveJobs <- liveJob{pcm: []byte{4, 5, 6}}
	current.batchJobs <- []byte{7, 8, 9}
	coordinator := &Coordinator{active: current}

	coordinator.cleanupSession(current)

	if coordinator.active != nil || current.segmenter != nil || current.retained != nil || current.liveJobs != nil || current.batchJobs != nil {
		t.Fatalf("audio references remain after cleanup: active=%v segmenter=%v retained=%v liveJobs=%v batchJobs=%v",
			coordinator.active, current.segmenter, current.retained, current.liveJobs, current.batchJobs)
	}
	if current.accepted != 0 || current.batchPending != 0 {
		t.Fatalf("audio counters remain after cleanup: accepted=%d pending=%d", current.accepted, current.batchPending)
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
	waitView(t, harness.overlay, func(view View) bool { return view.Final != "" }, "fallback final")
	_, texts := harness.output.snapshot()
	if !reflect.DeepEqual(texts, []string{"one two"}) {
		t.Fatalf("output texts = %q, want %q", texts, "one two")
	}
	if strings.Contains(strings.Join(texts, ""), "old provisional") {
		t.Fatalf("old provisional became final output: %q", texts)
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
	if errorView.Notice != "" {
		t.Fatalf("error Notice = %q, want empty", errorView.Notice)
	}
	if len(finalViews(harness.overlay)) != 0 {
		t.Fatal("failed MiMo segment produced an incomplete Final")
	}
	if harness.capture.stopCall.Load() == 0 {
		t.Fatal("capture was not stopped after batch failure")
	}
}

func TestSessionLimitAutomaticallyStopsAndFlushesTail(t *testing.T) {
	sink, err := diagnostic.NewMemorySink(16)
	if err != nil {
		t.Fatalf("NewMemorySink() error = %v", err)
	}
	var lengths []int
	batch := newFakeTranscriber(func(_ context.Context, pcm []byte, index int) (string, error) {
		lengths = append(lengths, len(pcm))
		return string(rune('A' + index)), nil
	})
	harness := startHarnessWithOptions(t, newFakeRecognizer(), batch, Options{DiagnosticSink: sink})
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
	foundLimit := false
	for _, event := range sink.Snapshot().Events {
		if event.Kind() == diagnostic.KindCaptureStopped {
			foundLimit = true
			if event.Code() != diagnostic.CodeSessionLimit {
				t.Fatalf("limit stop code = %q", event.Code())
			}
		}
	}
	if !foundLimit {
		t.Fatal("session limit did not produce capture_stopped diagnostic")
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
		waitOutputCount(t, harness.output, index+1)
		waitFinalViewCount(t, harness.overlay, index+1)
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

func assertDiagnosticSequence(t *testing.T, snapshot diagnostic.Snapshot, sessionID domain.SessionID, kinds []diagnostic.Kind) {
	t.Helper()
	if len(snapshot.Events) != len(kinds) {
		t.Fatalf("diagnostic event count = %d, want %d; snapshot=%+v", len(snapshot.Events), len(kinds), snapshot)
	}
	for index, kind := range kinds {
		event := snapshot.Events[index]
		if event.Kind() != kind || event.SessionID() != sessionID {
			t.Fatalf("event[%d] = (kind=%v session=%q), want (kind=%v session=%q)", index, event.Kind(), event.SessionID(), kind, sessionID)
		}
	}
}
