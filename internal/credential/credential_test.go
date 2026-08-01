package credential

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestCredentialCLISetReplaceDeleteAndList(t *testing.T) {
	store := newFakeStore()
	for _, value := range []string{"first-secret-canary", "replacement-secret-canary"} {
		var stdout, stderr bytes.Buffer
		code := Execute([]string{"set", string(VolcAPIKey)}, &stdout, &stderr, store, func() ([]byte, error) {
			return []byte(value), nil
		})
		if code != 0 || stdout.String() != "VoxInk credential: configured\n" || stderr.Len() != 0 {
			t.Fatalf("set exit/output = %d %q %q", code, stdout.String(), stderr.String())
		}
		if strings.Contains(stdout.String()+stderr.String(), value) {
			t.Fatal("set output leaked input")
		}
	}
	if got := string(store.values[VolcAPIKey]); got != "replacement-secret-canary" {
		t.Fatalf("stored replacement = %q", got)
	}

	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"list", "--json"}, &stdout, &stderr, store, nil); code != 0 {
		t.Fatalf("list exit = %d; stderr=%q", code, stderr.String())
	}
	var statuses []Status
	if err := json.Unmarshal(stdout.Bytes(), &statuses); err != nil {
		t.Fatal(err)
	}
	if len(statuses) != len(Names()) || !statuses[0].Configured || statuses[0].Name != VolcAPIKey {
		t.Fatalf("statuses = %+v", statuses)
	}
	for _, canary := range []string{"replacement-secret-canary", "first-secret-canary"} {
		if strings.Contains(stdout.String(), canary) {
			t.Fatalf("list leaked %q", canary)
		}
	}

	for range 2 {
		stdout.Reset()
		stderr.Reset()
		if code := Execute([]string{"delete", string(VolcAPIKey)}, &stdout, &stderr, store, nil); code != 0 || stdout.String() != "VoxInk credential: deleted\n" {
			t.Fatalf("delete exit/output = %d %q %q", code, stdout.String(), stderr.String())
		}
	}
}

func TestCredentialCLIRejectsArgumentsEmptyInputAndUnsupported(t *testing.T) {
	store := newFakeStore()
	tests := []struct {
		name  string
		args  []string
		store Store
		read  ValueReader
		code  int
		want  string
	}{
		{name: "unknown name", args: []string{"set", "secret-canary"}, store: store, read: func() ([]byte, error) { return []byte("never-read"), nil }, code: 2, want: invalidArgumentsMessage + "\n"},
		{name: "value argument", args: []string{"set", string(VolcAPIKey), "secret-canary"}, store: store, code: 2, want: invalidArgumentsMessage + "\n"},
		{name: "empty", args: []string{"set", string(VolcAPIKey)}, store: store, read: func() ([]byte, error) { return nil, errors.New("empty") }, code: 1, want: invalidInputMessage + "\n"},
		{name: "unsupported", args: []string{"list"}, code: 1, want: unsupportedMessage + "\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := Execute(tt.args, &stdout, &stderr, tt.store, tt.read); code != tt.code || stdout.Len() != 0 || stderr.String() != tt.want {
				t.Fatalf("exit/output = %d %q %q", code, stdout.String(), stderr.String())
			}
			if strings.Contains(stdout.String()+stderr.String(), "secret-canary") {
				t.Fatal("argument failure leaked canary")
			}
		})
	}
}

func TestCredentialCLITextListAndStorageFailureAreRedacted(t *testing.T) {
	store := newFakeStore()
	store.values[VolcResourceID] = []byte("resource-secret-canary")
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"list"}, &stdout, &stderr, store, nil); code != 0 || stderr.Len() != 0 {
		t.Fatalf("text list exit/output = %d %q %q", code, stdout.String(), stderr.String())
	}
	want := "volc-api-key configured=false\nvolc-resource-id configured=true\nvolc-app-key configured=false\nvolc-access-key configured=false\nmimo-api-key configured=false\n"
	if stdout.String() != want || strings.Contains(stdout.String(), "resource-secret-canary") {
		t.Fatalf("text list = %q", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	if code := Execute([]string{"list"}, &stdout, &stderr, failingStore{}, nil); code != 1 || stdout.Len() != 0 || stderr.String() != storageFailedMessage+"\n" {
		t.Fatalf("storage failure exit/output = %d %q %q", code, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), "raw-storage-canary") {
		t.Fatal("storage failure leaked raw error")
	}
}

func TestResolverCredentialThenEnvironmentThenMissing(t *testing.T) {
	store := newFakeStore()
	store.values[MiMoAPIKey] = []byte("stored-secret")
	resolver := Resolver{Store: store, Getenv: func(string) string { return "environment-secret" }}
	if got, err := resolver.Get(MiMoAPIKey); err != nil || got != "stored-secret" {
		t.Fatalf("stored resolution = %q, %v", got, err)
	}
	delete(store.values, MiMoAPIKey)
	if got, err := resolver.Get(MiMoAPIKey); err != nil || got != "environment-secret" {
		t.Fatalf("environment resolution = %q, %v", got, err)
	}
	resolver.Getenv = func(string) string { return "" }
	if got, err := resolver.Get(MiMoAPIKey); err != nil || got != "" {
		t.Fatalf("missing resolution = %q, %v", got, err)
	}
}

func TestReadValueRemovesOnlyLineEndingAndBoundsInput(t *testing.T) {
	value, err := ReadValue(strings.NewReader(" secret \r\n"))
	if err != nil || string(value) != " secret " {
		t.Fatalf("ReadValue() = %q, %v", value, err)
	}
	if _, err := ReadValue(strings.NewReader("\n")); err == nil {
		t.Fatal("empty input accepted")
	}
	if _, err := ReadValue(strings.NewReader(strings.Repeat("x", MaximumValueBytes+1))); err == nil {
		t.Fatal("oversized input accepted")
	}
}

func TestFixedTargetsAndEnvironmentKeys(t *testing.T) {
	for _, name := range Names() {
		if got := Target(name); got != "VoxInk/"+string(name) {
			t.Fatalf("Target(%q) = %q", name, got)
		}
		if key := EnvironmentKey(name); !strings.HasPrefix(key, "VOXINK_") {
			t.Fatalf("EnvironmentKey(%q) = %q", name, key)
		}
	}
}

type fakeStore struct{ values map[Name][]byte }

func newFakeStore() *fakeStore { return &fakeStore{values: make(map[Name][]byte)} }

func (s *fakeStore) Read(name Name) ([]byte, error) {
	value, ok := s.values[name]
	if !ok {
		return nil, ErrNotFound
	}
	return append([]byte(nil), value...), nil
}

func (s *fakeStore) Write(name Name, value []byte) error {
	s.values[name] = append([]byte(nil), value...)
	return nil
}

func (s *fakeStore) Delete(name Name) error {
	if _, ok := s.values[name]; !ok {
		return ErrNotFound
	}
	delete(s.values, name)
	return nil
}

type failingStore struct{}

func (failingStore) Read(Name) ([]byte, error) { return nil, errors.New("raw-storage-canary") }
func (failingStore) Write(Name, []byte) error  { return errors.New("raw-storage-canary") }
func (failingStore) Delete(Name) error         { return errors.New("raw-storage-canary") }
