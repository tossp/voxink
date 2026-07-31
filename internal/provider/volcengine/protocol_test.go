package volcengine

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestEncodeFullRequest(t *testing.T) {
	frame, err := encodeFullRequest(RequestConfig{UserID: "local-user", EnableITN: true, EnablePunc: true})
	if err != nil {
		t.Fatalf("encodeFullRequest() error = %v", err)
	}
	if got, want := frame[:4], []byte{0x11, 0x10, 0x11, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("header = %x, want %x", got, want)
	}
	payload := decodeTestSizedGZIP(t, frame[4:])
	var request fullRequest
	if err := json.Unmarshal(payload, &request); err != nil {
		t.Fatalf("decode request JSON: %v", err)
	}
	if request.User.UID != "local-user" || request.Audio.Format != "pcm" || request.Audio.Codec != "raw" {
		t.Fatalf("unexpected request metadata: %+v", request)
	}
	if request.Audio.Rate != 16_000 || request.Audio.Bits != 16 || request.Audio.Channel != 1 {
		t.Fatalf("unexpected audio contract: %+v", request.Audio)
	}
	if request.Request.ModelName != "bigmodel" || !request.Request.ShowUtterances ||
		!request.Request.EnableITN || !request.Request.EnablePunc {
		t.Fatalf("unexpected ASR request: %+v", request.Request)
	}
}

func TestEncodeAudioFrames(t *testing.T) {
	pcm := []byte{1, 2, 3, 4}
	regular, err := encodeAudio(pcm, false)
	if err != nil {
		t.Fatalf("encodeAudio(regular) error = %v", err)
	}
	last, err := encodeAudio(nil, true)
	if err != nil {
		t.Fatalf("encodeAudio(last) error = %v", err)
	}
	if got, want := regular[:4], []byte{0x11, 0x20, 0x01, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("regular header = %x, want %x", got, want)
	}
	if got, want := last[:4], []byte{0x11, 0x22, 0x01, 0x00}; !bytes.Equal(got, want) {
		t.Fatalf("last header = %x, want %x", got, want)
	}
	if got := decodeTestSizedGZIP(t, regular[4:]); !bytes.Equal(got, pcm) {
		t.Fatalf("regular PCM = %x, want %x", got, pcm)
	}
	if got := decodeTestSizedGZIP(t, last[4:]); len(got) != 0 {
		t.Fatalf("last PCM length = %d, want 0", len(got))
	}
}

func TestDecodeFullServerResponse(t *testing.T) {
	body := []byte(`{"result":{"text":"你好","utterances":[{"text":"你","start_time":10,"end_time":20,"definite":true},{"text":"好","start_time":20,"end_time":30,"definite":false}]}}`)
	frame := testServerFrame(t, flagPositiveSequence|flagLastNoSequence, compressionGZIP, 7, body)
	event, err := decodeServerFrame(frame)
	if err != nil {
		t.Fatalf("decodeServerFrame() error = %v", err)
	}
	if event.Text != "你好" || !event.HasSequence || event.Sequence != 7 || !event.ProtocolTerminal {
		t.Fatalf("unexpected event: %+v", event)
	}
	if len(event.Utterances) != 2 || !event.Utterances[0].Stable || event.Utterances[1].Stable {
		t.Fatalf("definite mapping = %+v", event.Utterances)
	}
}

func TestDecodeServerError(t *testing.T) {
	body := []byte(`{"message":"bad local fixture","request":"must not be echoed"}`)
	frame := make([]byte, 12+len(body))
	copy(frame[:4], []byte{0x11, 0xf0, 0x10, 0x00})
	binary.BigEndian.PutUint32(frame[4:8], 45000001)
	binary.BigEndian.PutUint32(frame[8:12], uint32(len(body)))
	copy(frame[12:], body)
	_, err := decodeServerFrame(frame)
	var serverErr *ServerError
	if !errors.As(err, &serverErr) {
		t.Fatalf("decodeServerFrame() error = %v, want ServerError", err)
	}
	if serverErr.Code != 45000001 {
		t.Fatalf("server error = %+v", serverErr)
	}
	if strings.Contains(err.Error(), "fixture") || strings.Contains(err.Error(), "request") {
		t.Fatalf("error echoed server body: %q", err)
	}
}

func TestDecodeRejectsInvalidFrames(t *testing.T) {
	valid := testServerFrame(t, flagPositiveSequence, compressionGZIP, 1, []byte(`{"result":{"text":"ok"}}`))
	tests := []struct {
		name  string
		frame []byte
	}{
		{name: "short header", frame: []byte{0x11}},
		{name: "wrong version", frame: append([]byte{0x21}, valid[1:]...)},
		{name: "oversized header", frame: []byte{0x12, 0x90, 0x11, 0}},
		{name: "unsupported message", frame: []byte{0x11, 0x30, 0, 0}},
		{name: "truncated sequence", frame: []byte{0x11, 0x91, 0x11, 0, 0, 0}},
		{name: "wrong size", frame: append(append([]byte(nil), valid[:8]...), valid[9:]...)},
		{name: "unsupported compression", frame: append([]byte{valid[0], valid[1], 0x1f, valid[3]}, valid[4:]...)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := decodeServerFrame(tt.frame); err == nil {
				t.Fatal("decodeServerFrame() error = nil")
			}
		})
	}
}

func decodeTestSizedGZIP(t *testing.T, sized []byte) []byte {
	t.Helper()
	if len(sized) < 4 {
		t.Fatalf("sized payload is truncated: %x", sized)
	}
	size := binary.BigEndian.Uint32(sized[:4])
	if int(size) != len(sized)-4 {
		t.Fatalf("payload size = %d, bytes = %d", size, len(sized)-4)
	}
	reader, err := gzip.NewReader(bytes.NewReader(sized[4:]))
	if err != nil {
		t.Fatalf("gzip.NewReader() error = %v", err)
	}
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll(gzip) error = %v", err)
	}
	if err := reader.Close(); err != nil {
		t.Fatalf("gzip Close() error = %v", err)
	}
	return body
}

func testServerFrame(t *testing.T, flags, compression byte, sequence int32, body []byte) []byte {
	t.Helper()
	if compression == compressionGZIP {
		var compressed bytes.Buffer
		writer := gzip.NewWriter(&compressed)
		if _, err := writer.Write(body); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		body = compressed.Bytes()
	}
	extra := 0
	if flags&flagPositiveSequence != 0 {
		extra = 4
	}
	frame := make([]byte, 8+extra+len(body))
	copy(frame[:4], []byte{0x11, messageFullServer<<4 | flags, serializationJSON<<4 | compression, 0})
	offset := 4
	if extra != 0 {
		binary.BigEndian.PutUint32(frame[offset:offset+4], uint32(sequence))
		offset += 4
	}
	binary.BigEndian.PutUint32(frame[offset:offset+4], uint32(len(body)))
	copy(frame[offset+4:], body)
	return frame
}
