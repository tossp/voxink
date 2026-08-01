package settings

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
)

const maximumFileBytes int64 = 64 * 1024

var (
	// ErrInvalidFile is the fixed error for malformed, unsupported, or oversized settings.
	ErrInvalidFile = errors.New("settings file is invalid")
	// ErrStorage is the fixed error for settings path, read, or write failures.
	ErrStorage = errors.New("settings storage failed")
)

// File is the injectable file handle required by FileSystem.
type File interface {
	io.Reader
	io.Writer
	Close() error
	Sync() error
	Name() string
	Chmod(fs.FileMode) error
}

// FileSystem is the injectable persistence boundary used by FileStore tests.
type FileSystem interface {
	Open(string) (File, error)
	CreateTemp(string, string) (File, error)
	MkdirAll(string, fs.FileMode) error
	Replace(string, string) error
	Remove(string) error
}

// FileStore persists one bounded JSON document using atomic replacement.
type FileStore struct {
	path     string
	pathFunc func() (string, error)
	fs       FileSystem
}

// NewFileStore creates a store with an injected path and filesystem.
func NewFileStore(path string, fileSystem FileSystem) *FileStore {
	return &FileStore{path: path, fs: fileSystem}
}

// NewDefaultStore uses os.UserConfigDir()/VoxInk/config.json.
func NewDefaultStore() *FileStore {
	return &FileStore{pathFunc: defaultPath, fs: systemFS{}}
}

func defaultPath() (string, error) {
	directory, err := os.UserConfigDir()
	if err != nil {
		return "", ErrStorage
	}
	return filepath.Join(directory, "VoxInk", "config.json"), nil
}

func (s *FileStore) location() (string, error) {
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

// Load performs a bounded strict decode and treats absence as no overrides.
func (s *FileStore) Load() (Document, error) {
	path, err := s.location()
	if err != nil {
		return Document{}, err
	}
	input, err := s.fs.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return EmptyDocument(), nil
	}
	if err != nil {
		return Document{}, ErrStorage
	}
	body, err := io.ReadAll(io.LimitReader(input, maximumFileBytes+1))
	closeErr := input.Close()
	if err != nil || closeErr != nil {
		return Document{}, ErrStorage
	}
	if int64(len(body)) > maximumFileBytes {
		return Document{}, ErrInvalidFile
	}
	document, err := decode(body)
	if err != nil {
		return Document{}, ErrInvalidFile
	}
	return document, nil
}

func decode(body []byte) (Document, error) {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var document Document
	if err := decoder.Decode(&document); err != nil {
		return Document{}, err
	}
	if document.SchemaVersion != SchemaVersion {
		return Document{}, ErrInvalidFile
	}
	if err := validateDocument(document); err != nil {
		return Document{}, err
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return Document{}, ErrInvalidFile
	}
	return document, nil
}

func validateDocument(document Document) error {
	copy := EmptyDocument()
	for _, item := range []struct {
		key   Key
		value *string
	}{{HotkeyKey, document.Hotkey}, {VolcEndpointKey, document.VolcEndpoint}, {MiMoEndpointKey, document.MiMoEndpoint}, {MiMoAuthModeKey, document.MiMoAuthMode}} {
		if item.value != nil {
			if err := Set(&copy, item.key, *item.value); err != nil {
				return err
			}
		}
	}
	if document.VolcReadLimitBytes != nil {
		if err := Set(&copy, VolcReadLimitBytesKey, strconv.FormatInt(*document.VolcReadLimitBytes, 10)); err != nil {
			return err
		}
	}
	return nil
}

// Save validates, writes, syncs, and atomically replaces the settings file.
func (s *FileStore) Save(document Document) error {
	path, err := s.location()
	if err != nil {
		return err
	}
	document.SchemaVersion = SchemaVersion
	if err := validateDocument(document); err != nil {
		return ErrInvalidFile
	}
	body, err := json.MarshalIndent(document, "", "  ")
	if err != nil || int64(len(body)+1) > maximumFileBytes {
		return ErrInvalidFile
	}
	body = append(body, '\n')
	directory := filepath.Dir(path)
	if err := s.fs.MkdirAll(directory, 0o700); err != nil {
		return ErrStorage
	}
	temporary, err := s.fs.CreateTemp(directory, ".config-*.tmp")
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
	if _, err := io.Copy(temporary, bytes.NewReader(body)); err != nil {
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

func (systemFS) Open(path string) (File, error) { return os.Open(path) }
func (systemFS) CreateTemp(directory, pattern string) (File, error) {
	return os.CreateTemp(directory, pattern)
}
func (systemFS) MkdirAll(path string, mode fs.FileMode) error { return os.MkdirAll(path, mode) }
func (systemFS) Replace(oldPath, newPath string) error        { return replaceFile(oldPath, newPath) }
func (systemFS) Remove(path string) error                     { return os.Remove(path) }
