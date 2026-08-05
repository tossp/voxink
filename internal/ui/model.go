// Package ui owns the Fyne desktop UI and its testable presentation model.
package ui

import (
	"errors"
	"strconv"
	"sync"

	runtimeapp "github.com/tossp/voxink/internal/app"
	"github.com/tossp/voxink/internal/credential"
	"github.com/tossp/voxink/internal/history"
	"github.com/tossp/voxink/internal/settings"
)

var (
	// ErrLoadFailed is the fixed redacted UI initialization error.
	ErrLoadFailed = errors.New("VoxInk UI data could not be loaded")
	// ErrSaveFailed is the fixed redacted settings save error.
	ErrSaveFailed = errors.New("VoxInk settings could not be saved")
)

// HistoryRepository is the read boundary used to initialize the main list.
type HistoryRepository interface {
	Load() ([]history.Entry, error)
}

// Options supplies persistence and thread-safe runtime callbacks.
type Options struct {
	Events           <-chan runtimeapp.RuntimeEvent
	Settings         settings.Repository
	Credentials      credential.Store
	History          HistoryRepository
	Getenv           func(string) string
	Toggle           func()
	UpdateHotkey     func(string) error
	Saved            func()
	CaptureSupported bool
}

// FormValues is the complete non-secret settings form plus write-only credentials.
type FormValues struct {
	Hotkey             string
	VolcEndpoint       string
	VolcReadLimitBytes string
	MiMoEndpoint       string
	MiMoAuthMode       string
	Credentials        map[credential.Name]string
}

// App owns the testable model and, while Run is active, both Fyne windows.
type App struct {
	options Options

	mu         sync.RWMutex
	status     runtimeapp.RuntimeStatus
	history    []history.Entry
	configured map[credential.Name]bool
	document   settings.Document
	form       FormValues

	widgets desktopWidgets
}

// New loads bounded local state without creating or starting a GUI driver.
func New(options Options) (*App, error) {
	if options.Settings == nil || options.Credentials == nil || options.History == nil {
		return nil, ErrLoadFailed
	}
	document, err := options.Settings.Load()
	if err != nil {
		return nil, ErrLoadFailed
	}
	effective, err := settings.Resolve(document, options.Getenv)
	if err != nil {
		return nil, ErrLoadFailed
	}
	entries, err := options.History.Load()
	if err != nil {
		return nil, ErrLoadFailed
	}
	configured := make(map[credential.Name]bool, len(credential.Names()))
	for _, name := range credential.Names() {
		value, readErr := options.Credentials.Read(name)
		clear(value)
		switch {
		case readErr == nil:
			configured[name] = true
		case errors.Is(readErr, credential.ErrNotFound):
		default:
			return nil, ErrLoadFailed
		}
	}
	return &App{
		options: options, status: runtimeapp.StatusIdle, history: reverseHistory(entries),
		configured: configured, document: document,
		form: FormValues{
			Hotkey: effective.Hotkey.String(), VolcEndpoint: effective.VolcEndpoint,
			VolcReadLimitBytes: strconv.FormatInt(effective.VolcReadLimitBytes, 10),
			MiMoEndpoint:       effective.MiMoEndpoint, MiMoAuthMode: string(effective.MiMoAuthMode),
		},
	}, nil
}

func reverseHistory(entries []history.Entry) []history.Entry {
	reversed := make([]history.Entry, len(entries))
	for index := range entries {
		reversed[len(entries)-1-index] = entries[index]
	}
	return reversed
}

// HandleEvent updates the model directly; Run schedules this method with fyne.Do.
func (a *App) HandleEvent(event runtimeapp.RuntimeEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if event.Status != "" {
		a.status = event.Status
	}
	if event.History != nil {
		a.history = append([]history.Entry{*event.History}, a.history...)
		if len(a.history) > history.MaximumEntries {
			a.history = a.history[:history.MaximumEntries]
		}
	}
}

// Save validates all fields before persisting settings and non-empty credentials.
func (a *App) Save(form FormValues) error {
	a.mu.RLock()
	document := a.document
	a.mu.RUnlock()
	for _, field := range []struct {
		key   settings.Key
		value string
	}{
		{settings.HotkeyKey, form.Hotkey},
		{settings.VolcEndpointKey, form.VolcEndpoint},
		{settings.VolcReadLimitBytesKey, form.VolcReadLimitBytes},
		{settings.MiMoEndpointKey, form.MiMoEndpoint},
		{settings.MiMoAuthModeKey, form.MiMoAuthMode},
	} {
		if settings.Set(&document, field.key, field.value) != nil {
			return ErrSaveFailed
		}
	}
	for _, name := range credential.Names() {
		value := form.Credentials[name]
		if len(value) > credential.MaximumValueBytes {
			return ErrSaveFailed
		}
	}
	if err := a.options.Settings.Save(document); err != nil {
		return ErrSaveFailed
	}
	for _, name := range credential.Names() {
		value := form.Credentials[name]
		if value == "" {
			continue
		}
		secret := []byte(value)
		err := a.options.Credentials.Write(name, secret)
		clear(secret)
		if err != nil {
			return ErrSaveFailed
		}
	}
	effective, err := settings.Resolve(document, nil)
	if err != nil {
		return ErrSaveFailed
	}
	if a.options.UpdateHotkey != nil && a.options.UpdateHotkey(effective.Hotkey.String()) != nil {
		return ErrSaveFailed
	}
	a.mu.Lock()
	a.document = document
	a.form = FormValues{
		Hotkey: effective.Hotkey.String(), VolcEndpoint: effective.VolcEndpoint,
		VolcReadLimitBytes: strconv.FormatInt(effective.VolcReadLimitBytes, 10),
		MiMoEndpoint:       effective.MiMoEndpoint, MiMoAuthMode: string(effective.MiMoAuthMode),
	}
	for name, value := range form.Credentials {
		if value != "" {
			a.configured[name] = true
		}
	}
	a.mu.Unlock()
	if a.options.Saved != nil {
		a.options.Saved()
	}
	return nil
}

// CaptureEnabled reports whether key capture can be armed for the focused entry.
func (a *App) CaptureEnabled(focused bool) bool {
	return a.options.CaptureSupported && focused
}

// Status returns the current presentation state.
func (a *App) Status() runtimeapp.RuntimeStatus {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.status
}

// History returns newest-first entries for the main list.
func (a *App) History() []history.Entry {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return append([]history.Entry(nil), a.history...)
}

// Form returns the current non-secret effective values.
func (a *App) Form() FormValues {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.form
}

// CredentialConfigured reports only whether one fixed credential exists.
func (a *App) CredentialConfigured(name credential.Name) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.configured[name]
}

// Toggle forwards the main-window command through the injected callback.
func (a *App) Toggle() {
	if a.options.Toggle != nil {
		a.options.Toggle()
	}
}
