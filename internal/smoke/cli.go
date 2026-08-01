package smoke

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/tossp/voxink/internal/credential"
	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
	"github.com/tossp/voxink/internal/settings"
)

const (
	defaultTimeout = 30 * time.Second
	minimumTimeout = time.Second
	maximumTimeout = 120 * time.Second
)

// Execute parses smoke arguments, runs only the selected Provider, writes one
// redacted report, and returns a process exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	return ExecuteWithCredentials(args, stdout, stderr, nil)
}

// ExecuteWithCredentials runs smoke with Credential Manager precedence over env.
func ExecuteWithCredentials(args []string, stdout, stderr io.Writer, store credential.Store) int {
	return ExecuteWithSettings(args, stdout, stderr, store, settings.NewDefaultStore())
}

// ExecuteWithSettings runs smoke with injected credential and settings stores.
func ExecuteWithSettings(args []string, stdout, stderr io.Writer, store credential.Store, loader settings.Loader) int {
	deps := defaultDependencies()
	deps.store = store
	deps.settings = loader
	return execute(args, stdout, stderr, deps)
}

func execute(args []string, stdout, stderr io.Writer, deps dependencies) int {
	options, ok := parseArguments(args)
	if !ok {
		fmt.Fprintln(stderr, "VoxInk smoke: invalid arguments")
		return 2
	}
	fmt.Fprintln(stderr, "VoxInk smoke: the authorized WAV will be sent only to the selected Provider")

	report := run(context.Background(), options, deps)
	if options.json {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, "VoxInk smoke: could not write report")
			return 1
		}
	} else {
		writeText(stdout, report)
	}
	if report.Status == StatusPass {
		return 0
	}
	return 1
}

func parseArguments(args []string) (options, bool) {
	if len(args) == 0 {
		return options{}, false
	}
	provider := Provider(args[0])
	if provider != ProviderVolc && provider != ProviderMiMo {
		return options{}, false
	}
	flags := flag.NewFlagSet("smoke", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	audioPath := flags.String("audio", "", "authorized WAV")
	confirmSend := flags.Bool("confirm-send", false, "confirm Provider transmission")
	timeout := flags.Duration("timeout", defaultTimeout, "Provider timeout")
	jsonOutput := flags.Bool("json", false, "write JSON")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 {
		return options{}, false
	}
	if *audioPath == "" || !*confirmSend || *timeout < minimumTimeout || *timeout > maximumTimeout {
		return options{}, false
	}
	return options{provider: provider, audio: *audioPath, timeout: *timeout, json: *jsonOutput}, true
}

func run(parent context.Context, options options, deps dependencies) Report {
	report := Report{
		SchemaVersion: SchemaVersion,
		Timestamp:     deps.now().UTC().Format(time.RFC3339),
		Provider:      options.provider,
		Model:         modelFor(options.provider),
		Status:        StatusFail,
		Code:          CodeInternalFailure,
	}

	runner, err := deps.provider(options.provider, deps.getenv, deps.store, deps.settings)
	if err != nil {
		report.Code = CodeConfigMissing
		return report
	}
	input, err := deps.readAudio(options.audio)
	if err != nil {
		report.Code = classifyAudioError(err)
		return report
	}
	defer input.release()
	report.Metrics.AudioDurationMS = integer(input.durationMS)
	report.Metrics.AudioBytes = integer(int64(len(input.wav)))

	ctx, cancel := context.WithTimeout(parent, options.timeout)
	started := deps.now()
	result, err := runner.Run(ctx, input)
	report.Metrics.LatencyMS = integer(max(deps.now().Sub(started).Milliseconds(), 0))
	applyResult(&report.Metrics, result)
	applyErrorMetrics(&report.Metrics, err)
	if err != nil {
		report.Code = classifyProviderError(ctx, err)
		cancel()
		return report
	}
	cancel()
	report.Status = StatusPass
	report.Code = CodeOK
	return report
}

func modelFor(provider Provider) Model {
	if provider == ProviderVolc {
		return ModelVolc
	}
	return ModelMiMo
}

func classifyAudioError(err error) Code {
	switch {
	case errors.Is(err, errAudioUnavailable):
		return CodeAudioUnavailable
	case errors.Is(err, errAudioTooLarge):
		return CodeAudioTooLarge
	default:
		return CodeAudioInvalid
	}
}

func applyResult(metrics *Metrics, result providerResult) {
	metrics.EventCount = integer(result.eventCount)
	metrics.FinalReceived = boolean(result.finalReceived)
	if result.protocolTerminal {
		metrics.ProtocolTerminal = boolean(true)
	}
}

func applyErrorMetrics(metrics *Metrics, err error) {
	var mimoHTTP *mimo.HTTPError
	if errors.As(err, &mimoHTTP) {
		if value := statusClass(mimoHTTP.StatusCode); value != "" {
			metrics.HTTPStatusClass = &value
		}
		return
	}
	var volcHandshake *volcengine.HandshakeError
	if errors.As(err, &volcHandshake) {
		if value := statusClass(volcHandshake.StatusCode); value != "" {
			metrics.HTTPStatusClass = &value
		}
	}
}

func writeText(output io.Writer, report Report) {
	fmt.Fprintf(output, "VoxInk Provider smoke %s\n", report.SchemaVersion)
	fmt.Fprintf(output, "Provider: %s\n", report.Provider)
	fmt.Fprintf(output, "Model: %s\n", report.Model)
	fmt.Fprintf(output, "Status: %s\n", report.Status)
	fmt.Fprintf(output, "Code: %s\n", report.Code)
}

type providerRunner interface {
	Run(context.Context, *audioInput) (providerResult, error)
}

type dependencies struct {
	now       func() time.Time
	getenv    func(string) string
	readAudio func(string) (*audioInput, error)
	provider  func(Provider, func(string) string, credential.Store, settings.Loader) (providerRunner, error)
	store     credential.Store
	settings  settings.Loader
}

func defaultDependencies() dependencies {
	return dependencies{
		now: time.Now, getenv: os.Getenv, readAudio: readWAV,
		provider: newProviderRunner, settings: settings.NewDefaultStore(),
	}
}
