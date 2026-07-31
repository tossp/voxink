package asr

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// ErrProcessorClosed reports an enqueue or repeated finish after input closure.
var ErrProcessorClosed = errors.New("ASR processor input is closed")

// Processor owns one in-memory FIFO and drains it through one transcriber.
type Processor struct {
	transcriber SegmentTranscriber
	queue       [][]byte
	closed      bool
}

// NewProcessor creates a sequential segment processor.
func NewProcessor(transcriber SegmentTranscriber) (*Processor, error) {
	if transcriber == nil {
		return nil, fmt.Errorf("segment transcriber must not be nil")
	}
	return &Processor{transcriber: transcriber}, nil
}

// Enqueue copies one PCM segment to the end of the FIFO.
func (p *Processor) Enqueue(pcm []byte) error {
	if p.closed {
		return ErrProcessorClosed
	}
	p.queue = append(p.queue, append([]byte(nil), pcm...))
	return nil
}

// Finish closes input, drains the FIFO serially, and returns one final text.
func (p *Processor) Finish(ctx context.Context) (string, error) {
	if p.closed {
		return "", ErrProcessorClosed
	}
	p.closed = true

	var final string
	for index, pcm := range p.queue {
		text, err := p.transcriber.Transcribe(ctx, pcm)
		if err != nil {
			return "", fmt.Errorf("transcribe FIFO segment %d: %w", index, err)
		}
		final = JoinText(final, text)
	}
	p.queue = nil
	return final, nil
}

// JoinText appends non-blank text and separates adjacent ASCII words or numbers.
func JoinText(current, next string) string {
	current = strings.TrimSpace(current)
	next = strings.TrimSpace(next)
	if current == "" {
		return next
	}
	if next == "" {
		return current
	}

	last, _ := utf8.DecodeLastRuneInString(current)
	first, _ := utf8.DecodeRuneInString(next)
	if isASCIIAlphanumeric(last) && isASCIIAlphanumeric(first) {
		return current + " " + next
	}
	return current + next
}

func isASCIIAlphanumeric(r rune) bool {
	return r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9'
}
