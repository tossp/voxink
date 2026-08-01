// Package settings persists and resolves VoxInk's fixed non-sensitive settings.
package settings

import (
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	platformwindows "github.com/tossp/voxink/internal/platform/windows"
	"github.com/tossp/voxink/internal/provider/mimo"
	"github.com/tossp/voxink/internal/provider/volcengine"
)

const (
	// SchemaVersion is the only supported persisted settings schema.
	SchemaVersion = 1
	// DefaultVolcReadLimitBytes is the built-in defensive WebSocket read cap.
	DefaultVolcReadLimitBytes int64 = 1024 * 1024
	// MinimumVolcReadLimitBytes is the smallest accepted runtime cap.
	MinimumVolcReadLimitBytes int64 = 64 * 1024
	// MaximumVolcReadLimitBytes is the largest accepted runtime cap.
	MaximumVolcReadLimitBytes int64 = 64 * 1024 * 1024
)

// Key is one fixed non-sensitive setting name.
type Key string

const (
	HotkeyKey             Key = "hotkey"
	VolcEndpointKey       Key = "volc-endpoint"
	VolcReadLimitBytesKey Key = "volc-read-limit-bytes"
	MiMoEndpointKey       Key = "mimo-endpoint"
	MiMoAuthModeKey       Key = "mimo-auth-mode"
)

var (
	// ErrInvalidValue reports a setting value outside its fixed validation rules.
	ErrInvalidValue = errors.New("invalid settings value")
	allKeys         = []Key{HotkeyKey, VolcEndpointKey, VolcReadLimitBytesKey, MiMoEndpointKey, MiMoAuthModeKey}
)

// Document is the complete persisted schema. Pointers distinguish an explicit
// setting from environment/default fallback without sentinel values.
type Document struct {
	SchemaVersion      int
	Hotkey             *string `json:",omitzero"`
	VolcEndpoint       *string `json:",omitzero"`
	VolcReadLimitBytes *int64  `json:",omitzero"`
	MiMoEndpoint       *string `json:",omitzero"`
	MiMoAuthMode       *string `json:",omitzero"`
}

// Effective contains the five validated values after settings > env > default resolution.
type Effective struct {
	Hotkey             platformwindows.Hotkey
	VolcEndpoint       string
	VolcReadLimitBytes int64
	MiMoEndpoint       string
	MiMoAuthMode       mimo.AuthMode
}

// Loader is the read-only settings boundary used by startup and smoke paths.
type Loader interface {
	Load() (Document, error)
}

// Repository is the persistence boundary used by the settings CLI.
type Repository interface {
	Loader
	Save(Document) error
}

// Keys returns the fixed CLI order.
func Keys() []Key { return append([]Key(nil), allKeys...) }

// ParseKey accepts only one of the five supported names.
func ParseKey(raw string) (Key, bool) {
	key := Key(raw)
	for _, allowed := range allKeys {
		if key == allowed {
			return key, true
		}
	}
	return "", false
}

// EmptyDocument returns a schema-valid document with no persisted overrides.
func EmptyDocument() Document { return Document{SchemaVersion: SchemaVersion} }

// Load returns an empty document for a nil loader and otherwise preserves fixed loader errors.
func Load(loader Loader) (Document, error) {
	if loader == nil {
		return EmptyDocument(), nil
	}
	return loader.Load()
}

// Set validates and stores one canonical value in document.
func Set(document *Document, key Key, raw string) error {
	if document == nil {
		return ErrInvalidValue
	}
	document.SchemaVersion = SchemaVersion
	switch key {
	case HotkeyKey:
		hotkey, err := platformwindows.ParseHotkey(raw)
		if err != nil {
			return ErrInvalidValue
		}
		value := hotkey.String()
		document.Hotkey = &value
	case VolcEndpointKey:
		value, err := endpoint(raw, "wss")
		if err != nil {
			return ErrInvalidValue
		}
		document.VolcEndpoint = &value
	case VolcReadLimitBytesKey:
		value, err := readLimit(raw)
		if err != nil {
			return ErrInvalidValue
		}
		document.VolcReadLimitBytes = &value
	case MiMoEndpointKey:
		value, err := endpoint(raw, "https")
		if err != nil {
			return ErrInvalidValue
		}
		document.MiMoEndpoint = &value
	case MiMoAuthModeKey:
		value := strings.ToLower(strings.TrimSpace(raw))
		if value != string(mimo.AuthAPIKey) && value != string(mimo.AuthBearer) {
			return ErrInvalidValue
		}
		document.MiMoAuthMode = &value
	default:
		return ErrInvalidValue
	}
	return nil
}

// Delete removes one persisted override. Resolution then falls back to env/default.
func Delete(document *Document, key Key) {
	switch key {
	case HotkeyKey:
		document.Hotkey = nil
	case VolcEndpointKey:
		document.VolcEndpoint = nil
	case VolcReadLimitBytesKey:
		document.VolcReadLimitBytes = nil
	case MiMoEndpointKey:
		document.MiMoEndpoint = nil
	case MiMoAuthModeKey:
		document.MiMoAuthMode = nil
	}
}

// Resolve validates all five effective values.
func Resolve(document Document, getenv func(string) string) (Effective, error) {
	hotkey, err := resolveHotkey(document.Hotkey, getenv)
	if err != nil {
		return Effective{}, err
	}
	volcEndpoint, readLimitBytes, err := ResolveVolc(document, getenv)
	if err != nil {
		return Effective{}, err
	}
	mimoEndpoint, authMode, err := ResolveMiMo(document, getenv)
	if err != nil {
		return Effective{}, err
	}
	return Effective{
		Hotkey: hotkey, VolcEndpoint: volcEndpoint, VolcReadLimitBytes: readLimitBytes,
		MiMoEndpoint: mimoEndpoint, MiMoAuthMode: authMode,
	}, nil
}

// ResolveVolc resolves only Volcengine's non-sensitive settings.
func ResolveVolc(document Document, getenv func(string) string) (string, int64, error) {
	rawEndpoint := choose(document.VolcEndpoint, getenv, "VOXINK_VOLC_ENDPOINT", volcengine.DefaultEndpoint)
	volcEndpoint, err := endpoint(rawEndpoint, "wss")
	if err != nil {
		return "", 0, fmt.Errorf("invalid Volcengine endpoint setting")
	}
	rawLimit := chooseInt(document.VolcReadLimitBytes, getenv, "VOXINK_VOLC_READ_LIMIT_BYTES", DefaultVolcReadLimitBytes)
	limit, err := readLimit(rawLimit)
	if err != nil {
		return "", 0, fmt.Errorf("invalid Volcengine read limit setting")
	}
	return volcEndpoint, limit, nil
}

// ResolveMiMo resolves only MiMo's non-sensitive settings.
func ResolveMiMo(document Document, getenv func(string) string) (string, mimo.AuthMode, error) {
	rawEndpoint := choose(document.MiMoEndpoint, getenv, "VOXINK_MIMO_ENDPOINT", mimo.DefaultEndpoint)
	mimoEndpoint, err := endpoint(rawEndpoint, "https")
	if err != nil {
		return "", "", fmt.Errorf("invalid MiMo endpoint setting")
	}
	rawMode := strings.ToLower(choose(document.MiMoAuthMode, getenv, "VOXINK_MIMO_AUTH_MODE", string(mimo.AuthAPIKey)))
	mode := mimo.AuthMode(rawMode)
	if mode != mimo.AuthAPIKey && mode != mimo.AuthBearer {
		return "", "", fmt.Errorf("invalid MiMo authentication mode setting")
	}
	return mimoEndpoint, mode, nil
}

func resolveHotkey(value *string, getenv func(string) string) (platformwindows.Hotkey, error) {
	raw := choose(value, getenv, "VOXINK_HOTKEY", platformwindows.DefaultHotkey)
	hotkey, err := platformwindows.ParseHotkey(raw)
	if err != nil {
		return platformwindows.Hotkey{}, fmt.Errorf("invalid hotkey setting")
	}
	return hotkey, nil
}

func choose(setting *string, getenv func(string) string, environment, fallback string) string {
	if setting != nil {
		return *setting
	}
	if getenv != nil {
		if value := strings.TrimSpace(getenv(environment)); value != "" {
			return value
		}
	}
	return fallback
}

func chooseInt(setting *int64, getenv func(string) string, environment string, fallback int64) string {
	if setting != nil {
		return strconv.FormatInt(*setting, 10)
	}
	if getenv != nil {
		if value := strings.TrimSpace(getenv(environment)); value != "" {
			return value
		}
	}
	return strconv.FormatInt(fallback, 10)
}

func endpoint(raw, scheme string) (string, error) {
	value := strings.TrimSpace(raw)
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != scheme || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Opaque != "" {
		return "", ErrInvalidValue
	}
	return value, nil
}

func readLimit(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value < MinimumVolcReadLimitBytes || value > MaximumVolcReadLimitBytes {
		return 0, ErrInvalidValue
	}
	return value, nil
}
