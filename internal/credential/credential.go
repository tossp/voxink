// Package credential manages the fixed Provider secrets used by VoxInk.
package credential

import (
	"bufio"
	"errors"
	"fmt"
	"io"
)

const (
	// MaximumValueBytes bounds credential input and Windows credential blobs.
	MaximumValueBytes = 2560
	targetPrefix      = "VoxInk/"
)

// Name is one fixed Provider credential name.
type Name string

const (
	VolcAPIKey     Name = "volc-api-key"
	VolcResourceID Name = "volc-resource-id"
	VolcAppKey     Name = "volc-app-key"
	VolcAccessKey  Name = "volc-access-key"
	MiMoAPIKey     Name = "mimo-api-key"
)

var (
	// ErrNotFound reports an unconfigured credential.
	ErrNotFound = errors.New("credential is not configured")
	// ErrStorage reports a credential backend failure without exposing details.
	ErrStorage = errors.New("credential storage failed")
	allNames   = []Name{VolcAPIKey, VolcResourceID, VolcAppKey, VolcAccessKey, MiMoAPIKey}
)

// Store is the minimal secret persistence boundary. Read returns a
// caller-owned buffer so callers can clear it promptly.
type Store interface {
	Read(Name) ([]byte, error)
	Write(Name, []byte) error
	Delete(Name) error
}

// Status is the only credential metadata exposed by list.
type Status struct {
	Name       Name
	Configured bool
}

// Names returns the fixed credential names in CLI display order.
func Names() []Name { return append([]Name(nil), allNames...) }

// ParseName accepts only a fixed credential name.
func ParseName(raw string) (Name, bool) {
	name := Name(raw)
	for _, allowed := range allNames {
		if name == allowed {
			return name, true
		}
	}
	return "", false
}

// Target returns the fixed Windows Credential Manager target for name.
func Target(name Name) string { return targetPrefix + string(name) }

// EnvironmentKey maps a fixed credential name to its compatibility variable.
func EnvironmentKey(name Name) string {
	switch name {
	case VolcAPIKey:
		return "VOXINK_VOLC_API_KEY"
	case VolcResourceID:
		return "VOXINK_VOLC_RESOURCE_ID"
	case VolcAppKey:
		return "VOXINK_VOLC_APP_KEY"
	case VolcAccessKey:
		return "VOXINK_VOLC_ACCESS_KEY"
	case MiMoAPIKey:
		return "VOXINK_MIMO_API_KEY"
	default:
		return ""
	}
}

// ReadValue reads one newline-terminated or EOF-terminated value. It removes
// only the terminal line ending and never trims credential contents.
func ReadValue(input io.Reader) ([]byte, error) {
	reader := bufio.NewReaderSize(input, MaximumValueBytes+2)
	value, err := reader.ReadBytes('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		clear(value)
		return nil, err
	}
	if len(value) > 0 && value[len(value)-1] == '\n' {
		value = value[:len(value)-1]
		if len(value) > 0 && value[len(value)-1] == '\r' {
			value = value[:len(value)-1]
		}
	}
	if len(value) == 0 || len(value) > MaximumValueBytes {
		clear(value)
		return nil, fmt.Errorf("invalid credential input")
	}
	return value, nil
}

// Resolver applies Credential Manager > environment > missing precedence.
type Resolver struct {
	Store  Store
	Getenv func(string) string
}

// Get returns one fixed credential without retaining the store's byte buffer.
func (r Resolver) Get(name Name) (string, error) {
	if r.Store != nil {
		value, err := r.Store.Read(name)
		switch {
		case err == nil:
			defer clear(value)
			return string(value), nil
		case errors.Is(err, ErrNotFound):
		default:
			return "", ErrStorage
		}
	}
	if r.Getenv == nil {
		return "", nil
	}
	return r.Getenv(EnvironmentKey(name)), nil
}
