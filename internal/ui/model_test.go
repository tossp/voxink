package ui

import (
	"errors"
	"strings"
	"testing"
	"time"

	runtimeapp "github.com/tossp/voxink/internal/app"
	"github.com/tossp/voxink/internal/credential"
	"github.com/tossp/voxink/internal/domain"
	"github.com/tossp/voxink/internal/history"
	"github.com/tossp/voxink/internal/settings"
)

func TestAppModelLoadsNewestFirstAndHandlesRuntimeEvents(t *testing.T) {
	settingsStore := &fakeSettingsStore{document: settings.EmptyDocument()}
	credentialStore := &fakeCredentialStore{values: map[credential.Name][]byte{
		credential.VolcAPIKey: []byte("configured-canary"),
	}}
	older := history.Entry{Time: time.Unix(1, 0), Provider: domain.ProviderVolcengineV3, Mode: history.ModeInjected, Final: "older"}
	newer := history.Entry{Time: time.Unix(2, 0), Provider: domain.ProviderMiMoASR, Mode: history.ModeCopied, Final: "newer"}
	application, err := New(Options{
		Settings: settingsStore, Credentials: credentialStore,
		History: fakeHistoryRepository{entries: []history.Entry{older, newer}}, CaptureSupported: true,
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if got := application.History(); len(got) != 2 || got[0].Final != "newer" || got[1].Final != "older" {
		t.Fatalf("History() = %+v", got)
	}
	if !application.CredentialConfigured(credential.VolcAPIKey) || application.CredentialConfigured(credential.MiMoAPIKey) {
		t.Fatalf("configured status was not mapped")
	}
	added := history.Entry{Time: time.Unix(3, 0), Provider: domain.ProviderVolcengineV3, Mode: history.ModeFailed, Final: "latest"}
	application.HandleEvent(runtimeapp.RuntimeEvent{Status: runtimeapp.StatusFailed, History: &added})
	if application.Status() != runtimeapp.StatusFailed || application.History()[0].Final != "latest" {
		t.Fatalf("event model = %q/%+v", application.Status(), application.History())
	}
	if !application.CaptureEnabled(true) || application.CaptureEnabled(false) {
		t.Fatal("capture focus/platform gate was not enforced")
	}
}

func TestSaveValidatesMapsCallbacksAndLeavesBlankCredentialsUntouched(t *testing.T) {
	settingsStore := &fakeSettingsStore{document: settings.EmptyDocument()}
	credentialStore := &fakeCredentialStore{values: make(map[credential.Name][]byte)}
	var hotkey string
	var saved int
	application, err := New(Options{
		Settings: settingsStore, Credentials: credentialStore, History: fakeHistoryRepository{},
		UpdateHotkey: func(value string) error { hotkey = value; return nil },
		Saved:        func() { saved++ },
	})
	if err != nil {
		t.Fatal(err)
	}
	form := FormValues{
		Hotkey: "Shift+Ctrl+Space", VolcEndpoint: "wss://example.test/live",
		VolcReadLimitBytes: "2097152", MiMoEndpoint: "https://example.test/asr",
		MiMoAuthMode: "bearer", Credentials: map[credential.Name]string{
			credential.VolcAPIKey: "new-secret", credential.MiMoAPIKey: "",
		},
	}
	if err := application.Save(form); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if hotkey != "Ctrl+Shift+Space" || settingsStore.document.Hotkey == nil || *settingsStore.document.Hotkey != hotkey {
		t.Fatalf("hotkey callback/document = %q/%+v", hotkey, settingsStore.document.Hotkey)
	}
	if saved != 1 {
		t.Fatalf("saved callback count = %d", saved)
	}
	if got := string(credentialStore.values[credential.VolcAPIKey]); got != "new-secret" {
		t.Fatalf("stored credential = %q", got)
	}
	if _, ok := credentialStore.writes[credential.MiMoAPIKey]; ok {
		t.Fatal("blank credential was modified")
	}
	if application.Form().MiMoAuthMode != "bearer" || !application.CredentialConfigured(credential.VolcAPIKey) {
		t.Fatalf("saved form/configured = %+v/%t", application.Form(), application.CredentialConfigured(credential.VolcAPIKey))
	}
}

func TestSaveValidationAndStorageErrorsAreFixedAndRedacted(t *testing.T) {
	const canary = "credential-path-raw-error-canary"
	settingsStore := &fakeSettingsStore{document: settings.EmptyDocument()}
	credentialStore := &fakeCredentialStore{values: make(map[credential.Name][]byte)}
	application, err := New(Options{Settings: settingsStore, Credentials: credentialStore, History: fakeHistoryRepository{}})
	if err != nil {
		t.Fatal(err)
	}
	invalid := application.Form()
	invalid.Hotkey = "Space"
	if err := application.Save(invalid); !errors.Is(err, ErrSaveFailed) {
		t.Fatalf("invalid Save() error = %v", err)
	}
	credentialStore.writeErr = errors.New(canary)
	valid := application.Form()
	valid.Credentials = map[credential.Name]string{credential.MiMoAPIKey: "secret"}
	err = application.Save(valid)
	if !errors.Is(err, ErrSaveFailed) || strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), "secret") {
		t.Fatalf("storage Save() error = %q", err)
	}
}

type fakeHistoryRepository struct {
	entries []history.Entry
	err     error
}

func (r fakeHistoryRepository) Load() ([]history.Entry, error) {
	return append([]history.Entry(nil), r.entries...), r.err
}

type fakeSettingsStore struct {
	document settings.Document
	err      error
}

func (s *fakeSettingsStore) Load() (settings.Document, error) { return s.document, s.err }
func (s *fakeSettingsStore) Save(document settings.Document) error {
	if s.err == nil {
		s.document = document
	}
	return s.err
}

type fakeCredentialStore struct {
	values   map[credential.Name][]byte
	writes   map[credential.Name]bool
	writeErr error
}

func (s *fakeCredentialStore) Read(name credential.Name) ([]byte, error) {
	value, ok := s.values[name]
	if !ok {
		return nil, credential.ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *fakeCredentialStore) Write(name credential.Name, value []byte) error {
	if s.writeErr != nil {
		return s.writeErr
	}
	if s.writes == nil {
		s.writes = make(map[credential.Name]bool)
	}
	s.writes[name] = true
	s.values[name] = append([]byte(nil), value...)
	return nil
}

func (*fakeCredentialStore) Delete(credential.Name) error { return credential.ErrNotFound }
