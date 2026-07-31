// Package diagnostic provides bounded, structured, in-memory diagnostics.
package diagnostic

import (
	"errors"
	"sync"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/domain"
)

const (
	// MaxSessionIDLength bounds the identifier retained by diagnostics.
	MaxSessionIDLength = 64
	// MaxCodeLength bounds fixed diagnostic reason codes.
	MaxCodeLength = 32
)

var (
	// ErrInvalidCapacity reports a non-positive MemorySink capacity.
	ErrInvalidCapacity = errors.New("diagnostic capacity must be positive")
	// ErrInvalidEvent reports metadata outside the diagnostic allowlist.
	ErrInvalidEvent = errors.New("invalid diagnostic event")
)

// Kind identifies one fixed diagnostic event category.
type Kind uint8

const (
	// KindSessionStarted records creation of one accepted session.
	KindSessionStarted Kind = iota + 1
	// KindCaptureStopped records user or hard-limit capture stop.
	KindCaptureStopped
	// KindLiveFallback records switching from live recognition to batch backup.
	KindLiveFallback
	// KindSessionCompleted records successful final delivery to stage-one presentation.
	KindSessionCompleted
	// KindSessionFailed records a terminal categorized session failure.
	KindSessionFailed
	// KindCaptureFault records a categorized capture-path failure.
	KindCaptureFault
)

// Stage identifies the fixed subsystem stage associated with an event.
type Stage uint8

const (
	// StageSession identifies session creation or state handling.
	StageSession Stage = iota + 1
	// StageCapture identifies microphone capture and segmentation.
	StageCapture
	// StageLive identifies the primary live recognition path.
	StageLive
	// StageBatch identifies the backup completed-segment path.
	StageBatch
	// StageDelivery identifies final stage-one overlay delivery.
	StageDelivery
)

// Code is a bounded safe token, never an error message or provider payload.
type Code string

const (
	// CodeUserStop identifies an explicit user capture stop.
	CodeUserStop Code = "user_stop"
	// CodeSessionLimit identifies the 60-second capture limit.
	CodeSessionLimit Code = "session_limit"
	// CodeLiveDialFailed identifies primary connection setup failure.
	CodeLiveDialFailed Code = "live_dial_failed"
	// CodeLiveWorkerFailed identifies primary reader or writer failure.
	CodeLiveWorkerFailed Code = "live_worker_failed"
	// CodeLiveQueueFull identifies bounded primary input saturation.
	CodeLiveQueueFull Code = "live_queue_full"
	// CodeCaptureOverflow identifies bounded capture ingress saturation.
	CodeCaptureOverflow Code = "capture_overflow"
	// CodeCaptureInternal identifies other asynchronous capture failures.
	CodeCaptureInternal Code = "capture_internal"
	// CodeCaptureStartFailed identifies microphone start failure.
	CodeCaptureStartFailed Code = "capture_start_failed"
	// CodeCaptureStopFailed identifies microphone stop failure.
	CodeCaptureStopFailed Code = "capture_stop_failed"
	// CodeInvalidPCM identifies malformed PCM framing.
	CodeInvalidPCM Code = "invalid_pcm"
	// CodeSegmenterFailed identifies local segmentation failure.
	CodeSegmenterFailed Code = "segmenter_failed"
	// CodeBatchFailed identifies backup transcription failure.
	CodeBatchFailed Code = "batch_failed"
	// CodeBatchQueueFull identifies bounded backup queue saturation.
	CodeBatchQueueFull Code = "batch_queue_full"
	// CodeBatchQueueClosed identifies enqueue after backup input closure.
	CodeBatchQueueClosed Code = "batch_queue_closed"
	// CodeStateRejected identifies an invalid session-state transition.
	CodeStateRejected Code = "state_rejected"
)

// EventInput contains the complete allowlisted input to NewEvent.
// It intentionally has no free-text, error, endpoint, header, or byte fields.
type EventInput struct {
	SessionID  domain.SessionID
	Kind       Kind
	Stage      Stage
	AsrVendor  asr.AsrVendor
	Code       Code
	Count      uint64
	DurationMS uint64
}

// Event is an immutable structured diagnostic record.
type Event struct {
	sessionID  domain.SessionID
	kind       Kind
	stage      Stage
	asrVendor  asr.AsrVendor
	code       Code
	count      uint64
	durationMS uint64
}

// NewEvent validates all string-like metadata against strict token allowlists.
func NewEvent(input EventInput) (Event, error) {
	if !validToken(string(input.SessionID), MaxSessionIDLength) ||
		input.Kind < KindSessionStarted || input.Kind > KindCaptureFault ||
		input.Stage < StageSession || input.Stage > StageDelivery ||
		!validVendor(input.AsrVendor) ||
		!validCode(input.Code) {
		return Event{}, ErrInvalidEvent
	}
	return Event{
		sessionID: input.SessionID, kind: input.Kind, stage: input.Stage,
		asrVendor: input.AsrVendor, code: input.Code,
		count: input.Count, durationMS: input.DurationMS,
	}, nil
}

// SessionID returns the event session identifier.
func (e Event) SessionID() domain.SessionID { return e.sessionID }

// Kind returns the fixed event category.
func (e Event) Kind() Kind { return e.kind }

// Stage returns the fixed event stage.
func (e Event) Stage() Stage { return e.stage }

// AsrVendor returns the optional fixed supplier identifier.
func (e Event) AsrVendor() asr.AsrVendor { return e.asrVendor }

// Code returns the optional bounded safe reason code.
func (e Event) Code() Code { return e.code }

// Count returns the event-specific non-content count.
func (e Event) Count() uint64 { return e.count }

// DurationMS returns the event-specific duration in milliseconds.
func (e Event) DurationMS() uint64 { return e.durationMS }

// Sink accepts complete diagnostic events.
type Sink interface {
	Record(Event)
}

type noopSink struct{}

func (noopSink) Record(Event) {}

// NoopSink returns a sink that discards every event.
func NoopSink() Sink { return noopSink{} }

// Snapshot is an atomic copy of a MemorySink's chronological events and count.
type Snapshot struct {
	// Events contains a chronological copy from oldest to newest.
	Events []Event
	// Overwritten counts oldest events replaced since sink creation.
	Overwritten uint64
}

// MemorySink is a concurrency-safe fixed-capacity overwrite-oldest ring.
type MemorySink struct {
	mu          sync.RWMutex
	events      []Event
	start       int
	size        int
	overwritten uint64
}

// NewMemorySink creates a bounded in-memory diagnostic ring.
func NewMemorySink(capacity int) (*MemorySink, error) {
	if capacity <= 0 {
		return nil, ErrInvalidCapacity
	}
	return &MemorySink{events: make([]Event, capacity)}, nil
}

// Record appends an event, overwriting the oldest event when full.
func (s *MemorySink) Record(event Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.size < len(s.events) {
		index := (s.start + s.size) % len(s.events)
		s.events[index] = event
		s.size++
		return
	}
	s.events[s.start] = event
	s.start = (s.start + 1) % len(s.events)
	s.overwritten++
}

// Snapshot returns events from oldest to newest with the overwrite count.
func (s *MemorySink) Snapshot() Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	events := make([]Event, s.size)
	for index := range events {
		events[index] = s.events[(s.start+index)%len(s.events)]
	}
	return Snapshot{Events: events, Overwritten: s.overwritten}
}

func validVendor(vendor asr.AsrVendor) bool {
	switch vendor {
	case "", asr.VendorVolcengine, asr.VendorMiMo, asr.VendorMOSI:
		return true
	default:
		return false
	}
}

func validCode(code Code) bool {
	if code != "" && !validToken(string(code), MaxCodeLength) {
		return false
	}
	switch code {
	case "", CodeUserStop, CodeSessionLimit, CodeLiveDialFailed, CodeLiveWorkerFailed,
		CodeLiveQueueFull, CodeCaptureOverflow, CodeCaptureInternal, CodeCaptureStartFailed,
		CodeCaptureStopFailed, CodeInvalidPCM, CodeSegmenterFailed, CodeBatchFailed,
		CodeBatchQueueFull, CodeBatchQueueClosed, CodeStateRejected:
		return true
	default:
		return false
	}
}

func validToken(value string, maximum int) bool {
	if value == "" || len(value) > maximum {
		return false
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			continue
		}
		return false
	}
	return true
}
