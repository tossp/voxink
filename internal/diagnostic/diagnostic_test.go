package diagnostic

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/domain"
)

func TestMemorySinkCapacityOrderAndOverwriteCount(t *testing.T) {
	sink, err := NewMemorySink(3)
	if err != nil {
		t.Fatalf("NewMemorySink() error = %v", err)
	}
	for index := 1; index <= 5; index++ {
		sink.Record(mustEvent(t, EventInput{
			SessionID: domain.SessionID("session-" + string(rune('0'+index))),
			Kind:      KindSessionStarted, Stage: StageSession, Count: uint64(index),
		}))
	}
	snapshot := sink.Snapshot()
	if snapshot.Overwritten != 2 {
		t.Fatalf("Overwritten = %d, want 2", snapshot.Overwritten)
	}
	if len(snapshot.Events) != 3 {
		t.Fatalf("events = %d, want 3", len(snapshot.Events))
	}
	for index, want := range []uint64{3, 4, 5} {
		if got := snapshot.Events[index].Count(); got != want {
			t.Fatalf("event[%d].Count = %d, want %d", index, got, want)
		}
	}
}

func TestMemorySinkConcurrentRecordAndSnapshot(t *testing.T) {
	sink, err := NewMemorySink(32)
	if err != nil {
		t.Fatalf("NewMemorySink() error = %v", err)
	}
	event := mustEvent(t, EventInput{SessionID: "session-race", Kind: KindSessionStarted, Stage: StageSession})
	var workers sync.WaitGroup
	for range 8 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 1_000 {
				sink.Record(event)
				_ = sink.Snapshot()
			}
		}()
	}
	workers.Wait()
	snapshot := sink.Snapshot()
	if len(snapshot.Events) != 32 || snapshot.Overwritten != 8_000-32 {
		t.Fatalf("snapshot = (%d events, %d overwritten)", len(snapshot.Events), snapshot.Overwritten)
	}
}

func TestEventRejectsUnsafeStringMetadata(t *testing.T) {
	base := EventInput{SessionID: "session-safe", Kind: KindSessionFailed, Stage: StageLive}
	tests := []EventInput{
		{SessionID: "Authorization Bearer secret", Kind: base.Kind, Stage: base.Stage},
		{SessionID: base.SessionID, Kind: base.Kind, Stage: base.Stage, Code: Code(strings.Repeat("a", MaxCodeLength+1))},
		{SessionID: base.SessionID, Kind: base.Kind, Stage: base.Stage, Code: "canary-api-key-123"},
		{SessionID: base.SessionID, Kind: base.Kind, Stage: base.Stage, Code: "pcm:AAECAw=="},
		{SessionID: base.SessionID, Kind: base.Kind, Stage: base.Stage, AsrVendor: asr.AsrVendor("https://secret.example")},
	}
	for _, input := range tests {
		if _, err := NewEvent(input); !errors.Is(err, ErrInvalidEvent) {
			t.Fatalf("NewEvent(%+v) error = %v, want ErrInvalidEvent", input, err)
		}
	}
	if _, err := NewEvent(EventInput{SessionID: base.SessionID, Kind: base.Kind, Stage: base.Stage, Code: CodeBatchQueueClosed}); err != nil {
		t.Fatalf("NewEvent(fixed safe code) error = %v", err)
	}
}

func TestFixedCodesMeetLengthAndCharacterBoundary(t *testing.T) {
	codes := []Code{
		CodeUserStop, CodeSessionLimit, CodeLiveDialFailed, CodeLiveWorkerFailed,
		CodeLiveQueueFull, CodeCaptureOverflow, CodeCaptureInternal, CodeCaptureStartFailed,
		CodeCaptureStopFailed, CodeInvalidPCM, CodeSegmenterFailed, CodeBatchFailed,
		CodeBatchQueueFull, CodeBatchQueueClosed, CodeStateRejected,
	}
	for _, code := range codes {
		if !validToken(string(code), MaxCodeLength) {
			t.Fatalf("fixed code %q violates token boundary", code)
		}
	}
}

func TestEventHasNoExportedPayloadFields(t *testing.T) {
	typeOfEvent := reflect.TypeOf(Event{})
	for index := range typeOfEvent.NumField() {
		field := typeOfEvent.Field(index)
		if field.IsExported() {
			t.Fatalf("Event field %q is exported and can bypass construction", field.Name)
		}
		if field.Type.Kind() == reflect.Slice || field.Type.Kind() == reflect.Map || field.Type.Kind() == reflect.Interface {
			t.Fatalf("Event field %q can retain arbitrary payloads", field.Name)
		}
	}
}

func TestNewMemorySinkRejectsUnboundedCapacity(t *testing.T) {
	if _, err := NewMemorySink(0); !errors.Is(err, ErrInvalidCapacity) {
		t.Fatalf("NewMemorySink(0) error = %v", err)
	}
}

func mustEvent(t *testing.T, input EventInput) Event {
	t.Helper()
	event, err := NewEvent(input)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}
	return event
}
