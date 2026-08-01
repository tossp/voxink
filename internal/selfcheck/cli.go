package selfcheck

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"time"
)

const (
	defaultAudioDuration = 3 * time.Second
	defaultToggleTimeout = 15 * time.Second
	maximumProbeDuration = 60 * time.Second
)

// Execute parses self-check arguments, writes one report, and returns a process exit code.
func Execute(args []string, stdout, stderr io.Writer) int {
	return execute(args, stdout, stderr, defaultDependencies())
}

func execute(args []string, stdout, stderr io.Writer, deps dependencies) int {
	flags := flag.NewFlagSet("self-check", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	modeValue := flags.String("mode", string(ModeStatic), "static, audio, or interactive")
	duration := flags.Duration("duration", defaultAudioDuration, "audio probe duration")
	timeout := flags.Duration("timeout", defaultToggleTimeout, "interactive toggle timeout")
	jsonOutput := flags.Bool("json", false, "write JSON")
	if err := flags.Parse(args); err != nil || flags.NArg() != 0 {
		fmt.Fprintln(stderr, "VoxInk self-check: invalid arguments")
		return 2
	}
	mode := Mode(*modeValue)
	if mode != ModeStatic && mode != ModeAudio && mode != ModeInteractive {
		fmt.Fprintln(stderr, "VoxInk self-check: mode must be static, audio, or interactive")
		return 2
	}
	if *duration <= 0 || *duration > maximumProbeDuration || *timeout <= 0 || *timeout > maximumProbeDuration {
		fmt.Fprintln(stderr, "VoxInk self-check: duration and timeout must be greater than zero and at most 60s")
		return 2
	}

	if mode == ModeInteractive {
		fmt.Fprintln(stderr, "VoxInk self-check: press Ctrl+Shift+Space once; visually confirm the overlay does not take focus")
	}
	report := run(Options{Mode: mode, Duration: *duration, Timeout: *timeout}, deps)
	if *jsonOutput {
		encoder := json.NewEncoder(stdout)
		encoder.SetEscapeHTML(false)
		if err := encoder.Encode(report); err != nil {
			fmt.Fprintln(stderr, "VoxInk self-check: could not write report")
			return 1
		}
	} else {
		writeText(stdout, report)
	}
	return ExitCode(report)
}

func writeText(output io.Writer, report Report) {
	fmt.Fprintf(output, "VoxInk self-check %s (%s)\n", report.SchemaVersion, report.Mode)
	fmt.Fprintf(output, "Version: %s; %s/%s; %s; cgo=%t\n", report.App.Version, report.Runtime.GOOS, report.Runtime.GOARCH, report.Runtime.GoVersion, report.Runtime.CGOAvailable)
	for _, check := range report.Checks {
		fmt.Fprintf(output, "- %s: %s (%s)\n", check.Name, check.Status, check.Code)
	}
}
