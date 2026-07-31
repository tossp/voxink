// Package app coordinates capture, recognition, session state, and presentation.
package app

import (
	"context"
	"errors"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/domain"
)

var (
	// ErrMissingCapture reports an incomplete application assembly.
	ErrMissingCapture = errors.New("capture adapter must not be nil")
	// ErrMissingOverlay reports an incomplete application assembly.
	ErrMissingOverlay = errors.New("overlay adapter must not be nil")
	// ErrMissingProviders reports that neither stage-one recognition path exists.
	ErrMissingProviders = errors.New("at least one ASR provider must be configured")
	// ErrNoCredentials reports that no runtime provider credentials were supplied.
	ErrNoCredentials = errors.New("no ASR credentials configured")
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
	NewSessionID  SessionIDGenerator
	DetectSpeech  SpeechDetector
	WorkerBuffer  int
	LivePCMBuffer int
}

// Coordinator serializes all session.Controller access on its Run goroutine.
type Coordinator struct {
	capture Capture
	overlay Overlay
	live    asr.LiveRecognizer
	batch   asr.SegmentTranscriber
	options Options

	controller sessionController
	active     *activeSession
	workers    chan workerEvent
	view       View
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
	if live == nil && batch == nil {
		return nil, ErrMissingProviders
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
	return &Coordinator{
		capture: capture, overlay: overlay, live: live, batch: batch,
		options: options, workers: make(chan workerEvent, options.WorkerBuffer),
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
		case <-c.capture.Errors():
			c.failActive("Capture failed")
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
	c.view = view
	c.overlay.Update(view)
}
