package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestExecuteSetListDelete(t *testing.T) {
	repository := &memoryRepository{document: EmptyDocument()}
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"set", "hotkey", "alt+f3"}, &stdout, &stderr, repository, nil); code != 0 {
		t.Fatalf("set code = %d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if repository.document.Hotkey == nil || *repository.document.Hotkey != "Alt+F3" {
		t.Fatalf("stored hotkey = %+v", repository.document.Hotkey)
	}

	stdout.Reset()
	if code := Execute([]string{"list", "--json"}, &stdout, &stderr, repository, nil); code != 0 {
		t.Fatalf("list code = %d stderr=%q", code, stderr.String())
	}
	var values map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &values); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if values["hotkey"] != "Alt+F3" || values["mimo-auth-mode"] != "api-key" || values["volc-read-limit-bytes"] != float64(DefaultVolcReadLimitBytes) || len(values) != 5 {
		t.Fatalf("list values = %#v", values)
	}

	stdout.Reset()
	if code := Execute([]string{"delete", "hotkey"}, &stdout, &stderr, repository, func(key string) string {
		if key == "VOXINK_HOTKEY" {
			return "Win+9"
		}
		return ""
	}); code != 0 {
		t.Fatalf("delete code = %d stderr=%q", code, stderr.String())
	}
	if repository.document.Hotkey != nil {
		t.Fatalf("delete retained hotkey = %v", *repository.document.Hotkey)
	}
	stdout.Reset()
	if code := Execute([]string{"list"}, &stdout, &stderr, repository, func(key string) string {
		if key == "VOXINK_HOTKEY" {
			return "Win+9"
		}
		return ""
	}); code != 0 || !strings.HasPrefix(stdout.String(), "hotkey=Win+9\n") {
		t.Fatalf("fallback list code/output = %d/%q", code, stdout.String())
	}
}

func TestExecuteInvalidDoesNotSaveOrLeak(t *testing.T) {
	const canary = "token-secret-canary"
	repository := &memoryRepository{document: EmptyDocument()}
	var stdout, stderr bytes.Buffer
	code := Execute([]string{"set", "mimo-endpoint", "https://example.test/asr?token=" + canary}, &stdout, &stderr, repository, nil)
	if code != 1 || repository.saves != 0 || stdout.Len() != 0 || stderr.String() != invalidValueMessage+"\n" {
		t.Fatalf("invalid set = code %d saves %d stdout=%q stderr=%q", code, repository.saves, stdout.String(), stderr.String())
	}
	if strings.Contains(stderr.String(), canary) {
		t.Fatal("invalid set leaked value")
	}

	stderr.Reset()
	if code := Execute([]string{"set", "unknown", canary}, &stdout, &stderr, repository, nil); code != 2 || repository.saves != 0 {
		t.Fatalf("invalid args code/saves = %d/%d", code, repository.saves)
	}
}

func TestExecuteStorageFailureIsFixed(t *testing.T) {
	repository := &memoryRepository{loadError: errors.New("path raw-canary")}
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"list"}, &stdout, &stderr, repository, nil); code != 1 {
		t.Fatalf("list code = %d", code)
	}
	if stdout.Len() != 0 || stderr.String() != storageFailedMessage+"\n" || strings.Contains(stderr.String(), "canary") {
		t.Fatalf("storage output = %q/%q", stdout.String(), stderr.String())
	}
}

type memoryRepository struct {
	document  Document
	loadError error
	saveError error
	saves     int
}

func (r *memoryRepository) Load() (Document, error) { return r.document, r.loadError }
func (r *memoryRepository) Save(document Document) error {
	r.saves++
	if r.saveError == nil {
		r.document = document
	}
	return r.saveError
}
