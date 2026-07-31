package audio

import (
	"bytes"
	"errors"
	"testing"
	"time"
)

func TestSilenceSplitUsesMidpointAndPreservesPCM(t *testing.T) {
	segmenter := NewProgressiveSegmenter()
	speech := testPCM(durationSamples(500*time.Millisecond), 1)
	silence := testPCM(durationSamples(600*time.Millisecond), 2)
	input := append(append([]byte(nil), speech...), silence...)

	result, err := segmenter.Append(speech, true)
	if err != nil || len(result.Segments) != 0 {
		t.Fatalf("speech Append() = %+v, %v", result, err)
	}
	result, err = segmenter.Append(silence, false)
	if err != nil {
		t.Fatalf("silence Append() error = %v", err)
	}
	if len(result.Segments) != 1 {
		t.Fatalf("automatic segments = %d, want 1", len(result.Segments))
	}
	wantFirstBytes := durationSamples(800*time.Millisecond) * BytesPerSample
	if len(result.Segments[0]) != wantFirstBytes {
		t.Fatalf("first segment bytes = %d, want %d", len(result.Segments[0]), wantFirstBytes)
	}
	tail, err := segmenter.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	assertPCMConserved(t, input, append(result.Segments, tail...))
}

func TestShortSpeechDoesNotAutoSplitButFinishFlushes(t *testing.T) {
	segmenter := NewProgressiveSegmenter()
	speech := testPCM(durationSamples(499*time.Millisecond), 3)
	silence := testPCM(durationSamples(600*time.Millisecond), 4)
	input := append(append([]byte(nil), speech...), silence...)

	first, err := segmenter.Append(speech, true)
	if err != nil {
		t.Fatalf("Append(speech) error = %v", err)
	}
	second, err := segmenter.Append(silence, false)
	if err != nil {
		t.Fatalf("Append(silence) error = %v", err)
	}
	if len(first.Segments)+len(second.Segments) != 0 {
		t.Fatal("short speech was automatically sealed")
	}
	tail, err := segmenter.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if len(tail) != 1 {
		t.Fatalf("Finish() segments = %d, want 1", len(tail))
	}
	assertPCMConserved(t, input, tail)
}

func TestContinuousSpeechHardSplitPreservesPCM(t *testing.T) {
	segmenter := NewProgressiveSegmenter()
	input := testPCM(durationSamples(15*time.Second+250*time.Millisecond), 5)

	result, err := segmenter.Append(input, true)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if len(result.Segments) != 1 {
		t.Fatalf("hard segments = %d, want 1", len(result.Segments))
	}
	wantBytes := durationSamples(15*time.Second) * BytesPerSample
	if len(result.Segments[0]) != wantBytes {
		t.Fatalf("hard segment bytes = %d, want %d", len(result.Segments[0]), wantBytes)
	}
	tail, err := segmenter.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	assertPCMConserved(t, input, append(result.Segments, tail...))
}

func TestSessionLimitFlushesAcceptedPCMAndRejectsMore(t *testing.T) {
	segmenter := NewProgressiveSegmenter()
	initialSilence := testPCM(durationSamples(time.Second), 6)
	speech := testPCM(durationSamples(60*time.Second), 7)
	acceptedSpeech := speech[:durationSamples(59*time.Second)*BytesPerSample]
	accepted := append(append([]byte(nil), initialSilence...), acceptedSpeech...)

	initial, err := segmenter.Append(initialSilence, false)
	if err != nil {
		t.Fatalf("Append(initial silence) error = %v", err)
	}
	if len(initial.Segments) != 0 || initial.LimitReached {
		t.Fatalf("Append(initial silence) = %+v", initial)
	}

	result, err := segmenter.Append(speech, true)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	if !result.LimitReached {
		t.Fatal("LimitReached = false, want true")
	}
	if len(result.Segments) != 4 {
		t.Fatalf("segments at limit = %d, want 4", len(result.Segments))
	}
	wantTailBytes := durationSamples(14*time.Second) * BytesPerSample
	if got := len(result.Segments[len(result.Segments)-1]); got != wantTailBytes {
		t.Fatalf("limit-flushed tail bytes = %d, want %d", got, wantTailBytes)
	}
	assertPCMConserved(t, accepted, result.Segments)

	late, err := segmenter.Append([]byte{0, 0}, true)
	if !errors.Is(err, ErrSessionLimitReached) || !late.LimitReached {
		t.Fatalf("late Append() = %+v, %v", late, err)
	}
	tail, err := segmenter.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if len(tail) != 0 {
		t.Fatalf("Finish() after limit returned %d segments", len(tail))
	}
}

func TestOddPCMBytesAreRejectedWithoutMutation(t *testing.T) {
	segmenter := NewProgressiveSegmenter()
	if _, err := segmenter.Append([]byte{1}, true); !errors.Is(err, ErrInvalidPCM) {
		t.Fatalf("Append(odd bytes) error = %v, want %v", err, ErrInvalidPCM)
	}
	valid := []byte{2, 3}
	if _, err := segmenter.Append(valid, true); err != nil {
		t.Fatalf("Append(valid) error = %v", err)
	}
	tail, err := segmenter.Finish()
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	assertPCMConserved(t, valid, tail)
}

func testPCM(samples int, seed byte) []byte {
	pcm := make([]byte, samples*BytesPerSample)
	for i := range pcm {
		pcm[i] = byte(int(seed) + i%251)
	}
	return pcm
}

func assertPCMConserved(t *testing.T, input []byte, segments [][]byte) {
	t.Helper()
	var joined []byte
	for _, segment := range segments {
		joined = append(joined, segment...)
	}
	if !bytes.Equal(joined, input) {
		t.Fatalf("joined PCM differs: got %d bytes, want %d", len(joined), len(input))
	}
}
