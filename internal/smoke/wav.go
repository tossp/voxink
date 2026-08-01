package smoke

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/tossp/voxink/internal/audio"
)

var (
	errAudioUnavailable = errors.New("audio unavailable")
	errAudioTooLarge    = errors.New("audio too large")
	errAudioInvalid     = errors.New("audio invalid")
)

const maximumPCMBytes = audio.SampleRate * audio.ChannelCount * audio.BytesPerSample * 60

type audioInput struct {
	wav        []byte
	pcm        []byte
	durationMS int64
}

func readWAV(path string) (*audioInput, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("%w", errAudioUnavailable)
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, MaximumAudioBytes+1))
	if err != nil {
		return nil, fmt.Errorf("%w", errAudioUnavailable)
	}
	if int64(len(data)) > MaximumAudioBytes {
		return nil, errAudioTooLarge
	}
	input, err := parseWAV(data)
	if err != nil {
		return nil, err
	}
	return input, nil
}

func parseWAV(data []byte) (*audioInput, error) {
	if len(data) < 12 || string(data[:4]) != "RIFF" || string(data[8:12]) != "WAVE" {
		return nil, errAudioInvalid
	}
	if uint64(binary.LittleEndian.Uint32(data[4:8]))+8 != uint64(len(data)) {
		return nil, errAudioInvalid
	}

	var pcm []byte
	foundFormat := false
	for offset := 12; offset < len(data); {
		if len(data)-offset < 8 {
			return nil, errAudioInvalid
		}
		chunkID := string(data[offset : offset+4])
		size := int64(binary.LittleEndian.Uint32(data[offset+4 : offset+8]))
		start := int64(offset) + 8
		end := start + size
		paddedEnd := end + size%2
		if end < start || paddedEnd > int64(len(data)) {
			return nil, errAudioInvalid
		}
		chunk := data[start:end]
		switch chunkID {
		case "fmt ":
			if foundFormat || !validPCMFormat(chunk) {
				return nil, errAudioInvalid
			}
			foundFormat = true
		case "data":
			if pcm != nil || len(chunk) == 0 || len(chunk)%audio.BytesPerSample != 0 {
				return nil, errAudioInvalid
			}
			pcm = chunk
		}
		offset = int(paddedEnd)
	}
	if !foundFormat || pcm == nil {
		return nil, errAudioInvalid
	}
	if len(pcm) > maximumPCMBytes {
		return nil, errAudioTooLarge
	}
	return &audioInput{
		wav: data, pcm: pcm,
		durationMS: int64(len(pcm)) * 1000 / int64(audio.SampleRate*audio.ChannelCount*audio.BytesPerSample),
	}, nil
}

func validPCMFormat(chunk []byte) bool {
	if len(chunk) < 16 {
		return false
	}
	return binary.LittleEndian.Uint16(chunk[0:2]) == 1 &&
		binary.LittleEndian.Uint16(chunk[2:4]) == audio.ChannelCount &&
		binary.LittleEndian.Uint32(chunk[4:8]) == audio.SampleRate &&
		binary.LittleEndian.Uint32(chunk[8:12]) == audio.SampleRate*audio.ChannelCount*audio.BytesPerSample &&
		binary.LittleEndian.Uint16(chunk[12:14]) == audio.ChannelCount*audio.BytesPerSample &&
		binary.LittleEndian.Uint16(chunk[14:16]) == audio.BytesPerSample*8
}

func (a *audioInput) release() {
	a.wav = nil
	a.pcm = nil
}
