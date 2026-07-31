package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestEncodeWAVPCM16(t *testing.T) {
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	wav, err := EncodeWAVPCM16(pcm)
	if err != nil {
		t.Fatalf("EncodeWAVPCM16() error = %v", err)
	}
	if string(wav[0:4]) != "RIFF" || string(wav[8:12]) != "WAVE" || string(wav[36:40]) != "data" {
		t.Fatalf("unexpected WAV identifiers: %q %q %q", wav[0:4], wav[8:12], wav[36:40])
	}
	checks := map[string]struct {
		got  uint32
		want uint32
	}{
		"RIFF size":   {binary.LittleEndian.Uint32(wav[4:8]), 40},
		"sample rate": {binary.LittleEndian.Uint32(wav[24:28]), 16_000},
		"byte rate":   {binary.LittleEndian.Uint32(wav[28:32]), 32_000},
		"data size":   {binary.LittleEndian.Uint32(wav[40:44]), 4},
	}
	for name, check := range checks {
		if check.got != check.want {
			t.Errorf("%s = %d, want %d", name, check.got, check.want)
		}
	}
	if binary.LittleEndian.Uint16(wav[20:22]) != 1 ||
		binary.LittleEndian.Uint16(wav[22:24]) != 1 ||
		binary.LittleEndian.Uint16(wav[34:36]) != 16 {
		t.Fatalf("unexpected PCM format fields: %v", wav[20:36])
	}
	if !bytes.Equal(wav[44:], pcm) {
		t.Fatalf("WAV PCM = %v, want %v", wav[44:], pcm)
	}
}

func TestEncodeWAVPCM16RejectsInvalidPCM(t *testing.T) {
	for _, tt := range []struct {
		name string
		pcm  []byte
		want error
	}{
		{name: "empty", want: ErrEmptyPCM},
		{name: "odd length", pcm: []byte{1}, want: ErrInvalidPCM},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := EncodeWAVPCM16(tt.pcm)
			if !errors.Is(err, tt.want) {
				t.Fatalf("EncodeWAVPCM16() error = %v, want %v", err, tt.want)
			}
		})
	}
}
