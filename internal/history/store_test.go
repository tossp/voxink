package history

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/tossp/voxink/internal/domain"
)

func TestAppendLoadAndBoundedTrim(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	store := NewStore(path, systemFS{})
	start := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	for index := 0; index < MaximumEntries+3; index++ {
		entry := Entry{
			Time: start.Add(time.Duration(index) * time.Second), Provider: domain.ProviderVolcengineV3,
			Mode: ModeInjected, Final: fmt.Sprintf("text-%d", index),
		}
		if err := store.Append(entry); err != nil {
			t.Fatalf("Append(%d) error = %v", index, err)
		}
	}
	entries, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if len(entries) != MaximumEntries || entries[0].Final != "text-3" || entries[len(entries)-1].Final != "text-202" {
		t.Fatalf("bounded entries = %d, first=%q last=%q", len(entries), entries[0].Final, entries[len(entries)-1].Final)
	}
}

func TestLoadRecoversValidLinesAndAppendRepairsDamage(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.jsonl")
	valid := Entry{Time: time.Now().UTC(), Provider: domain.ProviderMiMoASR, Mode: ModeCopied, Final: "valid"}
	validJSON := fmt.Sprintf(`{"Time":%q,"Provider":%q,"Mode":%q,"Final":%q}`, valid.Time.Format(time.RFC3339Nano), valid.Provider, valid.Mode, valid.Final)
	if err := os.WriteFile(path, []byte("damaged\n"+validJSON+"\n{\"Final\":\"partial\""), 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path, systemFS{})
	entries, err := store.Load()
	if err != nil || len(entries) != 1 || entries[0].Final != valid.Final {
		t.Fatalf("Load() = %+v, %v", entries, err)
	}
	next := Entry{Time: valid.Time.Add(time.Second), Provider: domain.ProviderVolcengineV3, Mode: ModeInjected, Final: "next"}
	if err := store.Append(next); err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "damaged") || strings.Count(strings.TrimSpace(string(body)), "\n") != 1 {
		t.Fatalf("repaired JSONL = %q", body)
	}
}

func TestAppendReplaceFailureIsAtomicAndRedacted(t *testing.T) {
	const canary = "raw-path-and-final-canary"
	directory := t.TempDir()
	path := filepath.Join(directory, "history.jsonl")
	original := []byte("original\n")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path, failingReplaceFS{systemFS{}})
	err := store.Append(Entry{
		Time: time.Now().UTC(), Provider: domain.ProviderVolcengineV3, Mode: ModeFailed, Final: canary,
	})
	if !errors.Is(err, ErrStorage) || strings.Contains(err.Error(), canary) || strings.Contains(err.Error(), path) {
		t.Fatalf("Append() error = %q", err)
	}
	body, readErr := os.ReadFile(path)
	if readErr != nil || !reflect.DeepEqual(body, original) {
		t.Fatalf("target after failed replace = %q, %v", body, readErr)
	}
	matches, globErr := filepath.Glob(filepath.Join(directory, ".history-*.tmp"))
	if globErr != nil || len(matches) != 0 {
		t.Fatalf("temporary files = %v, %v", matches, globErr)
	}
}

type failingReplaceFS struct{ systemFS }

func (failingReplaceFS) Replace(string, string) error {
	return fmt.Errorf("replace failed: %w", fs.ErrPermission)
}
