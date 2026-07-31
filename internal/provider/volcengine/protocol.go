// Package volcengine implements the Volcengine V3 true-streaming ASR protocol.
package volcengine

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/tossp/voxink/internal/asr"
)

const (
	protocolVersion = 1
	headerWords     = 1

	messageFullClient  = 0x1
	messageAudioOnly   = 0x2
	messageFullServer  = 0x9
	messageServerError = 0xf

	flagNoSequence       = 0x0
	flagPositiveSequence = 0x1
	flagLastNoSequence   = 0x2

	serializationNone = 0x0
	serializationJSON = 0x1

	compressionNone = 0x0
	compressionGZIP = 0x1
)

var errInvalidFrame = errors.New("invalid Volcengine protocol frame")

// RequestConfig contains the implementation-supported full request options.
type RequestConfig struct {
	UserID     string
	EnableITN  bool
	EnablePunc bool
}

type fullRequest struct {
	User    requestUser  `json:"user,omitempty"`
	Audio   requestAudio `json:"audio"`
	Request requestBody  `json:"request"`
}

type requestUser struct {
	UID string `json:"uid,omitempty"`
}

type requestAudio struct {
	Format  string `json:"format"`
	Codec   string `json:"codec"`
	Rate    int    `json:"rate"`
	Bits    int    `json:"bits"`
	Channel int    `json:"channel"`
}

type requestBody struct {
	ModelName      string `json:"model_name"`
	EnableITN      bool   `json:"enable_itn"`
	EnablePunc     bool   `json:"enable_punc"`
	ShowUtterances bool   `json:"show_utterances"`
}

type responsePayload struct {
	Result responseResult `json:"result"`
}

type responseResult struct {
	Text       string              `json:"text"`
	Utterances []responseUtterance `json:"utterances"`
}

type responseUtterance struct {
	Text      string `json:"text"`
	StartTime int    `json:"start_time"`
	EndTime   int    `json:"end_time"`
	Definite  bool   `json:"definite"`
}

// ServerError is a sanitized Volcengine protocol error response.
type ServerError struct {
	Code uint32
}

func (e *ServerError) Error() string {
	return fmt.Sprintf("Volcengine protocol error code %d", e.Code)
}

func encodeFullRequest(config RequestConfig) ([]byte, error) {
	payload, err := json.Marshal(fullRequest{
		User: requestUser{UID: config.UserID},
		Audio: requestAudio{
			Format: "pcm", Codec: "raw", Rate: 16_000, Bits: 16, Channel: 1,
		},
		Request: requestBody{
			ModelName: "bigmodel", EnableITN: config.EnableITN,
			EnablePunc: config.EnablePunc, ShowUtterances: true,
		},
	})
	if err != nil {
		return nil, fmt.Errorf("encode Volcengine full request JSON: %w", err)
	}
	return encodeFrame(messageFullClient, flagNoSequence, serializationJSON, compressionGZIP, payload)
}

func encodeAudio(pcm []byte, last bool) ([]byte, error) {
	flag := byte(flagNoSequence)
	if last {
		flag = flagLastNoSequence
	}
	return encodeFrame(messageAudioOnly, flag, serializationNone, compressionGZIP, pcm)
}

func encodeFrame(messageType, flags, serialization, compression byte, payload []byte) ([]byte, error) {
	if compression == compressionGZIP {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(payload); err != nil {
			return nil, fmt.Errorf("compress Volcengine payload: %w", err)
		}
		if err := writer.Close(); err != nil {
			return nil, fmt.Errorf("finish Volcengine payload compression: %w", err)
		}
		payload = compressed.Bytes()
	}
	if uint64(len(payload)) > uint64(^uint32(0)) {
		return nil, fmt.Errorf("Volcengine payload is too large")
	}

	frame := make([]byte, 8+len(payload))
	frame[0] = protocolVersion<<4 | headerWords
	frame[1] = messageType<<4 | flags
	frame[2] = serialization<<4 | compression
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	copy(frame[8:], payload)
	return frame, nil
}

func decodeServerFrame(frame []byte) (asr.LiveEvent, error) {
	if len(frame) < 4 {
		return asr.LiveEvent{}, fmt.Errorf("%w: header is truncated", errInvalidFrame)
	}
	if frame[0]>>4 != protocolVersion {
		return asr.LiveEvent{}, fmt.Errorf("%w: unsupported version %d", errInvalidFrame, frame[0]>>4)
	}
	headerSize := int(frame[0]&0x0f) * 4
	if headerSize < 4 || headerSize > len(frame) {
		return asr.LiveEvent{}, fmt.Errorf("%w: invalid header size %d", errInvalidFrame, headerSize)
	}
	messageType := frame[1] >> 4
	flags := frame[1] & 0x0f
	serialization := frame[2] >> 4
	compression := frame[2] & 0x0f
	payload := frame[headerSize:]

	switch messageType {
	case messageFullServer:
		return decodeFullServer(flags, serialization, compression, payload)
	case messageServerError:
		return asr.LiveEvent{}, decodeServerError(compression, payload)
	default:
		return asr.LiveEvent{}, fmt.Errorf("%w: unsupported message type %d", errInvalidFrame, messageType)
	}
}

func decodeFullServer(flags, serialization, compression byte, payload []byte) (asr.LiveEvent, error) {
	event := asr.LiveEvent{ProtocolTerminal: flags&flagLastNoSequence != 0}
	if flags&flagPositiveSequence != 0 {
		if len(payload) < 4 {
			return asr.LiveEvent{}, fmt.Errorf("%w: response sequence is truncated", errInvalidFrame)
		}
		event.Sequence = int32(binary.BigEndian.Uint32(payload[:4]))
		event.HasSequence = true
		payload = payload[4:]
	}
	if serialization != serializationJSON {
		return asr.LiveEvent{}, fmt.Errorf("%w: response serialization %d is not JSON", errInvalidFrame, serialization)
	}
	body, err := sizedPayload(payload, compression)
	if err != nil {
		return asr.LiveEvent{}, err
	}
	var decoded responsePayload
	if err := json.Unmarshal(body, &decoded); err != nil {
		return asr.LiveEvent{}, fmt.Errorf("%w: decode response JSON: %v", errInvalidFrame, err)
	}
	event.Text = decoded.Result.Text
	for _, utterance := range decoded.Result.Utterances {
		event.Utterances = append(event.Utterances, asr.LiveUtterance{
			Text: utterance.Text, StartMS: utterance.StartTime,
			EndMS: utterance.EndTime, Stable: utterance.Definite,
		})
	}
	return event, nil
}

func decodeServerError(compression byte, payload []byte) error {
	if len(payload) < 8 {
		return fmt.Errorf("%w: error response is truncated", errInvalidFrame)
	}
	code := binary.BigEndian.Uint32(payload[:4])
	_, err := sizedPayload(payload[4:], compression)
	if err != nil {
		return err
	}
	return &ServerError{Code: code}
}

func sizedPayload(payload []byte, compression byte) ([]byte, error) {
	if len(payload) < 4 {
		return nil, fmt.Errorf("%w: payload size is truncated", errInvalidFrame)
	}
	size := binary.BigEndian.Uint32(payload[:4])
	if uint64(size) != uint64(len(payload)-4) {
		return nil, fmt.Errorf("%w: declared payload size %d differs from %d", errInvalidFrame, size, len(payload)-4)
	}
	body := payload[4:]
	switch compression {
	case compressionNone:
		return body, nil
	case compressionGZIP:
		reader, err := gzip.NewReader(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("%w: open gzip payload: %v", errInvalidFrame, err)
		}
		decompressed, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil {
			return nil, fmt.Errorf("%w: decompress payload: %v", errInvalidFrame, readErr)
		}
		if closeErr != nil {
			return nil, fmt.Errorf("%w: close gzip payload: %v", errInvalidFrame, closeErr)
		}
		return decompressed, nil
	default:
		return nil, fmt.Errorf("%w: unsupported compression %d", errInvalidFrame, compression)
	}
}
