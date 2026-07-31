package asr

import (
	"context"
	"fmt"
)

// SegmentTranscriber transcribes one complete in-memory PCM segment.
type SegmentTranscriber interface {
	Transcribe(context.Context, []byte) (string, error)
}

// FallbackTranscriber tries a route's suppliers serially for each segment.
type FallbackTranscriber struct {
	route   Route
	engines map[AsrVendor]SegmentTranscriber
}

// NewFallbackTranscriber validates a route and its required supplier engines.
func NewFallbackTranscriber(route Route, registry Registry, engines map[AsrVendor]SegmentTranscriber) (*FallbackTranscriber, error) {
	if err := route.Validate(registry); err != nil {
		return nil, err
	}
	if engines[route.Primary] == nil {
		return nil, fmt.Errorf("primary ASR vendor %q has no transcriber", route.Primary)
	}
	if route.Backup != "" && engines[route.Backup] == nil {
		return nil, fmt.Errorf("backup ASR vendor %q has no transcriber", route.Backup)
	}

	engineCopy := make(map[AsrVendor]SegmentTranscriber, len(engines))
	for vendor, engine := range engines {
		engineCopy[vendor] = engine
	}
	return &FallbackTranscriber{route: route, engines: engineCopy}, nil
}

// Transcribe calls the primary supplier first and only falls back after failure.
func (t *FallbackTranscriber) Transcribe(ctx context.Context, pcm []byte) (string, error) {
	text, primaryErr := t.engines[t.route.Primary].Transcribe(ctx, pcm)
	if primaryErr == nil {
		return text, nil
	}
	if t.route.Backup == "" {
		return "", fmt.Errorf("primary ASR vendor %q failed: %w", t.route.Primary, primaryErr)
	}

	text, backupErr := t.engines[t.route.Backup].Transcribe(ctx, pcm)
	if backupErr == nil {
		return text, nil
	}
	return "", fmt.Errorf(
		"ASR vendors failed: primary %q: %v; backup %q: %v",
		t.route.Primary,
		primaryErr,
		t.route.Backup,
		backupErr,
	)
}
