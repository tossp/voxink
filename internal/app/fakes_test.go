package app

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/audio"
	"github.com/tossp/voxink/internal/domain"
	"github.com/tossp/voxink/internal/output"
)

type fakeCapture struct {
	pcm       chan []byte
	levels    chan float64
	errors    chan error
	started   chan struct{}
	stopped   chan struct{}
	startCall atomic.Int32
	stopCall  atomic.Int32
	closeCall atomic.Int32
}

func newFakeCapture() *fakeCapture {
	return &fakeCapture{
		pcm: make(chan []byte, 32), levels: make(chan float64, 8), errors: make(chan error, 8),
		started: make(chan struct{}, 8), stopped: make(chan struct{}, 8),
	}
}

func (c *fakeCapture) Start() error {
	c.startCall.Add(1)
	c.started <- struct{}{}
	return nil
}

func (c *fakeCapture) Stop() error {
	c.stopCall.Add(1)
	select {
	case c.stopped <- struct{}{}:
	default:
	}
	return nil
}

func (c *fakeCapture) Close() error {
	c.closeCall.Add(1)
	return nil
}

func (c *fakeCapture) PCM() <-chan []byte     { return c.pcm }
func (c *fakeCapture) Levels() <-chan float64 { return c.levels }
func (c *fakeCapture) Errors() <-chan error   { return c.errors }

type fakeOverlay struct {
	toggles   chan struct{}
	exits     chan struct{}
	updates   chan struct{}
	closed    chan struct{}
	closeOnce sync.Once
	mu        sync.Mutex
	views     []View
}

func newFakeOverlay() *fakeOverlay {
	return &fakeOverlay{
		toggles: make(chan struct{}, 8), exits: make(chan struct{}, 1), updates: make(chan struct{}, 64), closed: make(chan struct{}),
	}
}

func (o *fakeOverlay) Run() error {
	<-o.closed
	return nil
}

func (o *fakeOverlay) Close() error {
	o.closeOnce.Do(func() { close(o.closed) })
	return nil
}

func (o *fakeOverlay) Toggles() <-chan struct{} { return o.toggles }
func (o *fakeOverlay) Exits() <-chan struct{}   { return o.exits }

func (o *fakeOverlay) Update(view View) {
	o.mu.Lock()
	o.views = append(o.views, view)
	o.mu.Unlock()
	select {
	case o.updates <- struct{}{}:
	default:
	}
}

func (o *fakeOverlay) snapshot() []View {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]View(nil), o.views...)
}

type fakeRecognizer struct {
	dialErr  error
	sessions chan *fakeLiveSession
	dials    atomic.Int32
}

func newFakeRecognizer(sessions ...*fakeLiveSession) *fakeRecognizer {
	queue := make(chan *fakeLiveSession, len(sessions))
	for _, live := range sessions {
		queue <- live
	}
	return &fakeRecognizer{sessions: queue}
}

func (r *fakeRecognizer) Dial(context.Context) (asr.LiveSession, error) {
	r.dials.Add(1)
	if r.dialErr != nil {
		return nil, r.dialErr
	}
	select {
	case live := <-r.sessions:
		return live, nil
	default:
		return nil, errors.New("no fake live session")
	}
}

type fakeLiveRead struct {
	event asr.LiveEvent
	err   error
}

type fakeLiveSession struct {
	reads     chan fakeLiveRead
	sent      chan []byte
	finished  chan struct{}
	closed    chan struct{}
	finishOne sync.Once
	closeOne  sync.Once
}

func newFakeLiveSession() *fakeLiveSession {
	return &fakeLiveSession{
		reads: make(chan fakeLiveRead, 16), sent: make(chan []byte, 32),
		finished: make(chan struct{}), closed: make(chan struct{}),
	}
}

func (s *fakeLiveSession) SendPCM(ctx context.Context, pcm []byte) error {
	select {
	case s.sent <- append([]byte(nil), pcm...):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-s.closed:
		return errors.New("fake live session closed")
	}
}

func (s *fakeLiveSession) FinishInput(context.Context) error {
	s.finishOne.Do(func() { close(s.finished) })
	return nil
}

func (s *fakeLiveSession) NextEvent(ctx context.Context) (asr.LiveEvent, error) {
	select {
	case result := <-s.reads:
		return result.event, result.err
	case <-ctx.Done():
		return asr.LiveEvent{}, ctx.Err()
	case <-s.closed:
		return asr.LiveEvent{}, errors.New("fake live session closed")
	}
}

func (s *fakeLiveSession) Close(context.Context) error {
	s.closeOne.Do(func() { close(s.closed) })
	return nil
}

type fakeTranscriber struct {
	fn    func(context.Context, []byte, int) (string, error)
	calls chan []byte
	count atomic.Int32
}

func newFakeTranscriber(fn func(context.Context, []byte, int) (string, error)) *fakeTranscriber {
	return &fakeTranscriber{fn: fn, calls: make(chan []byte, 128)}
}

func (t *fakeTranscriber) Transcribe(ctx context.Context, pcm []byte) (string, error) {
	index := int(t.count.Add(1)) - 1
	copyPCM := append([]byte(nil), pcm...)
	t.calls <- copyPCM
	if t.fn == nil {
		return "", nil
	}
	return t.fn(ctx, copyPCM, index)
}

type coordinatorHarness struct {
	capture     *fakeCapture
	overlay     *fakeOverlay
	output      *fakeOutputAdapter
	coordinator *Coordinator
	cancel      context.CancelFunc
	done        chan error
}

func startHarness(t *testing.T, live asr.LiveRecognizer, batch asr.SegmentTranscriber) *coordinatorHarness {
	return startHarnessWithOptions(t, live, batch, Options{})
}

func startHarnessWithOptions(t *testing.T, live asr.LiveRecognizer, batch asr.SegmentTranscriber, options Options) *coordinatorHarness {
	t.Helper()
	capture := newFakeCapture()
	overlay := newFakeOverlay()
	var ids atomic.Int32
	if options.NewSessionID == nil {
		options.NewSessionID = func() (domain.SessionID, error) {
			return domain.SessionID("session-" + string(rune('0'+ids.Add(1)))), nil
		}
	}
	if options.DetectSpeech == nil {
		options.DetectSpeech = func(pcm []byte) bool {
			for _, value := range pcm {
				if value != 0 {
					return true
				}
			}
			return false
		}
	}
	var outputAdapter *fakeOutputAdapter
	if options.Output == nil {
		outputAdapter = newFakeOutputAdapter(output.Result{Mode: output.ModeInjected})
		options.Output = outputAdapter
	} else {
		outputAdapter, _ = options.Output.(*fakeOutputAdapter)
	}
	coordinator, err := NewCoordinator(capture, overlay, live, batch, options)
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- coordinator.Run(ctx) }()
	return &coordinatorHarness{capture: capture, overlay: overlay, output: outputAdapter, coordinator: coordinator, cancel: cancel, done: done}
}

type fakeOutputAdapter struct {
	mu       sync.Mutex
	result   output.Result
	starts   int
	sessions []*fakeOutputSession
}

func newFakeOutputAdapter(result output.Result) *fakeOutputAdapter {
	return &fakeOutputAdapter{result: result}
}

func (a *fakeOutputAdapter) StartSession() output.Session {
	a.mu.Lock()
	defer a.mu.Unlock()
	session := &fakeOutputSession{result: a.result}
	a.starts++
	a.sessions = append(a.sessions, session)
	return session
}

func (a *fakeOutputAdapter) snapshot() (int, []string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	var texts []string
	for _, session := range a.sessions {
		texts = append(texts, session.snapshot()...)
	}
	return a.starts, texts
}

type fakeOutputSession struct {
	mu     sync.Mutex
	result output.Result
	texts  []string
}

func (s *fakeOutputSession) Deliver(text string) output.Result {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.texts = append(s.texts, text)
	return s.result
}

func (s *fakeOutputSession) snapshot() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.texts...)
}

func (h *coordinatorHarness) close(t *testing.T) {
	t.Helper()
	h.cancel()
	select {
	case err := <-h.done:
		if err != nil {
			t.Fatalf("Coordinator.Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Coordinator.Run() did not exit")
	}
}

func pcm(duration time.Duration, marker byte) []byte {
	samples := int64(audio.SampleRate) * int64(duration) / int64(time.Second)
	return makeFilled(int(samples)*audio.BytesPerSample, marker)
}

func makeFilled(length int, marker byte) []byte {
	value := make([]byte, length)
	for index := range value {
		value[index] = marker
	}
	return value
}

func waitSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
	}
}

func waitPCMCall(t *testing.T, calls <-chan []byte, name string) []byte {
	t.Helper()
	select {
	case pcm := <-calls:
		return pcm
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for %s", name)
		return nil
	}
}

func waitView(t *testing.T, overlay *fakeOverlay, predicate func(View) bool, name string) View {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		views := overlay.snapshot()
		for _, view := range views {
			if predicate(view) {
				return view
			}
		}
		select {
		case <-overlay.updates:
		case <-deadline.C:
			t.Fatalf("timed out waiting for view %s; views=%+v", name, views)
		}
	}
}

func finalViews(overlay *fakeOverlay) []View {
	var finals []View
	for _, view := range overlay.snapshot() {
		if view.Final != "" {
			finals = append(finals, view)
		}
	}
	return finals
}

func waitOutputCount(t *testing.T, adapter *fakeOutputAdapter, count int) []string {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		_, texts := adapter.snapshot()
		if len(texts) >= count {
			return texts
		}
		select {
		case <-time.After(time.Millisecond):
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d output calls; texts=%q", count, texts)
		}
	}
}

func waitFinalViewCount(t *testing.T, overlay *fakeOverlay, count int) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		if len(finalViews(overlay)) >= count {
			return
		}
		select {
		case <-overlay.updates:
		case <-deadline.C:
			t.Fatalf("timed out waiting for %d Final views; views=%+v", count, overlay.snapshot())
		}
	}
}
