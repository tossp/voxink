package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/tossp/voxink/internal/credential"
	"github.com/tossp/voxink/internal/selfcheck"
	"github.com/tossp/voxink/internal/settings"
	"github.com/tossp/voxink/internal/smoke"
)

func TestDispatchCommandLeavesNoArgumentsAndUnknownArgumentsUnchanged(t *testing.T) {
	for _, args := range [][]string{nil, {"unknown"}} {
		var stdout, stderr bytes.Buffer
		handled, code := dispatchCommand(args, strings.NewReader(""), &stdout, &stderr)
		if handled || code != 0 || stdout.Len() != 0 || stderr.Len() != 0 {
			t.Fatalf("dispatchCommand(%q) = %t/%d stdout=%q stderr=%q", args, handled, code, stdout.String(), stderr.String())
		}
	}
}

func TestDispatchCommandKeepsSelfCheckCompatible(t *testing.T) {
	var stdout, stderr bytes.Buffer
	handled, code := dispatchCommand([]string{"self-check", "--mode=static", "--json"}, strings.NewReader(""), &stdout, &stderr)
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
	handled, code := dispatchCommand([]string{"smoke", "volc", "--audio", "secret-path-canary"}, strings.NewReader(""), &stdout, &stderr)
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
	handled, code := dispatchCommandWithStores(
		[]string{"smoke", "volc", "--audio", canary, "--confirm-send", "--json"},
		strings.NewReader(""),
		&stdout,
		&stderr,
		&commandCredentialStore{values: make(map[credential.Name][]byte)},
		&commandSettingsStore{document: settings.EmptyDocument()},
		func(string) string { return "" },
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

func TestDispatchCredentialSetReadsStdinWithoutLeakingIt(t *testing.T) {
	const canary = "stdin-secret-canary"
	store := &commandCredentialStore{values: make(map[credential.Name][]byte)}
	var stdout, stderr bytes.Buffer
	handled, code := dispatchCommandWithStore(
		[]string{"config", "credential", "set", string(credential.MiMoAPIKey)},
		strings.NewReader(canary+"\n"), &stdout, &stderr, store,
	)
	if !handled || code != 0 || stdout.String() != "VoxInk credential: configured\n" || stderr.Len() != 0 {
		t.Fatalf("dispatch credential = %t/%d stdout=%q stderr=%q", handled, code, stdout.String(), stderr.String())
	}
	if got := string(store.values[credential.MiMoAPIKey]); got != canary {
		t.Fatalf("stored value = %q", got)
	}
	if strings.Contains(stdout.String()+stderr.String(), canary) {
		t.Fatal("credential command leaked stdin")
	}
}

func TestDispatchSettingsSetListDeleteAndSelfCheckIsolation(t *testing.T) {
	store := &commandSettingsStore{document: settings.EmptyDocument()}
	credentials := &commandCredentialStore{values: make(map[credential.Name][]byte)}
	var stdout, stderr bytes.Buffer
	handled, code := dispatchCommandWithStores(
		[]string{"config", "settings", "set", "hotkey", "Win+F7"}, strings.NewReader(""), &stdout, &stderr,
		credentials, store, func(string) string { return "" },
	)
	if !handled || code != 0 || store.document.Hotkey == nil || *store.document.Hotkey != "Win+F7" {
		t.Fatalf("settings set = %t/%d document=%+v stderr=%q", handled, code, store.document, stderr.String())
	}
	stdout.Reset()
	handled, code = dispatchCommandWithStores(
		[]string{"config", "settings", "list", "--json"}, strings.NewReader(""), &stdout, &stderr,
		credentials, store, func(string) string { return "" },
	)
	if !handled || code != 0 || !strings.Contains(stdout.String(), `"hotkey":"Win+F7"`) {
		t.Fatalf("settings list = %t/%d stdout=%q stderr=%q", handled, code, stdout.String(), stderr.String())
	}

	store.loadErr = errors.New("settings path canary")
	stdout.Reset()
	stderr.Reset()
	handled, code = dispatchCommandWithStores(
		[]string{"self-check", "--mode=static", "--json"}, strings.NewReader(""), &stdout, &stderr,
		credentials, store, func(string) string { return "" },
	)
	if !handled || code != 0 || store.loadCalls != 2 {
		t.Fatalf("self-check settings isolation = %t/%d loadCalls=%d stderr=%q", handled, code, store.loadCalls, stderr.String())
	}
}

type commandCredentialStore struct{ values map[credential.Name][]byte }

type commandSettingsStore struct {
	document  settings.Document
	loadErr   error
	saveErr   error
	loadCalls int
}

func (s *commandSettingsStore) Load() (settings.Document, error) {
	s.loadCalls++
	return s.document, s.loadErr
}
func (s *commandSettingsStore) Save(document settings.Document) error {
	if s.saveErr == nil {
		s.document = document
	}
	return s.saveErr
}

func (s *commandCredentialStore) Read(name credential.Name) ([]byte, error) {
	value, ok := s.values[name]
	if !ok {
		return nil, credential.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}
func (s *commandCredentialStore) Write(name credential.Name, value []byte) error {
	s.values[name] = append([]byte(nil), value...)
	return nil
}
func (s *commandCredentialStore) Delete(name credential.Name) error {
	if _, ok := s.values[name]; !ok {
		return credential.ErrNotFound
	}
	delete(s.values, name)
	return nil
}
