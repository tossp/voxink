package mimo

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tossp/voxink/internal/asr"
)

func TestTranscribeRequestAndAuthentication(t *testing.T) {
	tests := []struct {
		name       string
		authMode   AuthMode
		language   Language
		wantHeader string
		wantValue  string
	}{
		{name: "api key auto", authMode: AuthAPIKey, language: LanguageAuto, wantHeader: "api-key", wantValue: "local-api-key"},
		{name: "bearer Chinese", authMode: AuthBearer, language: LanguageChinese, wantHeader: "Authorization", wantValue: "Bearer local-api-key"},
		{name: "api key English", authMode: AuthAPIKey, language: LanguageEnglish, wantHeader: "api-key", wantValue: "local-api-key"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if got := r.Header.Get(tt.wantHeader); got != tt.wantValue {
					t.Errorf("%s = %q, want %q", tt.wantHeader, got, tt.wantValue)
				}
				otherHeader := "Authorization"
				if tt.wantHeader == otherHeader {
					otherHeader = "api-key"
				}
				if got := r.Header.Get(otherHeader); got != "" {
					t.Errorf("mutually exclusive header %s = %q", otherHeader, got)
				}
				body, err := io.ReadAll(r.Body)
				if err != nil {
					t.Errorf("read request: %v", err)
					return
				}
				var raw map[string]json.RawMessage
				if err := json.Unmarshal(body, &raw); err != nil {
					t.Errorf("decode raw request: %v", err)
					return
				}
				if string(raw["stream"]) != "false" {
					t.Errorf("stream = %s, want false", raw["stream"])
				}
				var request chatRequest
				if err := json.Unmarshal(body, &request); err != nil {
					t.Errorf("decode request: %v", err)
					return
				}
				if request.Model != modelName || request.ASROptions.Language != tt.language {
					t.Errorf("model/language = %q/%q", request.Model, request.ASROptions.Language)
				}
				if len(request.Messages) != 1 || len(request.Messages[0].Content) != 1 ||
					request.Messages[0].Content[0].Type != "input_audio" {
					t.Errorf("unexpected messages: %+v", request.Messages)
					return
				}
				assertWAVDataURL(t, request.Messages[0].Content[0].InputAudio.Data, []byte{1, 2, 3, 4})
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"local result"}}]}`)
			}))
			defer server.Close()

			transcriber := newTestTranscriber(t, server.URL+"/v1/chat/completions", server.Client(), tt.authMode, tt.language)
			text, err := transcriber.Transcribe(context.Background(), []byte{1, 2, 3, 4})
			if err != nil {
				t.Fatalf("Transcribe() error = %v", err)
			}
			if text != "local result" {
				t.Fatalf("Transcribe() = %q", text)
			}
		})
	}
}

func TestTranscribeHTTPErrorIsSanitized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":{"type":"authentication_error","code":"denied","message":"local-api-key and full request body"}}`)
	}))
	defer server.Close()
	transcriber := newTestTranscriber(t, server.URL, server.Client(), AuthAPIKey, LanguageAuto)
	_, err := transcriber.Transcribe(context.Background(), []byte{1, 2})
	if err == nil {
		t.Fatal("Transcribe() error = nil")
	}
	message := err.Error()
	for _, want := range []string{"401", "authentication_error", "denied"} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not contain %q", message, want)
		}
	}
	for _, forbidden := range []string{"local-api-key", "full request body"} {
		if strings.Contains(message, forbidden) {
			t.Fatalf("error contains sensitive response text %q", forbidden)
		}
	}
}

func TestTranscribeRejectsEmptyChoices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"choices":[]}`)
	}))
	defer server.Close()
	transcriber := newTestTranscriber(t, server.URL, server.Client(), AuthBearer, LanguageChinese)
	_, err := transcriber.Transcribe(context.Background(), []byte{1, 2})
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Fatalf("Transcribe() error = %v", err)
	}
}

func TestTranscribeRejectsEncodedAudioOver10MBLocally(t *testing.T) {
	var calls atomic.Int32
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("network must not be called")
	})}
	transcriber := newTestTranscriber(t, "https://local.invalid", client, AuthAPIKey, LanguageAuto)
	_, err := transcriber.Transcribe(context.Background(), make([]byte, 8*1024*1024))
	if err == nil || !strings.Contains(err.Error(), "10 MB") {
		t.Fatalf("Transcribe() error = %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("HTTP calls = %d, want 0", calls.Load())
	}
}

func TestTranscribeContextCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(started)
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer server.Close()
	transcriber := newTestTranscriber(t, server.URL, server.Client(), AuthAPIKey, LanguageEnglish)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := transcriber.Transcribe(ctx, []byte{1, 2})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Transcribe() error = %v, want canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Transcribe() did not return after cancellation")
	}
	close(release)
}

func TestTranscribeBoundsResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, maxResponseBytes+1))
	}))
	defer server.Close()
	transcriber := newTestTranscriber(t, server.URL, server.Client(), AuthAPIKey, LanguageAuto)
	_, err := transcriber.Transcribe(context.Background(), []byte{1, 2})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("Transcribe() error = %v", err)
	}
}

func TestNewTranscriberValidatesModeLanguageAndKey(t *testing.T) {
	tests := []Config{
		{AuthMode: AuthAPIKey, Language: LanguageAuto},
		{AuthMode: "other", APIKey: "local", Language: LanguageAuto},
		{AuthMode: AuthBearer, APIKey: "local", Language: "fr"},
	}
	for index, config := range tests {
		if _, err := NewTranscriber(config); err == nil {
			t.Fatalf("case %d: NewTranscriber() error = nil", index)
		}
	}
}

func assertWAVDataURL(t *testing.T, dataURL string, wantPCM []byte) {
	t.Helper()
	if !strings.HasPrefix(dataURL, dataURLPrefix) {
		t.Fatalf("audio data lacks WAV data URL prefix")
	}
	wav, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(dataURL, dataURLPrefix))
	if err != nil {
		t.Fatalf("decode Base64 audio: %v", err)
	}
	if len(wav) < 44 || string(wav[:4]) != "RIFF" || string(wav[8:12]) != "WAVE" {
		t.Fatalf("invalid WAV header")
	}
	if binary.LittleEndian.Uint32(wav[24:28]) != 16_000 ||
		binary.LittleEndian.Uint16(wav[22:24]) != 1 ||
		binary.LittleEndian.Uint16(wav[34:36]) != 16 {
		t.Fatalf("unexpected WAV audio format")
	}
	if got := wav[44:]; string(got) != string(wantPCM) {
		t.Fatalf("WAV PCM = %v, want %v", got, wantPCM)
	}
}

func newTestTranscriber(t *testing.T, endpoint string, client *http.Client, mode AuthMode, language Language) *Transcriber {
	t.Helper()
	transcriber, err := NewTranscriber(Config{
		Endpoint: endpoint, HTTPClient: client, AuthMode: mode,
		APIKey: "local-api-key", Language: language,
	})
	if err != nil {
		t.Fatalf("NewTranscriber() error = %v", err)
	}
	return transcriber
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

var _ asr.SegmentTranscriber = (*Transcriber)(nil)
