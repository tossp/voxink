//go:build !windows || !cgo

package windows

import (
	"io"

	"github.com/tossp/voxink/internal/credential"
)

// NewCredentialStore reports credential storage as unsupported for this build.
func NewCredentialStore() credential.Store { return nil }

// ReadCredentialValue retains injectable stdin behavior on unsupported builds.
func ReadCredentialValue(input io.Reader) ([]byte, error) { return credential.ReadValue(input) }
