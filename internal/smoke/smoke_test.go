package smoke

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/tossp/voxink/internal/asr"
	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
)

func TestExecuteArgumentsAndExitCodesDoNotEchoCanaries(t *testing.T) {
	canary := `/private/audio/secret-canary.wav`
	for _, args := range [][]string{
		nil,
		{"other", "--audio", canary, "--confirm-send"},
		{"volc", "--audio", canary},
		{"mimo", "--confirm-send"},
		{"volc", "--audio", canary, "--confirm-send=false"},
		{"volc", "--audio", canary, "--confirm-send", "--timeout=999s"},
		{"mimo", "--audio", canary, "--confirm-send", "--unknown=secret-canary"},
	} {
		var stdout, stderr bytes.Buffer
		if code := execute(args, &stdout, &stderr, testDependencies()); code != 2 {
			t.Errorf("execute(%q) code = %d, want 2", args, code)
		}
		if stdout.Len() != 0 || stderr.String() != "VoxInk smoke: invalid arguments\n" {
			t.Errorf("execute(%q) stdout/stderr = %q/%q", args, stdout.String(), stderr.String())
		}
		if strings.Contains(stderr.String(), canary) || strings.Contains(stderr.String(), "secret-canary") {
			t.Fatalf("argument error leaked canary: %q", stderr.String())
		}
	}
}

func TestExecutePassJSONIsRedactedAndReleasesAudio(t *testing.T) {
	for _, provider := range []Provider{ProviderVolc, ProviderMiMo} {
		t.Run(string(provider), func(t *testing.T) {
			input := &audioInput{wav: []byte("path-canary secret-body"), pcm: []byte("recognized-text-canary"), durationMS: 25}
			deps := testDependencies()
			deps.readAudio = func(string) (*audioInput, error) { return input, nil }
			deps.provider = func(got Provider, _ func(string) string) (providerRunner, error) {
				if got != provider {
					t.Fatalf("selected Provider = %s, want %s", got, provider)
				}
				return runnerFunc(func(context.Context, *audioInput) (providerResult, error) {
					return providerResult{eventCount: 2, finalReceived: true, protocolTerminal: provider == ProviderVolc}, nil
				}), nil
			}
			var stdout, stderr bytes.Buffer
			args := []string{string(provider), "--audio", "/path-canary.wav", "--confirm-send", "--json"}
			if code := execute(args, &stdout, &stderr, deps); code != 0 {
				t.Fatalf("execute() code = %d; stdout=%q stderr=%q", code, stdout.String(), stderr.String())
			}
			var report Report
			if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
				t.Fatalf("decode report: %v", err)
			}
			if report.Status != StatusPass || report.Code != CodeOK || report.Provider != provider {
				t.Fatalf("report = %+v", report)
			}
			for _, canary := range []string{"path-canary", "secret-body", "recognized-text-canary"} {
				if strings.Contains(stdout.String(), canary) || strings.Contains(stderr.String(), canary) {
					t.Fatalf("output leaked %q: stdout=%q stderr=%q", canary, stdout.String(), stderr.String())
				}
			}
			if input.wav != nil || input.pcm != nil {
				t.Fatal("execute retained audio payload")
			}
		})
	}
}

func TestOnlySelectedProviderIsConstructedWithoutFallback(t *testing.T) {
	var calls atomic.Int32
	deps := testDependencies()
	deps.provider = func(provider Provider, _ func(string) string) (providerRunner, error) {
		calls.Add(1)
		if provider != ProviderMiMo {
			t.Fatalf("provider = %s, want mimo", provider)
		}
		return runnerFunc(func(context.Context, *audioInput) (providerResult, error) {
			return providerResult{}, errors.New("recognized-text-canary secret-body")
		}), nil
	}
	var stdout, stderr bytes.Buffer
	code := execute([]string{"mimo", "--audio", "canary-path", "--confirm-send", "--json"}, &stdout, &stderr, deps)
	if code != 1 || calls.Load() != 1 {
		t.Fatalf("exit/calls = %d/%d", code, calls.Load())
	}
	if strings.Contains(stdout.String()+stderr.String(), "canary") || strings.Contains(stdout.String(), "secret-body") {
		t.Fatalf("failure output leaked raw data: %q / %q", stdout.String(), stderr.String())
	}
}

func TestRealProviderFactoryLoadsOnlySelectedConfiguration(t *testing.T) {
	tests := []struct {
		provider Provider
		values   map[string]string
	}{
		{
			provider: ProviderMiMo,
			values: map[string]string{
				"VOXINK_MIMO_API_KEY":          "mimo-secret-canary",
				"VOXINK_VOLC_READ_LIMIT_BYTES": "invalid-volc-canary",
			},
		},
		{
			provider: ProviderVolc,
			values: map[string]string{
				"VOXINK_VOLC_API_KEY":     "volc-secret-canary",
				"VOXINK_VOLC_RESOURCE_ID": "resource-secret-canary",
				"VOXINK_MIMO_ENDPOINT":    "invalid-mimo-canary",
			},
		},
	}
	for _, tt := range tests {
		t.Run(string(tt.provider), func(t *testing.T) {
			getenv := func(key string) string { return tt.values[key] }
			if _, err := newProviderRunner(tt.provider, getenv); err != nil {
				t.Fatalf("newProviderRunner(%s) error = %v", tt.provider, err)
			}
		})
	}
}

func TestConfigAndAudioFailuresProduceFixedReports(t *testing.T) {
	tests := []struct {
		name        string
		providerErr error
		audioErr    error
		want        Code
	}{
		{name: "config", providerErr: errors.New("credential-secret-canary"), want: CodeConfigMissing},
		{name: "unavailable", audioErr: errAudioUnavailable, want: CodeAudioUnavailable},
		{name: "too large", audioErr: errAudioTooLarge, want: CodeAudioTooLarge},
		{name: "invalid", audioErr: errAudioInvalid, want: CodeAudioInvalid},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDependencies()
			if tt.providerErr != nil {
				deps.provider = func(Provider, func(string) string) (providerRunner, error) { return nil, tt.providerErr }
			}
			if tt.audioErr != nil {
				deps.readAudio = func(string) (*audioInput, error) { return nil, tt.audioErr }
			}
			var stdout, stderr bytes.Buffer
			code := execute([]string{"volc", "--audio", "path-secret-canary", "--confirm-send", "--json"}, &stdout, &stderr, deps)
			if code != 1 {
				t.Fatalf("execute() code = %d, want 1", code)
			}
			var report Report
			if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
				t.Fatal(err)
			}
			if report.Code != tt.want || report.Status != StatusFail {
				t.Fatalf("report = %+v", report)
			}
			for _, canary := range []string{"credential-secret-canary", "path-secret-canary"} {
				if strings.Contains(stdout.String()+stderr.String(), canary) {
					t.Fatalf("failure leaked %q", canary)
				}
			}
		})
	}
}

func TestTextReportContainsOnlyFixedFields(t *testing.T) {
	deps := testDependencies()
	deps.readAudio = func(string) (*audioInput, error) {
		return &audioInput{wav: []byte("wav-secret-canary"), pcm: []byte("recognized-text-canary")}, nil
	}
	var stdout, stderr bytes.Buffer
	if code := execute([]string{"mimo", "--audio", "path-secret-canary", "--confirm-send"}, &stdout, &stderr, deps); code != 0 {
		t.Fatalf("execute() code = %d", code)
	}
	want := "VoxInk Provider smoke 1\nProvider: mimo\nModel: mimo-v2.5-asr\nStatus: pass\nCode: ok\n"
	if stdout.String() != want {
		t.Fatalf("text report = %q, want %q", stdout.String(), want)
	}
	for _, canary := range []string{"wav-secret-canary", "recognized-text-canary", "path-secret-canary"} {
		if strings.Contains(stdout.String()+stderr.String(), canary) {
			t.Fatalf("text output leaked %q", canary)
		}
	}
}

func TestProviderErrorClassificationAndStatusMetrics(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		want       Code
		wantStatus HTTPStatusClass
	}{
		{name: "unauthorized", err: &mimo.HTTPError{StatusCode: 401}, want: CodeUnauthorized, wantStatus: HTTPStatus4xx},
		{name: "rate limited", err: &mimo.HTTPError{StatusCode: 429}, want: CodeRateLimited, wantStatus: HTTPStatus4xx},
		{name: "provider unavailable", err: &volcengine.HandshakeError{StatusCode: 503, Err: errors.New("body-canary")}, want: CodeProviderUnavailable, wantStatus: HTTPStatus5xx},
		{name: "transport unavailable", err: &mimo.TransportError{Err: errors.New("endpoint-canary")}, want: CodeProviderUnavailable},
		{name: "protocol", err: &volcengine.ServerError{Code: 123}, want: CodeProtocolFailed},
		{name: "response invalid", err: &mimo.ResponseError{}, want: CodeResponseInvalid},
		{name: "unknown", err: errors.New("raw-secret-canary"), want: CodeInternalFailure},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := testDependencies()
			deps.provider = func(Provider, func(string) string) (providerRunner, error) {
				return runnerFunc(func(context.Context, *audioInput) (providerResult, error) { return providerResult{}, tt.err }), nil
			}
			report := run(context.Background(), options{provider: ProviderVolc, audio: "ignored", timeout: time.Second}, deps)
			if report.Code != tt.want || report.Status != StatusFail {
				t.Fatalf("report status/code = %s/%s, want fail/%s", report.Status, report.Code, tt.want)
			}
			if tt.wantStatus == "" {
				if report.Metrics.HTTPStatusClass != nil {
					t.Fatalf("HTTPStatusClass = %v, want nil", *report.Metrics.HTTPStatusClass)
				}
			} else if report.Metrics.HTTPStatusClass == nil || *report.Metrics.HTTPStatusClass != tt.wantStatus {
				t.Fatalf("HTTPStatusClass = %v, want %s", report.Metrics.HTTPStatusClass, tt.wantStatus)
			}
			encoded, _ := json.Marshal(report)
			for _, canary := range []string{"body-canary", "endpoint-canary", "raw-secret-canary"} {
				if bytes.Contains(encoded, []byte(canary)) {
					t.Fatalf("report leaked %q: %s", canary, encoded)
				}
			}
		})
	}
}

func TestTimeoutCancelsRunnerAndReturnsTimeout(t *testing.T) {
	deps := testDependencies()
	finished := make(chan struct{})
	deps.provider = func(Provider, func(string) string) (providerRunner, error) {
		return runnerFunc(func(ctx context.Context, _ *audioInput) (providerResult, error) {
			<-ctx.Done()
			close(finished)
			return providerResult{}, ctx.Err()
		}), nil
	}
	report := run(context.Background(), options{provider: ProviderVolc, audio: "ignored", timeout: time.Millisecond}, deps)
	if report.Code != CodeTimeout {
		t.Fatalf("report code = %s, want timeout", report.Code)
	}
	select {
	case <-finished:
	default:
		t.Fatal("runner did not observe cancellation")
	}
}

func TestVolcRunnerFramesTerminalAndClosesWithoutRetainingText(t *testing.T) {
	session := &fakeLiveSession{events: []asr.LiveEvent{{Text: "recognized-text-canary"}, {Text: "secret-body", ProtocolTerminal: true}}}
	runner := volcRunner{client: fakeLiveClient{session: session}}
	input := &audioInput{pcm: make([]byte, volcFrameBytes+2)}
	result, err := runner.Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(session.frames) != 2 || session.finishes != 1 || session.closes != 1 {
		t.Fatalf("frames/finish/close = %d/%d/%d", len(session.frames), session.finishes, session.closes)
	}
	if result.eventCount != 2 || !result.finalReceived || !result.protocolTerminal {
		t.Fatalf("result = %+v", result)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("recognized-text-canary")) || bytes.Contains(encoded, []byte("secret-body")) {
		t.Fatalf("result retained recognized text: %s", encoded)
	}
}

func TestMiMoRunnerUsesAuthorizedWAVAndDropsText(t *testing.T) {
	transcriber := &fakeWAVTranscriber{text: "recognized-text-canary"}
	input := &audioInput{wav: []byte("authorized-wav-canary"), pcm: []byte("different-pcm")}
	result, err := (mimoRunner{transcriber: transcriber}).Run(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if string(transcriber.received) != "authorized-wav-canary" || !result.finalReceived {
		t.Fatalf("received/result = %q/%+v", transcriber.received, result)
	}
	encoded, _ := json.Marshal(result)
	if bytes.Contains(encoded, []byte("recognized-text-canary")) {
		t.Fatalf("result retained recognized text: %s", encoded)
	}
}

func TestReportSchemaHasNoFreePayloadContainers(t *testing.T) {
	metricsType := reflect.TypeFor[Metrics]()
	for index := range metricsType.NumField() {
		field := metricsType.Field(index)
		if field.Type.Kind() != reflect.Pointer {
			t.Fatalf("Metrics.%s type = %s, want pointer", field.Name, field.Type)
		}
		kind := field.Type.Elem().Kind()
		if kind != reflect.Int64 && kind != reflect.Bool && field.Type.Elem() != reflect.TypeFor[HTTPStatusClass]() {
			t.Fatalf("Metrics.%s exposes %s", field.Name, field.Type)
		}
	}
	reportType := reflect.TypeFor[Report]()
	for index := range reportType.NumField() {
		field := reportType.Field(index)
		if field.Type.Kind() == reflect.Map || field.Type.Kind() == reflect.Interface || field.Type.Kind() == reflect.Slice {
			t.Fatalf("Report.%s exposes free payload type %s", field.Name, field.Type)
		}
		closedString := field.Type == reflect.TypeFor[Provider]() || field.Type == reflect.TypeFor[Model]() ||
			field.Type == reflect.TypeFor[Status]() || field.Type == reflect.TypeFor[Code]()
		if field.Type.Kind() == reflect.String && field.Name != "SchemaVersion" && field.Name != "Timestamp" && !closedString {
			t.Fatalf("Report.%s is an unrestricted string", field.Name)
		}
	}
}

func testDependencies() dependencies {
	now := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	return dependencies{
		now:    func() time.Time { return now },
		getenv: func(string) string { return "" },
		readAudio: func(string) (*audioInput, error) {
			return &audioInput{wav: []byte{1, 2}, pcm: []byte{1, 2}, durationMS: 1}, nil
		},
		provider: func(Provider, func(string) string) (providerRunner, error) {
			return runnerFunc(func(context.Context, *audioInput) (providerResult, error) {
				return providerResult{finalReceived: true}, nil
			}), nil
		},
	}
}

type runnerFunc func(context.Context, *audioInput) (providerResult, error)

func (f runnerFunc) Run(ctx context.Context, input *audioInput) (providerResult, error) {
	return f(ctx, input)
}

type fakeLiveClient struct{ session asr.LiveSession }

func (f fakeLiveClient) Dial(context.Context) (asr.LiveSession, error) { return f.session, nil }

type fakeLiveSession struct {
	events   []asr.LiveEvent
	frames   [][]byte
	finishes int
	closes   int
}

func (f *fakeLiveSession) SendPCM(_ context.Context, pcm []byte) error {
	f.frames = append(f.frames, append([]byte(nil), pcm...))
	return nil
}
func (f *fakeLiveSession) FinishInput(context.Context) error { f.finishes++; return nil }
func (f *fakeLiveSession) NextEvent(context.Context) (asr.LiveEvent, error) {
	if len(f.events) == 0 {
		return asr.LiveEvent{}, errors.New("no event")
	}
	event := f.events[0]
	f.events = f.events[1:]
	return event, nil
}
func (f *fakeLiveSession) Close(context.Context) error { f.closes++; return nil }

type fakeWAVTranscriber struct {
	text     string
	received []byte
}

func (f *fakeWAVTranscriber) TranscribeWAV(_ context.Context, wav []byte) (string, error) {
	f.received = append([]byte(nil), wav...)
	return f.text, nil
}
