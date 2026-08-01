package selfcheck

import (
	"errors"
	"time"

	platformwindows "github.com/tossp/voxink/internal/platform/windows"
)

func runAudio(duration time.Duration, deps dependencies) []Check {
	started := deps.now()
	capture, err := deps.newAudio()
	if err != nil {
		return []Check{{Name: "audio_capture", Status: StatusFail, Code: platformCode(err, CodeAudioUnavailable)}}
	}
	format, ok := capture.Format()
	metrics := Metrics{DurationMS: metric(0)}
	if ok {
		metrics.Format = metricEnum(format.Format)
		metrics.Channels = metric(int64(format.Channels))
		metrics.SampleRate = metric(int64(format.SampleRate))
	}
	status, code := StatusPass, CodeOK
	if !ok || format.Format != MetricPCM16LE || format.Channels != 1 || format.SampleRate != 16_000 {
		status, code = StatusFail, CodeAudioFormatMismatch
	} else if err := capture.Start(); err != nil {
		status, code = StatusFail, platformCode(err, CodeAudioStartFailed)
	} else {
		status, code, metrics = collectAudio(capture, duration, metrics)
	}
	if stopErr := capture.Stop(); stopErr != nil && status != StatusFail {
		status, code = StatusFail, CodeAudioCleanupFailed
	}
	if closeErr := capture.Close(); closeErr != nil && status != StatusFail {
		status, code = StatusFail, CodeAudioCleanupFailed
	}
	metrics.DurationMS = metric(deps.now().Sub(started).Milliseconds())
	metrics.OverflowCount = metricUint(capture.OverflowCount())
	return []Check{{Name: "audio_capture", Status: status, Code: code, Metrics: metrics}}
}

func collectAudio(capture audioCapture, duration time.Duration, metrics Metrics) (Status, Code, Metrics) {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	var received, frames, levels int64
	for {
		select {
		case pcm := <-capture.PCM():
			received = boundedAdd(received, int64(len(pcm)))
			frames = boundedAdd(frames, 1)
			// The callback bytes are counted and immediately discarded.
			pcm = nil
		case <-capture.Levels():
			levels = boundedAdd(levels, 1)
		case err := <-capture.Errors():
			metrics.ReceivedBytes, metrics.Frames, metrics.LevelEvents = metric(received), metric(frames), metric(levels)
			if errors.Is(err, platformwindows.ErrPCMOverflow) {
				return StatusFail, CodeAudioRuntimeFailed, metrics
			}
			return StatusFail, platformCode(err, CodeAudioRuntimeFailed), metrics
		case <-timer.C:
			metrics.ReceivedBytes, metrics.Frames, metrics.LevelEvents = metric(received), metric(frames), metric(levels)
			if received == 0 || frames == 0 {
				return StatusFail, CodeAudioTimeout, metrics
			}
			return StatusPass, CodeOK, metrics
		}
	}
}

func boundedAdd(current, delta int64) int64 {
	current = min(maxMetricValue, max(current, 0))
	if delta <= 0 {
		return current
	}
	if delta >= maxMetricValue-current {
		return maxMetricValue
	}
	return current + delta
}

func metricUint(value uint64) *int64 {
	if value > uint64(maxMetricValue) {
		return metric(maxMetricValue)
	}
	return metric(int64(value))
}
