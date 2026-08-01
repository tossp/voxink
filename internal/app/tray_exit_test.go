package app

import (
	"context"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestOverlayExitStopsCaptureBeforeClosingRuntime(t *testing.T) {
	var eventMu sync.Mutex
	var events []string
	record := func(event string) {
		eventMu.Lock()
		events = append(events, event)
		eventMu.Unlock()
	}
	capture := &orderedCapture{fakeCapture: newFakeCapture(), record: record}
	overlay := &orderedOverlay{fakeOverlay: newFakeOverlay(), record: record}
	live := newFakeLiveSession()
	coordinator, err := NewCoordinator(capture, overlay, newFakeRecognizer(live), newFakeTranscriber(nil), Options{})
	if err != nil {
		t.Fatalf("NewCoordinator() error = %v", err)
	}
	done := make(chan error, 1)
	go func() { done <- coordinator.Run(context.Background()) }()

	overlay.toggles <- struct{}{}
	waitSignal(t, capture.started, "capture start")
	overlay.exits <- struct{}{}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Coordinator.Run() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Coordinator.Run() did not exit after tray request")
	}

	eventMu.Lock()
	got := append([]string(nil), events...)
	eventMu.Unlock()
	want := []string{"capture-stop", "capture-close", "overlay-close"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cleanup order = %v, want %v", got, want)
	}
	if err := overlay.Close(); err != nil {
		t.Fatalf("second overlay Close() error = %v", err)
	}
	if capture.stopCall.Load() != 1 || capture.closeCall.Load() != 1 {
		t.Fatalf("capture stop/close calls = (%d, %d), want (1, 1)", capture.stopCall.Load(), capture.closeCall.Load())
	}
}

type orderedCapture struct {
	*fakeCapture
	record func(string)
}

func (c *orderedCapture) Stop() error {
	c.record("capture-stop")
	return c.fakeCapture.Stop()
}

func (c *orderedCapture) Close() error {
	c.record("capture-close")
	return c.fakeCapture.Close()
}

type orderedOverlay struct {
	*fakeOverlay
	record    func(string)
	closeOnce sync.Once
}

func (o *orderedOverlay) Close() error {
	o.closeOnce.Do(func() {
		o.record("overlay-close")
		_ = o.fakeOverlay.Close()
	})
	return nil
}

var _ Capture = (*orderedCapture)(nil)
var _ Overlay = (*orderedOverlay)(nil)
