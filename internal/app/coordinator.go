package app

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"time"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/audio"
	"github.com/tossp/voxink/internal/diagnostic"
	"github.com/tossp/voxink/internal/domain"
	"github.com/tossp/voxink/internal/history"
	"github.com/tossp/voxink/internal/output"
	"github.com/tossp/voxink/internal/session"
)

const (
	maximumSessionBytes = audio.SampleRate * audio.BytesPerSample * 60
	batchQueueSize      = 128
	liveCloseTimeout    = time.Second
)

type activeSession struct {
	id        domain.SessionID
	ctx       context.Context
	cancel    context.CancelFunc
	segmenter *audio.ProgressiveSegmenter
	accepted  int
	stopped   bool
	finalSent bool
	output    output.Session

	live          asr.LiveSession
	liveHealthy   bool
	liveCancel    context.CancelFunc
	liveJobs      chan liveJob
	liveDone      chan struct{}
	retained      [][]byte
	liveText      liveAccumulator
	protocolEnded bool

	batchJobs    chan []byte
	batchDone    chan struct{}
	batchPending int
	batchClosed  bool
	batchText    string
	fallback     bool
}

type liveJob struct {
	pcm    []byte
	finish bool
}

type workerKind uint8

const (
	workerLiveEvent workerKind = iota
	workerLiveFailure
	workerBatchResult
	workerBatchFailure
)

type workerEvent struct {
	kind      workerKind
	sessionID domain.SessionID
	live      asr.LiveEvent
	text      string
	err       error
}

// NewSessionID generates a random 128-bit hexadecimal session identifier.
func NewSessionID() (domain.SessionID, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return domain.SessionID(hex.EncodeToString(value[:])), nil
}

func (c *Coordinator) handleToggle(ctx context.Context) {
	if c.active != nil {
		if c.controller.State() == domain.SessionCapturing {
			c.stopActive(diagnostic.CodeUserStop)
		}
		return
	}
	if c.controller != nil {
		state := c.controller.State()
		if state != domain.SessionStopped && state != domain.SessionFailed {
			return
		}
	}
	c.startSession(ctx)
}

func (c *Coordinator) startSession(parent context.Context) {
	id, err := c.options.NewSessionID()
	if err != nil || id == "" {
		c.publish(View{Status: ViewError, Error: "Could not create session"})
		return
	}
	controller := session.NewController()
	if err := controller.Start(id); err != nil {
		c.publish(View{Status: ViewError, Error: "Could not start session"})
		return
	}
	ctx, cancel := context.WithCancel(parent)
	current := &activeSession{
		id: id, ctx: ctx, cancel: cancel, segmenter: audio.NewProgressiveSegmenter(),
	}
	if c.output != nil {
		current.output = c.output.StartSession()
	}
	c.controller = controller
	c.active = current
	c.recordDiagnostic(current, diagnostic.KindSessionStarted, diagnostic.StageSession, asr.VendorVolcengine, "")
	c.publish(View{Status: ViewListening})

	if c.live != nil {
		live, dialErr := c.live.Dial(ctx)
		if dialErr == nil {
			current.live = live
			current.liveHealthy = true
			c.startLiveWorkers(current)
		} else {
			c.switchToBatch(current, diagnostic.CodeLiveDialFailed)
		}
	} else {
		c.switchToBatch(current, diagnostic.CodeLiveDialFailed)
	}
	if c.active != current {
		return
	}
	if err := c.capture.Start(); err != nil {
		c.failCapture("Capture failed", diagnostic.CodeCaptureStartFailed)
	}
}

func (c *Coordinator) handlePCM(pcm []byte) {
	current := c.active
	if current == nil || current.stopped || c.controller.State() != domain.SessionCapturing {
		return
	}
	if len(pcm)%audio.BytesPerSample != 0 {
		c.failCapture("Invalid microphone audio", diagnostic.CodeInvalidPCM)
		return
	}
	remaining := maximumSessionBytes - current.accepted
	if remaining <= 0 {
		return
	}
	if len(pcm) > remaining {
		pcm = pcm[:remaining]
	}
	current.accepted += len(pcm)
	result, err := current.segmenter.Append(pcm, c.options.DetectSpeech(pcm))
	if err != nil {
		c.failCapture("Could not segment microphone audio", diagnostic.CodeSegmenterFailed)
		return
	}
	for _, segment := range result.Segments {
		if !c.acceptSegment(current, segment) {
			return
		}
	}
	if current.liveHealthy && len(pcm) > 0 {
		select {
		case current.liveJobs <- liveJob{pcm: pcm}:
		default:
			c.switchToBatch(current, diagnostic.CodeLiveQueueFull)
		}
	}
	if c.active == current && result.LimitReached {
		c.stopActive(diagnostic.CodeSessionLimit)
	}
}

func (c *Coordinator) acceptSegment(current *activeSession, segment []byte) bool {
	if current.liveHealthy {
		current.retained = append(current.retained, segment)
		return true
	}
	if !current.fallback {
		c.switchToBatch(current, diagnostic.CodeLiveWorkerFailed)
	}
	if c.active != current {
		return false
	}
	return c.enqueueBatch(current, segment)
}

func (c *Coordinator) handleLevel(level float64) {
	current := c.active
	if current == nil || current.stopped || c.controller.State() != domain.SessionCapturing {
		return
	}
	event := domain.Event{SessionID: current.id, Kind: domain.EventLevel, Level: level}
	if c.controller.Handle(event) {
		view := c.view
		view.Status = ViewListening
		view.Level = level
		c.publish(view)
	}
}

func (c *Coordinator) stopActive(reason diagnostic.Code) {
	current := c.active
	if current == nil || current.stopped {
		return
	}
	if err := c.capture.Stop(); err != nil {
		c.failCapture("Capture failed", diagnostic.CodeCaptureStopFailed)
		return
	}
	current.stopped = true
	c.recordDiagnostic(current, diagnostic.KindCaptureStopped, diagnostic.StageCapture, "", reason)
	tail, err := current.segmenter.Finish()
	if err != nil && !errors.Is(err, audio.ErrSegmenterClosed) {
		c.failCapture("Could not finish microphone audio", diagnostic.CodeSegmenterFailed)
		return
	}
	for _, segment := range tail {
		if !c.acceptSegment(current, segment) {
			return
		}
	}
	if !c.controller.Handle(domain.Event{SessionID: current.id, Kind: domain.EventStopped}) {
		c.failActive("Session state failed", diagnostic.StageSession, diagnostic.CodeStateRejected)
		return
	}
	c.publish(View{Status: ViewTranscribing, Partial: c.view.Partial})

	if current.liveHealthy {
		select {
		case current.liveJobs <- liveJob{finish: true}:
		default:
			c.switchToBatch(current, diagnostic.CodeLiveQueueFull)
		}
		if current.protocolEnded && c.active == current {
			c.finishLive(current)
		}
		return
	}
	c.closeBatchInput(current)
	c.finishBatchIfReady(current)
}

func (c *Coordinator) handleWorker(event workerEvent) {
	current := c.active
	if current == nil || event.sessionID != current.id {
		return
	}
	switch event.kind {
	case workerLiveEvent:
		if !current.liveHealthy {
			return
		}
		current.liveText.add(event.live)
		partial := current.liveText.partial(event.live)
		if partial != "" && c.controller.Handle(domain.Event{SessionID: current.id, Kind: domain.EventPartial, Text: partial}) {
			status := ViewListening
			if current.stopped {
				status = ViewTranscribing
			}
			c.publish(View{Status: status, Partial: partial, Level: c.view.Level})
		}
		if event.live.ProtocolTerminal {
			current.protocolEnded = true
			if current.stopped {
				c.finishLive(current)
			}
		}
	case workerLiveFailure:
		if current.liveHealthy && !current.protocolEnded {
			c.switchToBatch(current, diagnostic.CodeLiveWorkerFailed)
		}
	case workerBatchResult:
		if !current.fallback || current.batchPending == 0 {
			return
		}
		current.batchPending--
		current.batchText = asr.JoinText(current.batchText, event.text)
		if current.batchText != "" {
			status := ViewListening
			if current.stopped {
				status = ViewTranscribing
			}
			c.controller.Handle(domain.Event{SessionID: current.id, Kind: domain.EventPartial, Text: current.batchText})
			c.publish(View{Status: status, Partial: current.batchText, Level: c.view.Level})
		}
		c.finishBatchIfReady(current)
	case workerBatchFailure:
		c.failActive("Transcription failed", diagnostic.StageBatch, diagnostic.CodeBatchFailed)
	}
}

func (c *Coordinator) finishLive(current *activeSession) {
	if current.finalSent || !current.protocolEnded {
		return
	}
	c.finalize(current, current.liveText.final())
}

func (c *Coordinator) finalize(current *activeSession, text string) {
	if c.active != current || current.finalSent {
		return
	}
	current.finalSent = true
	if !c.controller.Handle(domain.Event{SessionID: current.id, Kind: domain.EventFinal, Text: text}) {
		c.failActive("Session finalization failed", diagnostic.StageDelivery, diagnostic.CodeStateRejected)
		return
	}
	c.publishRuntimeStatus(StatusDelivering)
	result := output.Result{Mode: output.ModeInjected}
	if current.output != nil {
		result = current.output.Deliver(text)
	}
	var message string
	switch result.Mode {
	case output.ModeInjected:
		message = OutputInjectedMessage
	case output.ModeCopied:
		message = OutputCopiedMessage
	default:
		c.retainHistory(current, text, history.ModeFailed)
		c.failActive("Output failed", diagnostic.StageDelivery, "")
		return
	}
	if !c.controller.CompleteDelivery(current.id) {
		c.failActive("Session finalization failed", diagnostic.StageDelivery, diagnostic.CodeStateRejected)
		return
	}
	c.recordDiagnostic(current, diagnostic.KindSessionCompleted, diagnostic.StageDelivery, "", "")
	mode := history.ModeInjected
	if result.Mode == output.ModeCopied {
		mode = history.ModeCopied
	}
	c.retainHistory(current, text, mode)
	c.publish(View{Status: ViewIdle, Final: message})
	c.cleanupSession(current)
}

func (c *Coordinator) retainHistory(current *activeSession, text string, mode history.Mode) {
	provider := domain.ProviderVolcengineV3
	if current.fallback {
		provider = domain.ProviderMiMoASR
	}
	now := time.Now
	if c.options.Now != nil {
		now = c.options.Now
	}
	entry := history.Entry{Time: now().UTC(), Provider: provider, Mode: mode, Final: text}
	if c.history != nil {
		_ = c.history.Append(entry)
	}
	sendRuntimeEvent(c.events, RuntimeEvent{History: &entry})
}

func (c *Coordinator) failActive(message string, stage diagnostic.Stage, code diagnostic.Code) {
	current := c.active
	if current == nil {
		return
	}
	c.recordDiagnostic(current, diagnostic.KindSessionFailed, stage, "", code)
	c.controller.Handle(domain.Event{SessionID: current.id, Kind: domain.EventError, Message: message})
	_ = c.capture.Stop()
	c.publish(View{Status: ViewError, Error: message})
	c.cleanupSession(current)
}

func (c *Coordinator) shutdown() error {
	if c.active != nil {
		_ = c.capture.Stop()
		c.cleanupSession(c.active)
	}
	return c.capture.Close()
}

func detectSpeechPCM16(pcm []byte) bool {
	const threshold = 512
	for index := 0; index+1 < len(pcm); index += 2 {
		sample := int(int16(uint16(pcm[index]) | uint16(pcm[index+1])<<8))
		if sample < 0 {
			sample = -sample
		}
		if sample >= threshold {
			return true
		}
	}
	return false
}
