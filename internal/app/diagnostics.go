package app

import (
	"errors"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/audio"
	"github.com/tossp/voxink/internal/diagnostic"
	platformwindows "github.com/tossp/voxink/internal/platform/windows"
)

func (c *Coordinator) recordDiagnostic(current *activeSession, kind diagnostic.Kind, stage diagnostic.Stage, vendor asr.AsrVendor, code diagnostic.Code) {
	event, err := diagnostic.NewEvent(diagnostic.EventInput{
		SessionID: current.id, Kind: kind, Stage: stage, AsrVendor: vendor, Code: code,
		Count: uint64(current.accepted), DurationMS: audioDurationMS(current.accepted),
	})
	if err == nil {
		c.diagnostics.Record(event)
	}
}

func (c *Coordinator) handleCaptureError(err error) {
	code := diagnostic.CodeCaptureInternal
	if errors.Is(err, platformwindows.ErrPCMOverflow) {
		code = diagnostic.CodeCaptureOverflow
	}
	c.failCapture("Capture failed", code)
}

func (c *Coordinator) failCapture(message string, code diagnostic.Code) {
	current := c.active
	if current == nil {
		return
	}
	c.recordDiagnostic(current, diagnostic.KindCaptureFault, diagnostic.StageCapture, "", code)
	c.failActive(message, diagnostic.StageCapture, code)
}

func audioDurationMS(bytes int) uint64 {
	if bytes <= 0 {
		return 0
	}
	return uint64(bytes) * 1_000 / uint64(audio.SampleRate*audio.BytesPerSample)
}
