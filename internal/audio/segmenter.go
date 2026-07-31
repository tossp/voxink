// Package audio provides the fixed PCM contract and progressive segmentation.
package audio

import (
	"errors"
	"fmt"
	"time"
)

const (
	// SampleRate is the required PCM sample rate in hertz.
	SampleRate = 16_000
	// ChannelCount is the required mono channel count.
	ChannelCount = 1
	// BytesPerSample is the signed PCM16 sample width.
	BytesPerSample = 2
)

const (
	// MinimumSpeechDuration is the speech needed for an automatic silence split.
	MinimumSpeechDuration = 500 * time.Millisecond
	// SilenceSplitDuration triggers an automatic split after continuous silence.
	SilenceSplitDuration = 600 * time.Millisecond
	// MaximumContinuousSpeechDuration forces a split without a pause.
	MaximumContinuousSpeechDuration = 15 * time.Second
	// MaximumSessionDuration is the hard accepted-audio limit.
	MaximumSessionDuration = 60 * time.Second
)

var (
	// ErrInvalidPCM reports input that is not complete little-endian PCM16 samples.
	ErrInvalidPCM = errors.New("PCM16 input must contain an even number of bytes")
	// ErrSessionLimitReached reports audio submitted after the 60-second limit.
	ErrSessionLimitReached = errors.New("audio session limit reached")
	// ErrSegmenterClosed reports audio submitted after Finish.
	ErrSegmenterClosed = errors.New("audio segmenter is closed")
)

// AppendResult contains newly sealed segments and session-limit status.
type AppendResult struct {
	Segments     [][]byte
	LimitReached bool
}

// ProgressiveSegmenter incrementally splits caller-labelled speech and silence.
type ProgressiveSegmenter struct {
	buffer           []byte
	totalSamples     int
	speechSamples    int
	trailingSilence  int
	continuousSpeech int
	limitReached     bool
	closed           bool
}

// NewProgressiveSegmenter creates a segmenter for fixed 16 kHz mono PCM16 LE.
func NewProgressiveSegmenter() *ProgressiveSegmenter {
	return &ProgressiveSegmenter{}
}

// Append accepts PCM whose whole frame is labelled as speech or silence.
// If a frame crosses the session limit, only the prefix up to 60 seconds is
// accepted and LimitReached is returned with all accepted audio flushed.
func (s *ProgressiveSegmenter) Append(pcm []byte, speech bool) (AppendResult, error) {
	if len(pcm)%BytesPerSample != 0 {
		return AppendResult{LimitReached: s.limitReached}, ErrInvalidPCM
	}
	if s.closed {
		return AppendResult{LimitReached: s.limitReached}, ErrSegmenterClosed
	}
	if s.limitReached {
		return AppendResult{LimitReached: true}, ErrSessionLimitReached
	}

	inputSamples := len(pcm) / BytesPerSample
	remaining := durationSamples(MaximumSessionDuration) - s.totalSamples
	acceptedSamples := min(inputSamples, remaining)
	accepted := pcm[:acceptedSamples*BytesPerSample]

	var segments [][]byte
	if speech {
		segments = s.appendSpeech(accepted)
	} else {
		segments = s.appendSilence(accepted)
	}
	s.totalSamples += acceptedSamples

	if s.totalSamples == durationSamples(MaximumSessionDuration) {
		s.limitReached = true
		if len(s.buffer) > 0 {
			segments = append(segments, s.takeBuffer())
		}
	}
	return AppendResult{Segments: segments, LimitReached: s.limitReached}, nil
}

// Finish closes input and flushes a non-empty tail regardless of its duration.
func (s *ProgressiveSegmenter) Finish() ([][]byte, error) {
	if s.closed {
		return nil, ErrSegmenterClosed
	}
	s.closed = true
	if len(s.buffer) == 0 {
		return nil, nil
	}
	return [][]byte{s.takeBuffer()}, nil
}

func (s *ProgressiveSegmenter) appendSpeech(pcm []byte) [][]byte {
	s.trailingSilence = 0
	remainingSamples := len(pcm) / BytesPerSample
	offsetSamples := 0
	var segments [][]byte
	maximum := durationSamples(MaximumContinuousSpeechDuration)

	for remainingSamples > 0 {
		untilSplit := maximum - s.continuousSpeech
		count := min(remainingSamples, untilSplit)
		start := offsetSamples * BytesPerSample
		end := (offsetSamples + count) * BytesPerSample
		s.buffer = append(s.buffer, pcm[start:end]...)
		s.speechSamples += count
		s.continuousSpeech += count
		offsetSamples += count
		remainingSamples -= count

		if s.continuousSpeech == maximum {
			segments = append(segments, s.takeBuffer())
		}
	}
	return segments
}

func (s *ProgressiveSegmenter) appendSilence(pcm []byte) [][]byte {
	s.buffer = append(s.buffer, pcm...)
	s.trailingSilence += len(pcm) / BytesPerSample
	s.continuousSpeech = 0

	if s.speechSamples < durationSamples(MinimumSpeechDuration) ||
		s.trailingSilence < durationSamples(SilenceSplitDuration) {
		return nil
	}

	bufferSamples := len(s.buffer) / BytesPerSample
	cutSamples := bufferSamples - s.trailingSilence + s.trailingSilence/2
	cutBytes := cutSamples * BytesPerSample
	segment := append([]byte(nil), s.buffer[:cutBytes]...)
	s.buffer = append(s.buffer[:0], s.buffer[cutBytes:]...)
	s.trailingSilence = len(s.buffer) / BytesPerSample
	s.speechSamples = 0
	return [][]byte{segment}
}

func (s *ProgressiveSegmenter) takeBuffer() []byte {
	segment := append([]byte(nil), s.buffer...)
	s.buffer = nil
	s.speechSamples = 0
	s.trailingSilence = 0
	s.continuousSpeech = 0
	return segment
}

func durationSamples(duration time.Duration) int {
	samples := int64(SampleRate) * int64(duration) / int64(time.Second)
	if samples > int64(^uint(0)>>1) {
		panic(fmt.Sprintf("duration %s exceeds sample count capacity", duration))
	}
	return int(samples)
}
