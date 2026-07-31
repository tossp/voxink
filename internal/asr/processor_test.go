package asr

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type orderedTranscriber struct {
	results map[byte]string
	failOn  byte
	order   []byte
}

func (t *orderedTranscriber) Transcribe(_ context.Context, pcm []byte) (string, error) {
	key := pcm[0]
	t.order = append(t.order, key)
	if key == t.failOn {
		return "", errors.New("segment failed")
	}
	return t.results[key], nil
}

func TestProcessorDrainsFIFOAndJoinsText(t *testing.T) {
	transcriber := &orderedTranscriber{results: map[byte]string{
		1: "你好",
		2: "world",
		3: "   ",
		4: "2026",
	}}
	processor, err := NewProcessor(transcriber)
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}
	for _, segment := range [][]byte{{1}, {2}, {3}, {4}} {
		if err := processor.Enqueue(segment); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	got, err := processor.Finish(context.Background())
	if err != nil {
		t.Fatalf("Finish() error = %v", err)
	}
	if got != "你好world 2026" {
		t.Fatalf("Finish() = %q, want %q", got, "你好world 2026")
	}
	if want := []byte{1, 2, 3, 4}; !reflect.DeepEqual(transcriber.order, want) {
		t.Fatalf("transcription order = %v, want %v", transcriber.order, want)
	}
}

func TestProcessorStopsOnSegmentFailure(t *testing.T) {
	transcriber := &orderedTranscriber{
		results: map[byte]string{1: "first", 3: "third"},
		failOn:  2,
	}
	processor, err := NewProcessor(transcriber)
	if err != nil {
		t.Fatalf("NewProcessor() error = %v", err)
	}
	for _, segment := range [][]byte{{1}, {2}, {3}} {
		if err := processor.Enqueue(segment); err != nil {
			t.Fatalf("Enqueue() error = %v", err)
		}
	}

	if _, err := processor.Finish(context.Background()); err == nil {
		t.Fatal("Finish() error = nil, want segment failure")
	}
	if want := []byte{1, 2}; !reflect.DeepEqual(transcriber.order, want) {
		t.Fatalf("transcription order = %v, want %v", transcriber.order, want)
	}
}

func TestJoinTextASCIIAndChineseBoundaries(t *testing.T) {
	tests := []struct {
		current string
		next    string
		want    string
	}{
		{current: "hello", next: "world", want: "hello world"},
		{current: "version", next: "2", want: "version 2"},
		{current: "你好", next: "世界", want: "你好世界"},
		{current: "你好", next: "Go", want: "你好Go"},
		{current: "done.", next: "Next", want: "done.Next"},
		{current: "kept", next: "   ", want: "kept"},
	}
	for _, tt := range tests {
		if got := JoinText(tt.current, tt.next); got != tt.want {
			t.Errorf("JoinText(%q, %q) = %q, want %q", tt.current, tt.next, got, tt.want)
		}
	}
}
