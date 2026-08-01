// Package mimo implements the MiMo mimo-v2.5-asr batch transcriber.
package mimo

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/tossp/voxink/internal/audio"
)

const (
	// DefaultEndpoint is the built-in MiMo HTTPS transcription endpoint.
	DefaultEndpoint      = "https://api.xiaomimimo.com/v1/chat/completions"
	modelName            = "mimo-v2.5-asr"
	maxEncodedAudioBytes = 10 * 1024 * 1024
	maxResponseBytes     = 1024 * 1024
	dataURLPrefix        = "data:audio/wav;base64,"
)

// AuthMode selects one mutually exclusive MiMo authentication header.
type AuthMode string

const (
	// AuthAPIKey sends the official api-key header.
	AuthAPIKey AuthMode = "api-key"
	// AuthBearer sends the official Authorization Bearer header.
	AuthBearer AuthMode = "bearer"
)

// Language is a supported mimo-v2.5-asr language selector.
type Language string

const (
	// LanguageAuto asks MiMo to detect Chinese or English.
	LanguageAuto Language = "auto"
	// LanguageChinese selects Chinese.
	LanguageChinese Language = "zh"
	// LanguageEnglish selects English.
	LanguageEnglish Language = "en"
)

// Config configures the MiMo completed-segment transcriber.
type Config struct {
	Endpoint   string
	HTTPClient *http.Client
	AuthMode   AuthMode
	APIKey     string
	Language   Language
}

// Transcriber submits one complete in-memory PCM segment to MiMo.
type Transcriber struct {
	endpoint string
	client   *http.Client
	authMode AuthMode
	apiKey   string
	language Language
}

// NewTranscriber validates MiMo endpoint, authentication mode, and language.
func NewTranscriber(config Config) (*Transcriber, error) {
	if config.Endpoint == "" {
		config.Endpoint = DefaultEndpoint
	}
	if config.HTTPClient == nil {
		config.HTTPClient = http.DefaultClient
	}
	if config.APIKey == "" {
		return nil, fmt.Errorf("MiMo API key must not be empty")
	}
	if config.AuthMode != AuthAPIKey && config.AuthMode != AuthBearer {
		return nil, fmt.Errorf("unsupported MiMo authentication mode %q", config.AuthMode)
	}
	if config.Language != LanguageAuto && config.Language != LanguageChinese && config.Language != LanguageEnglish {
		return nil, fmt.Errorf("unsupported MiMo ASR language %q", config.Language)
	}
	return &Transcriber{
		endpoint: config.Endpoint, client: config.HTTPClient,
		authMode: config.AuthMode, apiKey: config.APIKey, language: config.Language,
	}, nil
}

type chatRequest struct {
	Model      string     `json:"model"`
	Messages   []message  `json:"messages"`
	ASROptions asrOptions `json:"asr_options"`
	Stream     bool       `json:"stream"`
}

type message struct {
	Role    string        `json:"role"`
	Content []contentPart `json:"content"`
}

type contentPart struct {
	Type       string     `json:"type"`
	InputAudio inputAudio `json:"input_audio"`
}

type inputAudio struct {
	Data string `json:"data"`
}

type asrOptions struct {
	Language Language `json:"language"`
}

type chatResponse struct {
	Choices []choice `json:"choices"`
}

type choice struct {
	Message responseMessage `json:"message"`
}

type responseMessage struct {
	Content string `json:"content"`
}

// HTTPError is a sanitized non-success MiMo HTTP response.
type HTTPError struct {
	StatusCode int
	category   string
	code       string
}

func (e *HTTPError) Error() string {
	if e.code == "" {
		return fmt.Sprintf("MiMo HTTP status %d, category %s", e.StatusCode, e.category)
	}
	return fmt.Sprintf("MiMo HTTP status %d, category %s, code %s", e.StatusCode, e.category, e.code)
}

// TransportError reports a MiMo request transport failure without exposing it
// through the Provider smoke report.
type TransportError struct{ Err error }

func (e *TransportError) Error() string { return "perform MiMo request: " + e.Err.Error() }
func (e *TransportError) Unwrap() error { return e.Err }

// ResponseError reports a structurally invalid or over-limit MiMo response.
type ResponseError struct{ reason string }

func (e *ResponseError) Error() string { return "invalid MiMo response: " + e.reason }

type errorResponse struct {
	Error struct {
		Type string `json:"type"`
		Code any    `json:"code"`
	} `json:"error"`
}

// Transcribe encodes fixed PCM16 LE as WAV and submits one non-streaming request.
func (t *Transcriber) Transcribe(ctx context.Context, pcm []byte) (string, error) {
	wav, err := audio.EncodeWAVPCM16(pcm)
	if err != nil {
		return "", fmt.Errorf("encode MiMo WAV: %w", err)
	}
	return t.TranscribeWAV(ctx, wav)
}

// TranscribeWAV submits one already validated, complete in-memory WAV file.
func (t *Transcriber) TranscribeWAV(ctx context.Context, wav []byte) (string, error) {
	dataURL := dataURLPrefix + base64.StdEncoding.EncodeToString(wav)
	if len(dataURL) > maxEncodedAudioBytes {
		return "", fmt.Errorf("MiMo encoded audio exceeds 10 MB limit")
	}

	payload := chatRequest{
		Model: modelName,
		Messages: []message{{Role: "user", Content: []contentPart{{
			Type: "input_audio", InputAudio: inputAudio{Data: dataURL},
		}}}},
		ASROptions: asrOptions{Language: t.language},
		Stream:     false,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("encode MiMo request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("create MiMo request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if t.authMode == AuthAPIKey {
		req.Header.Set("api-key", t.apiKey)
	} else {
		req.Header.Set("Authorization", "Bearer "+t.apiKey)
	}

	response, err := t.client.Do(req)
	if err != nil {
		return "", &TransportError{Err: err}
	}
	defer response.Body.Close()
	responseBody, err := readBounded(response.Body, maxResponseBytes)
	if err != nil {
		return "", &ResponseError{reason: "body exceeds limit or read failed"}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return "", parseHTTPError(response.StatusCode, responseBody)
	}

	var decoded chatResponse
	if err := json.Unmarshal(responseBody, &decoded); err != nil {
		return "", &ResponseError{reason: "JSON decoding failed"}
	}
	if len(decoded.Choices) == 0 {
		return "", &ResponseError{reason: "response has no choices"}
	}
	return decoded.Choices[0].Message.Content, nil
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response body exceeds %d bytes", limit)
	}
	return body, nil
}

func parseHTTPError(status int, body []byte) error {
	category := "unknown"
	code := ""
	var decoded errorResponse
	if json.Unmarshal(body, &decoded) == nil {
		if decoded.Error.Type != "" {
			category = safeToken(decoded.Error.Type)
		}
		if decoded.Error.Code != nil {
			code = safeToken(fmt.Sprint(decoded.Error.Code))
		}
	}
	return &HTTPError{StatusCode: status, category: category, code: code}
}

func safeToken(value string) string {
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("._-", r) {
			return r
		}
		return '_'
	}, value)
	if len(value) > 64 {
		value = value[:64]
	}
	return value
}
