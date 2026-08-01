package selfcheck

import (
	"bytes"
	"encoding/json"
	"errors"
	"reflect"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestExecuteStaticJSONSchemaAndExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := execute([]string{"--mode=static", "--json"}, &stdout, &stderr, testDependencies())
	wantCode := 0
	if runtime.GOOS == "windows" && !cgoAvailable {
		wantCode = 1
	}
	if code != wantCode {
		t.Fatalf("execute() code = %d, want %d; stderr = %q", code, wantCode, stderr.String())
	}
	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("Unmarshal() error = %v; output = %q", err, stdout.String())
	}
	if report.SchemaVersion != SchemaVersion || report.Mode != ModeStatic || report.Timestamp != "2026-08-01T12:00:00Z" {
		t.Fatalf("report header = %+v", report)
	}
	if len(report.Checks) != 5 {
		t.Fatalf("checks = %d, want 5", len(report.Checks))
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestExecuteRejectsArgumentsWithoutEchoingThem(t *testing.T) {
	for _, args := range [][]string{
		{"--mode=secret-value"},
		{"--duration=0s"},
		{"--timeout=61s"},
		{"--unknown=/private/device/path"},
		{"unexpected-secret"},
	} {
		var stdout, stderr bytes.Buffer
		if code := execute(args, &stdout, &stderr, testDependencies()); code != 2 {
			t.Errorf("execute(%q) code = %d, want 2", args, code)
		}
		if stdout.Len() != 0 {
			t.Errorf("execute(%q) stdout = %q", args, stdout.String())
		}
		for _, canary := range []string{"secret-value", "/private/device/path", "unexpected-secret"} {
			if strings.Contains(stderr.String(), canary) {
				t.Errorf("stderr leaked %q: %q", canary, stderr.String())
			}
		}
	}
}

func TestExitCode(t *testing.T) {
	for name, test := range map[string]struct {
		checks []Check
		want   int
	}{
		"pass":    {[]Check{{Status: StatusPass}}, 0},
		"manual":  {[]Check{{Status: StatusManual}, {Status: StatusSkipped}}, 0},
		"failure": {[]Check{{Status: StatusManual}, {Status: StatusFail}}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if got := ExitCode(Report{Checks: test.checks}); got != test.want {
				t.Fatalf("ExitCode() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestStaticChecksFixedContractsAndController(t *testing.T) {
	report := run(Options{Mode: ModeStatic}, testDependencies())
	checks := checksByName(report.Checks)
	for _, name := range []string{"audio_contract", "session_policy", "session_controller"} {
		if checks[name].Status != StatusPass || checks[name].Code != CodeOK {
			t.Fatalf("%s = %+v", name, checks[name])
		}
	}
	if got := *checks["audio_contract"].Metrics.SampleRate; got != 16_000 {
		t.Fatalf("SampleRate = %d", got)
	}
	if got := *checks["session_policy"].Metrics.MinimumSpeechMS; got != 500 {
		t.Fatalf("MinimumSpeechMS = %d", got)
	}
	if got := *checks["session_policy"].Metrics.MaximumSessionMS; got != 60_000 {
		t.Fatalf("MaximumSessionMS = %d", got)
	}
	if got := *checks["session_controller"].Metrics.StateCount; got != 6 {
		t.Fatalf("StateCount = %d", got)
	}
}

func TestAudioPassCountsOnlyBoundedMetricsAndCleansUp(t *testing.T) {
	capture := newFakeAudio()
	capture.pcm <- []byte{1, 2, 3, 4}
	capture.levels <- 0.25
	deps := testDependencies()
	deps.newAudio = func() (audioCapture, error) { return capture, nil }
	report := run(Options{Mode: ModeAudio, Duration: 5 * time.Millisecond}, deps)
	check := report.Checks[0]
	if check.Status != StatusPass || check.Code != CodeOK {
		t.Fatalf("audio check = %+v", check)
	}
	if *check.Metrics.ReceivedBytes != 4 || *check.Metrics.Frames != 1 || *check.Metrics.LevelEvents != 1 {
		t.Fatalf("metrics = %+v", check.Metrics)
	}
	if capture.stops.Load() != 1 || capture.closes.Load() != 1 {
		t.Fatalf("cleanup stops=%d closes=%d", capture.stops.Load(), capture.closes.Load())
	}
}

func TestAudioFailureTimeoutAndCleanup(t *testing.T) {
	t.Run("raw initialization error", func(t *testing.T) {
		deps := testDependencies()
		deps.newAudio = func() (audioCapture, error) {
			return nil, errors.New("device SecretMic at C:\\private\\capture failed")
		}
		report := run(Options{Mode: ModeAudio, Duration: time.Millisecond}, deps)
		assertCheck(t, report.Checks[0], StatusFail, CodeAudioUnavailable)
		assertNoCanary(t, report, "SecretMic", `C:\private\capture`)
	})
	t.Run("no frames", func(t *testing.T) {
		capture := newFakeAudio()
		deps := testDependencies()
		deps.newAudio = func() (audioCapture, error) { return capture, nil }
		report := run(Options{Mode: ModeAudio, Duration: time.Millisecond}, deps)
		assertCheck(t, report.Checks[0], StatusFail, CodeAudioTimeout)
		if capture.stops.Load() != 1 || capture.closes.Load() != 1 {
			t.Fatal("timeout did not stop and close capture")
		}
	})
	t.Run("cleanup failure", func(t *testing.T) {
		capture := newFakeAudio()
		capture.pcm <- []byte{0, 0}
		capture.closeErr = errors.New("secret cleanup detail")
		deps := testDependencies()
		deps.newAudio = func() (audioCapture, error) { return capture, nil }
		report := run(Options{Mode: ModeAudio, Duration: time.Millisecond}, deps)
		assertCheck(t, report.Checks[0], StatusFail, CodeAudioCleanupFailed)
		assertNoCanary(t, report, "secret cleanup detail")
	})
}

func TestExecuteAudioStartFailureDoesNotLeakAndCleansUp(t *testing.T) {
	const canary = `SecretMic at C:\private\capture failed`
	capture := newFakeAudio()
	capture.startErr = errors.New(canary)
	deps := testDependencies()
	deps.newAudio = func() (audioCapture, error) { return capture, nil }

	var stdout, stderr bytes.Buffer
	if code := execute([]string{"--mode=audio", "--duration=1ms", "--json"}, &stdout, &stderr, deps); code != 1 {
		t.Fatalf("execute() code = %d, want 1; stderr = %q", code, stderr.String())
	}
	var report Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("Unmarshal() error = %v; output = %q", err, stdout.String())
	}
	assertCheck(t, report.Checks[0], StatusFail, CodeAudioStartFailed)
	assertNoCanary(t, report, canary)
	if strings.Contains(stdout.String(), canary) {
		t.Fatalf("JSON output leaked start error: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if capture.stops.Load() != 1 || capture.closes.Load() != 1 {
		t.Fatalf("cleanup stops=%d closes=%d, want 1/1", capture.stops.Load(), capture.closes.Load())
	}
}

func TestExecuteTextAudioFailureUsesFixedCheckFields(t *testing.T) {
	const canary = "sensitive driver error from SecretMic"
	capture := newFakeAudio()
	capture.startErr = errors.New(canary)
	deps := testDependencies()
	deps.newAudio = func() (audioCapture, error) { return capture, nil }

	var stdout, stderr bytes.Buffer
	if code := execute([]string{"--mode=audio", "--duration=1ms"}, &stdout, &stderr, deps); code != 1 {
		t.Fatalf("execute() code = %d, want 1; stderr = %q", code, stderr.String())
	}
	lines := strings.Split(strings.TrimSpace(stdout.String()), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdout lines = %q, want two headers and one check", lines)
	}
	if lines[2] != "- audio_capture: fail (audio_start_failed)" {
		t.Fatalf("check line = %q", lines[2])
	}
	if strings.Contains(stdout.String(), canary) {
		t.Fatalf("text output leaked start error: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestExecuteRejectsInvalidDurationWithoutEchoingValue(t *testing.T) {
	for _, value := range []string{"abc", "abc-sensitive-canary"} {
		t.Run(value, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := execute([]string{"--duration=" + value}, &stdout, &stderr, testDependencies()); code != 2 {
				t.Fatalf("execute() code = %d, want 2", code)
			}
			if stdout.Len() != 0 {
				t.Fatalf("stdout = %q, want empty", stdout.String())
			}
			if got := stderr.String(); got != "VoxInk self-check: invalid arguments\n" {
				t.Fatalf("stderr = %q, want fixed invalid-arguments message", got)
			}
			if strings.Contains(stderr.String(), value) {
				t.Fatalf("stderr leaked invalid duration %q: %q", value, stderr.String())
			}
		})
	}
}

func TestInteractiveToggleTimeoutAndManualConfirmation(t *testing.T) {
	t.Run("toggle", func(t *testing.T) {
		overlay := newFakeOverlay()
		overlay.toggles <- struct{}{}
		deps := testDependencies()
		deps.newInteractive = func() interactiveOverlay { return overlay }
		report := run(Options{Mode: ModeInteractive, Timeout: time.Second}, deps)
		assertCheck(t, report.Checks[0], StatusPass, CodeOK)
		assertCheck(t, report.Checks[1], StatusManual, CodeFocusConfirmationRequired)
		if overlay.notice != selfCheckNotice || !*report.Checks[0].Metrics.ToggleReceived {
			t.Fatalf("notice/toggle = %q/%v", overlay.notice, report.Checks[0].Metrics.ToggleReceived)
		}
	})
	t.Run("timeout", func(t *testing.T) {
		overlay := newFakeOverlay()
		deps := testDependencies()
		deps.newInteractive = func() interactiveOverlay { return overlay }
		report := run(Options{Mode: ModeInteractive, Timeout: time.Millisecond}, deps)
		assertCheck(t, report.Checks[0], StatusFail, CodeToggleTimeout)
		assertCheck(t, report.Checks[1], StatusManual, CodeFocusConfirmationRequired)
		if overlay.closes.Load() != 1 {
			t.Fatal("timed out overlay was not closed")
		}
	})
}

func TestReportHasNoFreePayloadContainers(t *testing.T) {
	metricsType := reflect.TypeFor[Metrics]()
	for index := range metricsType.NumField() {
		field := metricsType.Field(index)
		if field.Type.Kind() != reflect.Pointer {
			t.Fatalf("Metrics.%s is %s, want pointer to closed scalar", field.Name, field.Type)
		}
		kind := field.Type.Elem().Kind()
		if kind != reflect.Int64 && kind != reflect.Bool && field.Type.Elem() != reflect.TypeFor[MetricEnum]() {
			t.Fatalf("Metrics.%s exposes unsupported payload type %s", field.Name, field.Type)
		}
	}
	reportType := reflect.TypeFor[Report]()
	for index := range reportType.NumField() {
		field := reportType.Field(index)
		if field.Type.Kind() == reflect.Map || field.Type.Kind() == reflect.Interface {
			t.Fatalf("Report.%s exposes free payload type %s", field.Name, field.Type)
		}
	}
}

func testDependencies() dependencies {
	fixed := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	return dependencies{
		now: func() time.Time { return fixed },
		newAudio: func() (audioCapture, error) {
			panic("unexpected audio probe")
		},
		newInteractive: func() interactiveOverlay {
			panic("unexpected interactive probe")
		},
	}
}

type fakeAudio struct {
	pcm      chan []byte
	levels   chan float64
	errors   chan error
	stops    atomic.Int64
	closes   atomic.Int64
	startErr error
	stopErr  error
	closeErr error
}

func newFakeAudio() *fakeAudio {
	return &fakeAudio{pcm: make(chan []byte, 4), levels: make(chan float64, 4), errors: make(chan error, 4)}
}
func (f *fakeAudio) Format() (audioFormat, bool) {
	return audioFormat{Format: MetricPCM16LE, Channels: 1, SampleRate: 16_000}, true
}
func (f *fakeAudio) Start() error           { return f.startErr }
func (f *fakeAudio) Stop() error            { f.stops.Add(1); return f.stopErr }
func (f *fakeAudio) Close() error           { f.closes.Add(1); return f.closeErr }
func (f *fakeAudio) PCM() <-chan []byte     { return f.pcm }
func (f *fakeAudio) Levels() <-chan float64 { return f.levels }
func (f *fakeAudio) Errors() <-chan error   { return f.errors }
func (f *fakeAudio) OverflowCount() uint64  { return 0 }

type fakeOverlay struct {
	toggles chan struct{}
	closed  chan struct{}
	closes  atomic.Int64
	notice  string
}

func newFakeOverlay() *fakeOverlay {
	return &fakeOverlay{toggles: make(chan struct{}, 1), closed: make(chan struct{})}
}
func (f *fakeOverlay) Run() error               { <-f.closed; return nil }
func (f *fakeOverlay) Toggles() <-chan struct{} { return f.toggles }
func (f *fakeOverlay) ShowNotice(notice string) { f.notice = notice }
func (f *fakeOverlay) Close() error {
	if f.closes.Add(1) == 1 {
		close(f.closed)
	}
	return nil
}

func checksByName(checks []Check) map[string]Check {
	result := make(map[string]Check, len(checks))
	for _, check := range checks {
		result[check.Name] = check
	}
	return result
}

func assertCheck(t *testing.T, check Check, status Status, code Code) {
	t.Helper()
	if check.Status != status || check.Code != code {
		t.Fatalf("check = %+v, want status=%s code=%s", check, status, code)
	}
}

func assertNoCanary(t *testing.T, report Report, canaries ...string) {
	t.Helper()
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	for _, canary := range canaries {
		if bytes.Contains(encoded, []byte(canary)) {
			t.Fatalf("report leaked %q: %s", canary, encoded)
		}
	}
}
