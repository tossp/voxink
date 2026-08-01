package selfcheck

import (
	"errors"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/tossp/voxink/internal/audio"
	"github.com/tossp/voxink/internal/domain"
	platformwindows "github.com/tossp/voxink/internal/platform/windows"
	"github.com/tossp/voxink/internal/session"
)

const selfCheckNotice = "VoxInk self-check: press Ctrl+Shift+Space once, then confirm the overlay did not take focus."

type audioCapture interface {
	Format() (audioFormat, bool)
	Start() error
	Stop() error
	Close() error
	PCM() <-chan []byte
	Levels() <-chan float64
	Errors() <-chan error
	OverflowCount() uint64
}

type audioFormat struct {
	Format     MetricEnum
	Channels   uint32
	SampleRate uint32
}

type interactiveOverlay interface {
	Run() error
	Close() error
	Toggles() <-chan struct{}
	ShowNotice(string)
}

type dependencies struct {
	now            func() time.Time
	newAudio       func() (audioCapture, error)
	newInteractive func() interactiveOverlay
}

// Run executes one self-check mode without reading provider configuration.
func Run(options Options) Report {
	return run(options, defaultDependencies())
}

func run(options Options, deps dependencies) Report {
	report := Report{
		SchemaVersion: SchemaVersion,
		Timestamp:     deps.now().UTC().Format(time.RFC3339),
		Mode:          options.Mode,
		App:           Build{Version: buildVersion()},
		Runtime: Runtime{
			GOOS: runtime.GOOS, GOARCH: runtime.GOARCH,
			GoVersion: runtime.Version(), CGOAvailable: cgoAvailable,
		},
	}
	switch options.Mode {
	case ModeAudio:
		report.Checks = runAudio(options.Duration, deps)
	case ModeInteractive:
		report.Checks = runInteractive(options.Timeout, deps)
	default:
		report.Checks = runStatic()
	}
	return report
}

// ExitCode returns 1 when any check failed and 0 otherwise.
func ExitCode(report Report) int {
	for _, check := range report.Checks {
		if check.Status == StatusFail {
			return 1
		}
	}
	return 0
}

func buildVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return "dev"
	}
	return info.Main.Version
}

func runStatic() []Check {
	checks := []Check{{Name: "build_runtime", Status: StatusPass, Code: CodeOK}}
	contract := Metrics{
		SampleRate: metric(audio.SampleRate), Channels: metric(audio.ChannelCount),
		BytesPerSample: metric(audio.BytesPerSample), Format: metricEnum(MetricPCM16LE),
	}
	contractStatus, contractCode := StatusPass, CodeOK
	if audio.SampleRate != 16_000 || audio.ChannelCount != 1 || audio.BytesPerSample != 2 {
		contractStatus, contractCode = StatusFail, CodeContractMismatch
	}
	checks = append(checks, Check{Name: "audio_contract", Status: contractStatus, Code: contractCode, Metrics: contract})

	policy := Metrics{
		MinimumSpeechMS:  metric(audio.MinimumSpeechDuration.Milliseconds()),
		SilenceSplitMS:   metric(audio.SilenceSplitDuration.Milliseconds()),
		MaximumSegmentMS: metric(audio.MaximumContinuousSpeechDuration.Milliseconds()),
		MaximumSessionMS: metric(audio.MaximumSessionDuration.Milliseconds()),
	}
	policyStatus, policyCode := StatusPass, CodeOK
	if audio.MinimumSpeechDuration != 500*time.Millisecond || audio.SilenceSplitDuration != 600*time.Millisecond ||
		audio.MaximumContinuousSpeechDuration != 15*time.Second || audio.MaximumSessionDuration != 60*time.Second {
		policyStatus, policyCode = StatusFail, CodePolicyMismatch
	}
	checks = append(checks, Check{Name: "session_policy", Status: policyStatus, Code: policyCode, Metrics: policy})

	controllerStatus, controllerCode := StatusPass, CodeOK
	if !controllerRunnable() {
		controllerStatus, controllerCode = StatusFail, CodeControllerFailed
	}
	checks = append(checks, Check{
		Name: "session_controller", Status: controllerStatus, Code: controllerCode,
		Metrics: Metrics{StateCount: metric(6)},
	})

	platform := Check{Name: "windows_platform", Status: StatusPass, Code: CodeOK}
	if runtime.GOOS != "windows" {
		platform.Status, platform.Code = StatusSkipped, CodePlatformUnsupported
	} else if !cgoAvailable {
		platform.Status, platform.Code = StatusFail, CodeCGODisabled
	}
	return append(checks, platform)
}

func controllerRunnable() bool {
	if domain.SessionIdle != 0 || domain.SessionCapturing != 1 || domain.SessionTranscribing != 2 ||
		domain.SessionDelivering != 3 || domain.SessionStopped != 4 || domain.SessionFailed != 5 {
		return false
	}
	normal := session.NewController()
	if normal.State() != domain.SessionIdle || normal.Start("self-check") != nil ||
		!normal.Handle(domain.Event{SessionID: "self-check", Kind: domain.EventStopped}) ||
		!normal.Handle(domain.Event{SessionID: "self-check", Kind: domain.EventFinal}) ||
		!normal.CompleteDelivery("self-check") || normal.State() != domain.SessionStopped {
		return false
	}
	failed := session.NewController()
	return failed.Start("self-check-fail") == nil &&
		failed.Handle(domain.Event{SessionID: "self-check-fail", Kind: domain.EventError}) &&
		failed.State() == domain.SessionFailed
}

func platformCode(err error, fallback Code) Code {
	switch {
	case errors.Is(err, platformwindows.ErrUnsupportedPlatform):
		return CodePlatformUnsupported
	case errors.Is(err, platformwindows.ErrCGODisabled):
		return CodeCGODisabled
	case errors.Is(err, platformwindows.ErrHotkeyUnavailable):
		return CodeHotkeyUnavailable
	default:
		return fallback
	}
}
