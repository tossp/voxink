package settings

import (
	"bytes"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestFileStoreStrictBoundedLoading(t *testing.T) {
	const path = "fake/VoxInk/config.json"
	tests := []struct {
		name string
		body string
	}{
		{"unknown field", `{"SchemaVersion":1,"CredentialCanary":"secret-canary"}`},
		{"version", `{"SchemaVersion":2}`},
		{"corrupt", `{"SchemaVersion":1`},
		{"trailing", `{"SchemaVersion":1} {}`},
		{"invalid endpoint", `{"SchemaVersion":1,"MiMoEndpoint":"https://example.test/?token=secret-canary"}`},
		{"oversize", strings.Repeat("x", int(maximumFileBytes)+1)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fileSystem := newMemoryFS()
			fileSystem.files[path] = []byte(test.body)
			_, err := NewFileStore(path, fileSystem).Load()
			if !errors.Is(err, ErrInvalidFile) {
				t.Fatalf("Load() error = %v, want ErrInvalidFile", err)
			}
			if strings.Contains(err.Error(), "secret-canary") || strings.Contains(err.Error(), path) {
				t.Fatalf("Load() leaked canary or path: %q", err)
			}
		})
	}
}

func TestFileStoreAtomicWriteAndRoundTrip(t *testing.T) {
	const path = "fake/VoxInk/config.json"
	fileSystem := newMemoryFS()
	store := NewFileStore(path, fileSystem)
	document := EmptyDocument()
	if err := Set(&document, HotkeyKey, "Win+F8"); err != nil {
		t.Fatal(err)
	}
	if err := Set(&document, MiMoAuthModeKey, "bearer"); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(document); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	if fileSystem.replaceOld == "" || fileSystem.replaceNew != path {
		t.Fatalf("atomic replacement evidence = old %q new %q", fileSystem.replaceOld, fileSystem.replaceNew)
	}
	if bytes.Contains(fileSystem.files[path], []byte("api-key")) || bytes.Contains(fileSystem.files[path], []byte("credential")) {
		t.Fatalf("persisted file contained credential material: %s", fileSystem.files[path])
	}
	loaded, err := store.Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if loaded.Hotkey == nil || *loaded.Hotkey != "Win+F8" || loaded.MiMoAuthMode == nil || *loaded.MiMoAuthMode != "bearer" {
		t.Fatalf("loaded document = %+v", loaded)
	}
}

func TestFileStoreFailureDoesNotReplaceDestination(t *testing.T) {
	const path = "fake/VoxInk/config.json"
	fileSystem := newMemoryFS()
	fileSystem.files[path] = []byte(`{"SchemaVersion":1}`)
	fileSystem.syncError = errors.New("sync raw-canary")
	err := NewFileStore(path, fileSystem).Save(EmptyDocument())
	if !errors.Is(err, ErrStorage) || fileSystem.replaceNew != "" {
		t.Fatalf("Save() error/replace = %v/%q", err, fileSystem.replaceNew)
	}
	if strings.Contains(err.Error(), "raw-canary") || strings.Contains(err.Error(), path) {
		t.Fatalf("Save() leaked raw error or path: %q", err)
	}
}

type memoryFS struct {
	files      map[string][]byte
	temporary  int
	replaceOld string
	replaceNew string
	syncError  error
}

func newMemoryFS() *memoryFS { return &memoryFS{files: make(map[string][]byte)} }

func (f *memoryFS) Open(path string) (File, error) {
	body, ok := f.files[path]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return &memoryFile{name: path, buffer: *bytes.NewBuffer(append([]byte(nil), body...))}, nil
}

func (f *memoryFS) CreateTemp(directory, _ string) (File, error) {
	f.temporary++
	name := directory + "/temp"
	return &memoryFile{name: name, owner: f, syncError: f.syncError}, nil
}

func (f *memoryFS) MkdirAll(string, fs.FileMode) error { return nil }
func (f *memoryFS) Replace(oldPath, newPath string) error {
	f.replaceOld, f.replaceNew = oldPath, newPath
	f.files[newPath] = append([]byte(nil), f.files[oldPath]...)
	delete(f.files, oldPath)
	return nil
}
func (f *memoryFS) Remove(path string) error { delete(f.files, path); return nil }

type memoryFile struct {
	name      string
	buffer    bytes.Buffer
	owner     *memoryFS
	syncError error
}

func (f *memoryFile) Read(body []byte) (int, error)  { return f.buffer.Read(body) }
func (f *memoryFile) Write(body []byte) (int, error) { return f.buffer.Write(body) }
func (f *memoryFile) Sync() error                    { return f.syncError }
func (f *memoryFile) Name() string                   { return f.name }
func (f *memoryFile) Chmod(fs.FileMode) error        { return nil }
func (f *memoryFile) Close() error {
	if f.owner != nil {
		f.owner.files[f.name] = append([]byte(nil), f.buffer.Bytes()...)
	}
	return nil
}
