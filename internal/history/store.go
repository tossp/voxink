// Package history persists the bounded local recognition history.
package history

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/tossp/voxink/internal/domain"
)

const (
	// MaximumEntries is the fixed number of recent entries retained on disk.
	MaximumEntries   = 200
	maximumLineBytes = 4 * 1024 * 1024
)

var (
	// ErrStorage is the fixed error for history path, read, or write failures.
	ErrStorage = errors.New("history storage failed")
	// ErrInvalidEntry reports an entry that cannot be persisted.
	ErrInvalidEntry = errors.New("history entry is invalid")
)

// Mode is the final local delivery outcome shown in history.
type Mode string

const (
	ModeInjected Mode = "Injected"
	ModeCopied   Mode = "Copied"
	ModeFailed   Mode = "Failed"
)

// Entry is one local final-recognition history item.
type Entry struct {
	Time     time.Time
	Provider domain.ProviderKind
	Mode     Mode
	Final    string
}

// File is the temporary file contract needed for atomic replacement.
type File interface {
	io.Writer
	Close() error
	Sync() error
	Name() string
	Chmod(fs.FileMode) error
}

// FileSystem is the injectable persistence boundary used by Store.
type FileSystem interface {
	ReadFile(string) ([]byte, error)
	MkdirAll(string, fs.FileMode) error
	CreateTemp(string, string) (File, error)
	Replace(string, string) error
	Remove(string) error
}

// Store atomically retains recent JSONL entries.
type Store struct {
	path     string
	pathFunc func() (string, error)
	fs       FileSystem
	mu       sync.Mutex
}

// NewStore creates a history store with an injected path and filesystem.
func NewStore(path string, fileSystem FileSystem) *Store {
	return &Store{path: path, fs: fileSystem}
}

// NewDefaultStore uses os.UserConfigDir()/VoxInk/history.jsonl.
func NewDefaultStore() *Store {
	return &Store{pathFunc: defaultPath, fs: systemFS{}}
}

func defaultPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", ErrStorage
	}
	return filepath.Join(directory, "VoxInk", "history.jsonl"), nil
}

func (s *Store) location() (string, error) {
	if s == nil || s.fs == nil {
		return "", ErrStorage
	}
	path := s.path
	if s.pathFunc != nil {
		var err error
		path, err = s.pathFunc()
		if err != nil {
			return "", ErrStorage
		}
	}
	if path == "" {
		return "", ErrStorage
	}
	return path, nil
}

// Load returns valid entries in oldest-to-newest order and skips damaged lines.
func (s *Store) Load() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadLocked()
}

func (s *Store) loadLocked() ([]Entry, error) {
	path, err := s.location()
	if err != nil {
		return nil, err
	}
	body, err := s.fs.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, ErrStorage
	}
	entries := decodeLines(body)
	if len(entries) > MaximumEntries {
		entries = entries[len(entries)-MaximumEntries:]
	}
	return entries, nil
}

// Append validates entry and atomically replaces the bounded JSONL file.
func (s *Store) Append(entry Entry) error {
	if !validEntry(entry) {
		return ErrInvalidEntry
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	entries, err := s.loadLocked()
	if err != nil {
		return err
	}
	entries = append(entries, entry)
	if len(entries) > MaximumEntries {
		entries = entries[len(entries)-MaximumEntries:]
	}
	return s.replaceLocked(entries)
}

func validEntry(entry Entry) bool {
	if entry.Time.IsZero() || entry.Provider == "" {
		return false
	}
	return entry.Mode == ModeInjected || entry.Mode == ModeCopied || entry.Mode == ModeFailed
}

func decodeLines(body []byte) []Entry {
	scanner := bufio.NewScanner(bytes.NewReader(body))
	scanner.Buffer(make([]byte, 4096), maximumLineBytes)
	entries := make([]Entry, 0, MaximumEntries)
	for scanner.Scan() {
		var entry Entry
		if json.Unmarshal(scanner.Bytes(), &entry) == nil && validEntry(entry) {
			entries = append(entries, entry)
		}
	}
	return entries
}

func (s *Store) replaceLocked(entries []Entry) error {
	path, err := s.location()
	if err != nil {
		return err
	}
	var body bytes.Buffer
	encoder := json.NewEncoder(&body)
	encoder.SetEscapeHTML(false)
	for _, entry := range entries {
		if err := encoder.Encode(entry); err != nil {
			return ErrStorage
		}
	}
	directory := filepath.Dir(path)
	if err := s.fs.MkdirAll(directory, 0o700); err != nil {
		return ErrStorage
	}
	temporary, err := s.fs.CreateTemp(directory, ".history-*.tmp")
	if err != nil {
		return ErrStorage
	}
	temporaryPath := temporary.Name()
	committed := false
	defer func() {
		if !committed {
			_ = s.fs.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return ErrStorage
	}
	if _, err := io.Copy(temporary, &body); err != nil {
		_ = temporary.Close()
		return ErrStorage
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return ErrStorage
	}
	if err := temporary.Close(); err != nil {
		return ErrStorage
	}
	if err := s.fs.Replace(temporaryPath, path); err != nil {
		return ErrStorage
	}
	committed = true
	return nil
}

type systemFS struct{}

func (systemFS) ReadFile(path string) ([]byte, error) { return os.ReadFile(path) }
func (systemFS) MkdirAll(path string, mode fs.FileMode) error {
	return os.MkdirAll(path, mode)
}
func (systemFS) CreateTemp(directory, pattern string) (File, error) {
	return os.CreateTemp(directory, pattern)
}
func (systemFS) Replace(oldPath, newPath string) error { return replaceFile(oldPath, newPath) }
func (systemFS) Remove(path string) error              { return os.Remove(path) }
