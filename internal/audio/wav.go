package audio

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math"
)

var (
	// ErrEmptyPCM reports an empty PCM segment.
	ErrEmptyPCM = errors.New("PCM16 input must not be empty")
	// ErrWAVTooLarge reports PCM that cannot be represented by a RIFF/WAVE size.
	ErrWAVTooLarge = errors.New("PCM16 input is too large for RIFF/WAVE")
)

const wavHeaderSize = 44

// EncodeWAVPCM16 wraps fixed 16 kHz mono PCM16 LE in an in-memory WAV file.
func EncodeWAVPCM16(pcm []byte) ([]byte, error) {
	if len(pcm) == 0 {
		return nil, ErrEmptyPCM
	}
	if len(pcm)%BytesPerSample != 0 {
		return nil, ErrInvalidPCM
	}
	if uint64(len(pcm))+36 > math.MaxUint32 {
		return nil, ErrWAVTooLarge
	}

	wav := make([]byte, wavHeaderSize+len(pcm))
	copy(wav[0:4], "RIFF")
	binary.LittleEndian.PutUint32(wav[4:8], uint32(len(pcm)+36))
	copy(wav[8:12], "WAVE")
	copy(wav[12:16], "fmt ")
	binary.LittleEndian.PutUint32(wav[16:20], 16)
	binary.LittleEndian.PutUint16(wav[20:22], 1)
	binary.LittleEndian.PutUint16(wav[22:24], checkedUint16(ChannelCount))
	binary.LittleEndian.PutUint32(wav[24:28], SampleRate)
	byteRate := SampleRate * ChannelCount * BytesPerSample
	binary.LittleEndian.PutUint32(wav[28:32], uint32(byteRate))
	binary.LittleEndian.PutUint16(wav[32:34], checkedUint16(ChannelCount*BytesPerSample))
	binary.LittleEndian.PutUint16(wav[34:36], checkedUint16(BytesPerSample*8))
	copy(wav[36:40], "data")
	binary.LittleEndian.PutUint32(wav[40:44], uint32(len(pcm)))
	copy(wav[44:], pcm)
	return wav, nil
}

func checkedUint16(value int) uint16 {
	if value < 0 || value > math.MaxUint16 {
		panic(fmt.Sprintf("WAV field %d exceeds uint16", value))
	}
	return uint16(value)
}
