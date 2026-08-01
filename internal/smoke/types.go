// Package smoke implements explicit, redacted Provider connectivity checks.
package smoke

import "time"

const (
	// SchemaVersion is the stable Provider smoke report schema.
	SchemaVersion = "1"
	// MaximumAudioBytes is the local WAV file cap. It accommodates the fixed
	// 60-second PCM contract while bounding all in-memory copies.
	MaximumAudioBytes int64 = 2 * 1024 * 1024
)

// Provider is a fixed smoke target.
type Provider string

const (
	ProviderVolc Provider = "volc"
	ProviderMiMo Provider = "mimo"
)

// Model is a fixed model identifier.
type Model string

const (
	ModelVolc Model = "volcengine-v3"
	ModelMiMo Model = "mimo-v2.5-asr"
)

// Status is a fixed smoke outcome.
type Status string

const (
	StatusPass Status = "pass"
	StatusFail Status = "fail"
)

// Code is a fixed, non-sensitive result category.
type Code string

const (
	CodeOK                  Code = "ok"
	CodeInvalidArguments    Code = "invalid_arguments"
	CodeConfigMissing       Code = "config_missing"
	CodeAudioUnavailable    Code = "audio_unavailable"
	CodeAudioTooLarge       Code = "audio_too_large"
	CodeAudioInvalid        Code = "audio_invalid"
	CodeTimeout             Code = "timeout"
	CodeUnauthorized        Code = "unauthorized"
	CodeRateLimited         Code = "rate_limited"
	CodeProviderUnavailable Code = "provider_unavailable"
	CodeProtocolFailed      Code = "protocol_failed"
	CodeResponseInvalid     Code = "response_invalid"
	CodeInternalFailure     Code = "internal_failure"
)

// HTTPStatusClass is a closed HTTP status class metric.
type HTTPStatusClass string

const (
	HTTPStatus4xx HTTPStatusClass = "4xx"
	HTTPStatus5xx HTTPStatusClass = "5xx"
)

// Metrics is the closed allowlist of Provider smoke measurements.
type Metrics struct {
	AudioDurationMS  *int64           `json:",omitzero"`
	AudioBytes       *int64           `json:",omitzero"`
	LatencyMS        *int64           `json:",omitzero"`
	EventCount       *int64           `json:",omitzero"`
	FinalReceived    *bool            `json:",omitzero"`
	HTTPStatusClass  *HTTPStatusClass `json:",omitzero"`
	ProtocolTerminal *bool            `json:",omitzero"`
}

// Report is the stable machine-readable Provider smoke document.
type Report struct {
	SchemaVersion string
	Timestamp     string
	Provider      Provider
	Model         Model
	Status        Status
	Code          Code
	Metrics       Metrics
}

type options struct {
	provider Provider
	audio    string
	timeout  time.Duration
	json     bool
}

type providerResult struct {
	eventCount       int64
	finalReceived    bool
	protocolTerminal bool
}

func integer(value int64) *int64 { return &value }
func boolean(value bool) *bool   { return &value }
