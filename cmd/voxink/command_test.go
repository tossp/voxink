package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tossp/voxink/internal/selfcheck"
	"github.com/tossp/voxink/internal/smoke"
)

func TestDispatchCommandLeavesNoArgumentsAndUnknownArgumentsUnchanged(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}} {
		var stdout, stderr bytes.Buffer
		handled, code := dispatchCommand(args, &stdout, &stderr)
		if handled || code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("dispatchCommand(%q) = %t/%d stdout=%q stderr=%q", args, handled, code, stdout.String(), stderr.String())
		}
	}
}

func TestDispatchCommandKeepsSelfCheckCompatible(t *testing.T) {
	var stdout, stderr bytes.Buffer
	handled, code := dispatchCommand([]string{"self-check", "--mode=static", "--json"}, &stdout, &stderr)
	if !handled || code != 0 {
		t.Fatalf("dispatch self-check = %t/%d; stderr=%q", handled, code, stderr.String())
	}
	var report selfcheck.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode self-check report: %v", err)
	}
	if report.Mode != selfcheck.ModeStatic || stderr.Len() != 0 {
		t.Fatalf("self-check report/stderr = %+v/%q", report, stderr.String())
	}
}

func TestDispatchSmokeBeforeApplicationStartup(t *testing.T) {
	var stdout, stderr bytes.Buffer
	handled, code := dispatchCommand([]string{"smoke", "volc", "--audio", "secret-path-canary"}, &stdout, &stderr)
	if !handled || code != 2 {
		t.Fatalf("dispatch smoke = %t/%d", handled, code)
	}
	if stdout.Len() != 0 || stderr.String() != "VoxInk smoke: invalid arguments\n" {
		t.Fatalf("smoke stdout/stderr = %q/%q", stdout.String(), stderr.String())
	}
}

func TestDispatchSmokeMissingConfigIsRedactedBeforeApplicationStartup(t *testing.T) {
	for _, key := range []string{
		"VOXINK_VOLC_API_KEY",
		"VOXINK_VOLC_RESOURCE_ID",
		"VOXINK_VOLC_APP_KEY",
		"VOXINK_VOLC_ACCESS_KEY",
		"VOXINK_VOLC_ENDPOINT",
		"VOXINK_VOLC_READ_LIMIT_BYTES",
	} {
		t.Setenv(key, "")
	}

	const canary = "/private/audio/dispatch-secret-canary.wav"
	var stdout, stderr bytes.Buffer
	handled, code := dispatchCommand(
		[]string{"smoke", "volc", "--audio", canary, "--confirm-send", "--json"},
		&stdout,
		&stderr,
	)
	if !handled || code != 1 {
		t.Fatalf("dispatch smoke = %t/%d", handled, code)
	}

	var report smoke.Report
	if err := json.Unmarshal(stdout.Bytes(), &report); err != nil {
		t.Fatalf("decode smoke report: %v", err)
	}
	if report.SchemaVersion != smoke.SchemaVersion ||
		report.Provider != smoke.ProviderVolc ||
		report.Model != smoke.ModelVolc ||
		report.Status != smoke.StatusFail ||
		report.Code != smoke.CodeConfigMissing ||
		report.Metrics != (smoke.Metrics{}) {
		t.Fatalf("smoke report = %+v", report)
	}
	if _, err := time.Parse(time.RFC3339, report.Timestamp); err != nil {
		t.Fatalf("smoke report timestamp = %q: %v", report.Timestamp, err)
	}
	if stderr.String() != "VoxInk smoke: the authorized WAV will be sent only to the selected Provider\n" {
		t.Fatalf("smoke stderr = %q", stderr.String())
	}
	if strings.Contains(stdout.String(), canary) || strings.Contains(stderr.String(), canary) ||
		strings.Contains(stdout.String(), "dispatch-secret-canary") || strings.Contains(stderr.String(), "dispatch-secret-canary") {
		t.Fatalf("smoke output leaked audio path: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}
