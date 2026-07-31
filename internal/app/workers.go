package app

import (
	"context"
	"strings"

	"github.com/tossp/voxink/internal/asr"
)

type liveAccumulator struct {
	lastText string
	stable   []asr.LiveUtterance
}

func (a *liveAccumulator) add(event asr.LiveEvent) {
	if text := strings.TrimSpace(event.Text); text != "" {
		a.lastText = text
	}
	for _, utterance := range event.Utterances {
		if !utterance.Stable || strings.TrimSpace(utterance.Text) == "" {
			continue
		}
		replaced := false
		for index := range a.stable {
			if a.stable[index].StartMS == utterance.StartMS && a.stable[index].EndMS == utterance.EndMS {
				a.stable[index] = utterance
				replaced = true
				break
			}
		}
		if !replaced {
			a.stable = append(a.stable, utterance)
		}
	}
}

func (a *liveAccumulator) partial(event asr.LiveEvent) string {
	if text := strings.TrimSpace(event.Text); text != "" {
		return text
	}
	return a.stableText()
}

func (a *liveAccumulator) final() string {
	if a.lastText != "" {
		return a.lastText
	}
	return a.stableText()
}

func (a *liveAccumulator) stableText() string {
	var text string
	for _, utterance := range a.stable {
		text = asr.JoinText(text, utterance.Text)
	}
	return text
}

func (c *Coordinator) startLiveWorkers(current *activeSession) {
	liveCtx, cancel := context.WithCancel(current.ctx)
	current.liveCancel = cancel
	current.liveJobs = make(chan liveJob, c.options.LivePCMBuffer)
	current.liveDone = make(chan struct{}, 2)
	go c.runLiveWriter(liveCtx, current)
	go c.runLiveReader(liveCtx, current)
}

func (c *Coordinator) runLiveWriter(ctx context.Context, current *activeSession) {
	defer func() { current.liveDone <- struct{}{} }()
	for {
		select {
		case <-ctx.Done():
			return
		case job := <-current.liveJobs:
			var err error
			if job.finish {
				err = current.live.FinishInput(ctx)
			} else {
				err = current.live.SendPCM(ctx, job.pcm)
			}
			if err != nil {
				c.sendWorker(ctx, workerEvent{kind: workerLiveFailure, sessionID: current.id, err: err})
				return
			}
			if job.finish {
				return
			}
		}
	}
}

func (c *Coordinator) runLiveReader(ctx context.Context, current *activeSession) {
	defer func() { current.liveDone <- struct{}{} }()
	for {
		event, err := current.live.NextEvent(ctx)
		if err != nil {
			c.sendWorker(ctx, workerEvent{kind: workerLiveFailure, sessionID: current.id, err: err})
			return
		}
		if !c.sendWorker(ctx, workerEvent{kind: workerLiveEvent, sessionID: current.id, live: event}) {
			return
		}
		if event.ProtocolTerminal {
			return
		}
	}
}

func (c *Coordinator) switchToBatch(current *activeSession) {
	if c.active != current || current.fallback {
		return
	}
	current.liveHealthy = false
	c.stopLive(current)
	if c.batch == nil {
		c.failActive("Transcription provider unavailable")
		return
	}
	current.fallback = true
	c.publish(View{Status: statusFor(current), Partial: "Primary unavailable; using backup", Level: c.view.Level})
	c.startBatchWorker(current)
	for _, segment := range current.retained {
		if !c.enqueueBatch(current, segment) {
			return
		}
	}
	current.retained = nil
	if current.stopped {
		c.closeBatchInput(current)
		c.finishBatchIfReady(current)
	}
}

func (c *Coordinator) startBatchWorker(current *activeSession) {
	current.batchJobs = make(chan []byte, batchQueueSize)
	current.batchDone = make(chan struct{})
	go func() {
		defer close(current.batchDone)
		for {
			select {
			case <-current.ctx.Done():
				return
			case pcm, ok := <-current.batchJobs:
				if !ok {
					return
				}
				text, err := c.batch.Transcribe(current.ctx, pcm)
				if err != nil {
					c.sendWorker(current.ctx, workerEvent{kind: workerBatchFailure, sessionID: current.id, err: err})
					return
				}
				if !c.sendWorker(current.ctx, workerEvent{kind: workerBatchResult, sessionID: current.id, text: text}) {
					return
				}
			}
		}
	}()
}

func (c *Coordinator) enqueueBatch(current *activeSession, segment []byte) bool {
	if current.batchClosed {
		c.failActive("Transcription queue closed")
		return false
	}
	select {
	case current.batchJobs <- segment:
		current.batchPending++
		return true
	default:
		c.failActive("Transcription queue full")
		return false
	}
}

func (c *Coordinator) closeBatchInput(current *activeSession) {
	if !current.fallback || current.batchClosed {
		return
	}
	close(current.batchJobs)
	current.batchClosed = true
}

func (c *Coordinator) finishBatchIfReady(current *activeSession) {
	if current.fallback && current.stopped && current.batchClosed && current.batchPending == 0 {
		c.finalize(current, current.batchText)
	}
}

func (c *Coordinator) sendWorker(ctx context.Context, event workerEvent) bool {
	select {
	case c.workers <- event:
		return true
	case <-ctx.Done():
		return false
	}
}

func (c *Coordinator) stopLive(current *activeSession) {
	if current.live == nil {
		return
	}
	if current.liveCancel != nil {
		current.liveCancel()
	}
	closeCtx, cancel := context.WithTimeout(context.Background(), liveCloseTimeout)
	_ = current.live.Close(closeCtx)
	cancel()
	if current.liveDone != nil {
		<-current.liveDone
		<-current.liveDone
	}
	current.live = nil
	current.liveCancel = nil
	current.liveDone = nil
}

func (c *Coordinator) cleanupSession(current *activeSession) {
	if c.active != current {
		return
	}
	current.cancel()
	c.stopLive(current)
	if current.fallback {
		c.closeBatchInput(current)
		if current.batchDone != nil {
			<-current.batchDone
		}
	}
	c.active = nil
}

func statusFor(current *activeSession) ViewStatus {
	if current.stopped {
		return ViewTranscribing
	}
	return ViewListening
}
