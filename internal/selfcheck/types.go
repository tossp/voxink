// Package selfcheck implements the provider-free VoxInk self-check report.
package selfcheck

import "time"

const (
	// SchemaVersion is the stable machine-readable report schema.
	SchemaVersion  = "1"
	maxMetricValue = int64(1_000_000_000_000)
)

// Mode selects one bounded self-check layer.
type Mode string

const (
	ModeStatic      Mode = "static"
	ModeAudio       Mode = "audio"
	ModeInteractive Mode = "interactive"
)

// Status is a fixed check outcome.
type Status string

const (
	StatusPass    Status = "pass"
	StatusFail    Status = "fail"
	StatusManual  Status = "manual"
	StatusSkipped Status = "skipped"
)

// Code is a fixed, non-sensitive result category.
type Code string

const (
	CodeOK                        Code = "ok"
	CodePlatformUnsupported       Code = "platform_unsupported"
	CodeCGODisabled               Code = "cgo_disabled"
	CodeContractMismatch          Code = "contract_mismatch"
	CodePolicyMismatch            Code = "policy_mismatch"
	CodeControllerFailed          Code = "controller_failed"
	CodeAudioUnavailable          Code = "audio_unavailable"
	CodeAudioFormatMismatch       Code = "audio_format_mismatch"
	CodeAudioStartFailed          Code = "audio_start_failed"
	CodeAudioRuntimeFailed        Code = "audio_runtime_failed"
	CodeAudioTimeout              Code = "audio_timeout"
	CodeAudioCleanupFailed        Code = "audio_cleanup_failed"
	CodeHotkeyUnavailable         Code = "hotkey_unavailable"
	CodeOverlayUnavailable        Code = "overlay_unavailable"
	CodeToggleTimeout             Code = "toggle_timeout"
	CodeFocusConfirmationRequired Code = "focus_confirmation_required"
	CodeFocusNotTested            Code = "focus_not_tested"
)

// MetricEnum is a fixed-enum metric value, never free-form text.
type MetricEnum string

const (
	MetricPCM16LE MetricEnum = "pcm_s16le"
	metricUnknown MetricEnum = "unknown"
)

// Metrics is the closed allowlist of report measurements.
type Metrics struct {
	DurationMS       *int64      `json:",omitempty"`
	ReceivedBytes    *int64      `json:",omitempty"`
	Frames           *int64      `json:",omitempty"`
	LevelEvents      *int64      `json:",omitempty"`
	OverflowCount    *int64      `json:",omitempty"`
	SampleRate       *int64      `json:",omitempty"`
	Channels         *int64      `json:",omitempty"`
	BytesPerSample   *int64      `json:",omitempty"`
	MinimumSpeechMS  *int64      `json:",omitempty"`
	SilenceSplitMS   *int64      `json:",omitempty"`
	MaximumSegmentMS *int64      `json:",omitempty"`
	MaximumSessionMS *int64      `json:",omitempty"`
	StateCount       *int64      `json:",omitempty"`
	ToggleReceived   *bool       `json:",omitempty"`
	Format           *MetricEnum `json:",omitempty"`
}

// Check is one fixed-name self-check result.
type Check struct {
	Name    string
	Status  Status
	Code    Code
	Metrics Metrics
}

// Build identifies the executable without exposing paths or build settings.
type Build struct {
	Version string
}

// Runtime describes the safe build/runtime platform fields.
type Runtime struct {
	GOOS         string
	GOARCH       string
	GoVersion    string
	CGOAvailable bool
}

// Report is the stable, machine-readable self-check document.
type Report struct {
	SchemaVersion string
	Timestamp     string
	Mode          Mode
	App           Build
	Runtime       Runtime
	Checks        []Check
}

// Options contains validated runner settings.
type Options struct {
	Mode     Mode
	Duration time.Duration
	Timeout  time.Duration
}

func metric(value int64) *int64 {
	value = min(max(value, 0), maxMetricValue)
	return &value
}

func boolean(value bool) *bool { return &value }

func metricEnum(value MetricEnum) *MetricEnum { return &value }
