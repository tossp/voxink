// Package app coordinates capture, recognition, session state, and presentation.
package app

import (
	"context"
	"errors"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/diagnostic"
	"github.com/tossp/voxink/internal/domain"
	"github.com/tossp/voxink/internal/output"
)

// AudioPrivacyNotice is the fixed stage-one active-overlay disclosure.
const AudioPrivacyNotice = "隐私提示：麦克风音频会发送至火山进行实时识别；主线路失败时，同一会话音频可能发送至 MiMo。原始音频默认仅驻留内存且不保存；供应商数据政策请以账户及当期条款为准。"

const (
	// OutputInjectedMessage is the fixed successful injection status.
	OutputInjectedMessage = "已输入"
	// OutputCopiedMessage is the fixed Copy Only status.
	OutputCopiedMessage = "已复制，请手动粘贴"
)

var (
	// ErrMissingCapture reports an incomplete application assembly.
	ErrMissingCapture = errors.New("capture adapter must not be nil")
	// ErrMissingOverlay reports an incomplete application assembly.
	ErrMissingOverlay = errors.New("overlay adapter must not be nil")
	// ErrMissingLiveRecognizer reports a missing stage-one primary dependency.
	ErrMissingLiveRecognizer = errors.New("stage-one primary Volcengine live recognizer must not be nil")
	// ErrMissingBatchTranscriber reports a missing stage-one backup dependency.
	ErrMissingBatchTranscriber = errors.New("stage-one backup MiMo batch transcriber must not be nil")
	// ErrNoCredentials reports that neither required stage-one credential set was supplied.
	ErrNoCredentials = errors.New("stage-one ASR route credentials are not configured")
	// ErrMissingVolcengineCredentials reports a missing primary credential set.
	ErrMissingVolcengineCredentials = errors.New("stage-one primary Volcengine credentials are required; configure VOXINK_VOLC_API_KEY and VOXINK_VOLC_RESOURCE_ID, or legacy VOXINK_VOLC_APP_KEY, VOXINK_VOLC_ACCESS_KEY, and VOXINK_VOLC_RESOURCE_ID")
	// ErrMissingMiMoCredentials reports a missing backup credential set.
	ErrMissingMiMoCredentials = errors.New("stage-one backup MiMo credentials are required; configure VOXINK_MIMO_API_KEY")
)

// Capture is the bounded callback boundary owned by the coordinator.
type Capture interface {
	Start() error
	Stop() error
	Close() error
	PCM() <-chan []byte
	Levels() <-chan float64
	Errors() <-chan error
}

// ViewStatus describes the stage-one overlay state.
type ViewStatus uint8

const (
	// ViewIdle indicates no active capture.
	ViewIdle ViewStatus = iota
	// ViewListening indicates active capture.
	ViewListening
	// ViewTranscribing indicates capture has stopped while recognition drains.
	ViewTranscribing
	// ViewError indicates a terminal session failure.
	ViewError
)

// View is a provider-neutral overlay update.
type View struct {
	Status  ViewStatus
	Level   float64
	Partial string
	Final   string
	Error   string
	Notice  string
}

// Overlay owns hotkey input and stage-one presentation.
type Overlay interface {
	Run() error
	Close() error
	Toggles() <-chan struct{}
	Update(View)
}

// SessionIDGenerator creates a cryptographically random identifier per start.
type SessionIDGenerator func() (domain.SessionID, error)

// SpeechDetector labels a complete PCM callback frame for the segmenter.
type SpeechDetector func([]byte) bool

// Options contains local coordinator policies and test seams.
type Options struct {
	NewSessionID   SessionIDGenerator
	DetectSpeech   SpeechDetector
	DiagnosticSink diagnostic.Sink
	Output         output.Adapter
	WorkerBuffer   int
	LivePCMBuffer  int
}

// Coordinator serializes all session.Controller access on its Run goroutine.
type Coordinator struct {
	capture Capture
	overlay Overlay
	live    asr.LiveRecognizer
	batch   asr.SegmentTranscriber
	output  output.Adapter
	options Options

	controller  sessionController
	active      *activeSession
	workers     chan workerEvent
	view        View
	diagnostics diagnostic.Sink
}

// sessionController is the subset kept private to emphasize single-owner use.
type sessionController interface {
	Start(domain.SessionID) error
	Handle(domain.Event) bool
	CompleteDelivery(domain.SessionID) bool
	State() domain.SessionState
}

// NewCoordinator validates the stage-one application dependencies.
func NewCoordinator(capture Capture, overlay Overlay, live asr.LiveRecognizer, batch asr.SegmentTranscriber, options Options) (*Coordinator, error) {
	if capture == nil {
		return nil, ErrMissingCapture
	}
	if overlay == nil {
		return nil, ErrMissingOverlay
	}
	if live == nil {
		return nil, ErrMissingLiveRecognizer
	}
	if batch == nil {
		return nil, ErrMissingBatchTranscriber
	}
	if options.NewSessionID == nil {
		options.NewSessionID = NewSessionID
	}
	if options.DetectSpeech == nil {
		options.DetectSpeech = detectSpeechPCM16
	}
	if options.WorkerBuffer <= 0 {
		options.WorkerBuffer = 64
	}
	if options.LivePCMBuffer <= 0 {
		options.LivePCMBuffer = 64
	}
	if options.DiagnosticSink == nil {
		options.DiagnosticSink = diagnostic.NoopSink()
	}
	return &Coordinator{
		capture: capture, overlay: overlay, live: live, batch: batch, output: options.Output,
		options: options, workers: make(chan workerEvent, options.WorkerBuffer), diagnostics: options.DiagnosticSink,
	}, nil
}

// Run owns the application message loop until cancellation or overlay exit.
func (c *Coordinator) Run(ctx context.Context) error {
	overlayDone := make(chan error, 1)
	go func() { overlayDone <- c.overlay.Run() }()
	c.publish(View{})

	var runErr error
	overlayExited := false
	loop := true
	for loop {
		select {
		case <-ctx.Done():
			loop = false
		case err := <-overlayDone:
			runErr = err
			overlayExited = true
			loop = false
		case <-c.overlay.Toggles():
			c.handleToggle(ctx)
		case pcm := <-c.capture.PCM():
			c.handlePCM(pcm)
		case level := <-c.capture.Levels():
			c.handleLevel(level)
		case err := <-c.capture.Errors():
			c.handleCaptureError(err)
		case event := <-c.workers:
			c.handleWorker(event)
		}
	}

	shutdownErr := c.shutdown()
	closeErr := c.overlay.Close()
	if !overlayExited {
		runErr = <-overlayDone
	}
	return errors.Join(runErr, shutdownErr, closeErr)
}

func (c *Coordinator) publish(view View) {
	if view.Status == ViewListening || view.Status == ViewTranscribing {
		view.Notice = AudioPrivacyNotice
	} else {
		view.Notice = ""
	}
	c.view = view
	c.overlay.Update(view)
}
