package smoke

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/tossp/voxink/internal/audio"
)

func TestParseWAVValidExtraChunksAndDuration(t *testing.T) {
	pcm := make([]byte, 32_000)
	wav := wavWithChunks(t, pcm, true)
	input, err := parseWAV(wav)
	if err != nil {
		t.Fatalf("parseWAV() error = %v", err)
	}
	if input.durationMS != 1000 || len(input.pcm) != len(pcm) || len(input.wav) != len(wav) {
		t.Fatalf("input duration/bytes = %d/%d/%d", input.durationMS, len(input.pcm), len(input.wav))
	}
	input.release()
	if input.wav != nil || input.pcm != nil {
		t.Fatal("release retained audio payload")
	}
}

func TestParseWAVRejectsFormatMismatchAndTruncation(t *testing.T) {
	valid := wavWithChunks(t, []byte{1, 2, 3, 4}, false)
	tests := map[string][]byte{
		"not wave":          []byte("not a wave"),
		"truncated":         valid[:len(valid)-1],
		"wrong sample rate": append([]byte(nil), valid...),
		"missing data":      valid[:36],
	}
	binary.LittleEndian.PutUint32(tests["wrong sample rate"][24:28], 44_100)
	for name, data := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseWAV(data); !errors.Is(err, errAudioInvalid) {
				t.Fatalf("parseWAV() error = %v, want audio invalid", err)
			}
		})
	}
}

func TestReadWAVBoundsInput(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "authorized.wav")
	if err := os.WriteFile(path, make([]byte, MaximumAudioBytes+1), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := readWAV(path); !errors.Is(err, errAudioTooLarge) {
		t.Fatalf("readWAV() error = %v, want too large", err)
	}
	if _, err := readWAV(filepath.Join(directory, "missing.wav")); !errors.Is(err, errAudioUnavailable) {
		t.Fatalf("readWAV(missing) error = %v, want unavailable", err)
	}
}

func TestParseWAVRejectsPCMOverSixtySeconds(t *testing.T) {
	wav := wavWithChunks(t, make([]byte, maximumPCMBytes+audio.BytesPerSample), false)
	if _, err := parseWAV(wav); !errors.Is(err, errAudioTooLarge) {
		t.Fatalf("parseWAV() error = %v, want too large", err)
	}
}

func wavWithChunks(t *testing.T, pcm []byte, extra bool) []byte {
	t.Helper()
	wav, err := audio.EncodeWAVPCM16(pcm)
	if err != nil {
		t.Fatal(err)
	}
	if !extra {
		return wav
	}
	junk := []byte{'J', 'U', 'N', 'K', 3, 0, 0, 0, 1, 2, 3, 0}
	result := append([]byte(nil), wav[:36]...)
	result = append(result, junk...)
	result = append(result, wav[36:]...)
	binary.LittleEndian.PutUint32(result[4:8], uint32(len(result)-8))
	return result
}
