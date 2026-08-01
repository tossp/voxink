//go:build windows && cgo

package windows

import (
	"errors"
	"io"
	"os"
	"runtime"
	"unsafe"

	"github.com/tossp/voxink/internal/credential"
	winapi "golang.org/x/sys/windows"
)

const (
	credentialTypeGeneric         = 1
	credentialPersistLocalMachine = 2
)

var (
	advapi32       = winapi.NewLazySystemDLL("advapi32.dll")
	procCredRead   = advapi32.NewProc("CredReadW")
	procCredWrite  = advapi32.NewProc("CredWriteW")
	procCredDelete = advapi32.NewProc("CredDeleteW")
	procCredFree   = advapi32.NewProc("CredFree")
)

type windowsCredential struct {
	flags              uint32
	kind               uint32
	targetName         uintptr
	comment            uintptr
	lastWritten        uint64
	credentialBlobSize uint32
	credentialBlob     *byte
	persist            uint32
	attributeCount     uint32
	attributes         uintptr
	targetAlias        uintptr
	userName           uintptr
}

type credentialStore struct{}

// NewCredentialStore constructs the current-user Windows Credential Manager adapter.
func NewCredentialStore() credential.Store { return credentialStore{} }

func (credentialStore) Read(name credential.Name) ([]byte, error) {
	target, err := winapi.UTF16PtrFromString(credential.Target(name))
	if err != nil {
		return nil, credential.ErrStorage
	}
	var native *windowsCredential
	ok, _, callErr := procCredRead.Call(
		uintptr(unsafe.Pointer(target)),
		credentialTypeGeneric,
		0,
		uintptr(unsafe.Pointer(&native)),
	)
	runtime.KeepAlive(target)
	if ok == 0 {
		if errors.Is(callErr, winapi.ERROR_NOT_FOUND) {
			return nil, credential.ErrNotFound
		}
		return nil, credential.ErrStorage
	}
	if native == nil {
		return nil, credential.ErrStorage
	}
	defer procCredFree.Call(uintptr(unsafe.Pointer(native)))
	if native.credentialBlobSize == 0 || native.credentialBlobSize > credential.MaximumValueBytes || native.credentialBlob == nil {
		return nil, credential.ErrStorage
	}
	blob := unsafe.Slice(native.credentialBlob, int(native.credentialBlobSize))
	return append([]byte(nil), blob...), nil
}

func (credentialStore) Write(name credential.Name, value []byte) error {
	if len(value) == 0 || len(value) > credential.MaximumValueBytes {
		return credential.ErrStorage
	}
	target, err := winapi.UTF16PtrFromString(credential.Target(name))
	if err != nil {
		return credential.ErrStorage
	}
	native := windowsCredential{
		kind:               credentialTypeGeneric,
		targetName:         uintptr(unsafe.Pointer(target)),
		credentialBlobSize: uint32(len(value)),
		credentialBlob:     &value[0],
		persist:            credentialPersistLocalMachine,
	}
	ok, _, _ := procCredWrite.Call(uintptr(unsafe.Pointer(&native)), 0)
	runtime.KeepAlive(target)
	runtime.KeepAlive(value)
	runtime.KeepAlive(native)
	if ok == 0 {
		return credential.ErrStorage
	}
	return nil
}

func (credentialStore) Delete(name credential.Name) error {
	target, err := winapi.UTF16PtrFromString(credential.Target(name))
	if err != nil {
		return credential.ErrStorage
	}
	ok, _, callErr := procCredDelete.Call(uintptr(unsafe.Pointer(target)), credentialTypeGeneric, 0)
	runtime.KeepAlive(target)
	if ok == 0 {
		if errors.Is(callErr, winapi.ERROR_NOT_FOUND) {
			return credential.ErrNotFound
		}
		return credential.ErrStorage
	}
	return nil
}

// ReadCredentialValue reads stdin with console echo disabled when stdin is an
// interactive Windows console. Non-console readers remain injectable in tests.
func ReadCredentialValue(input io.Reader) ([]byte, error) {
	file, interactive := input.(*os.File)
	if !interactive {
		return credential.ReadValue(input)
	}
	handle := winapi.Handle(file.Fd())
	var mode uint32
	if err := winapi.GetConsoleMode(handle, &mode); err != nil {
		return credential.ReadValue(input)
	}
	if err := winapi.SetConsoleMode(handle, mode&^winapi.ENABLE_ECHO_INPUT); err != nil {
		return nil, credential.ErrStorage
	}
	defer winapi.SetConsoleMode(handle, mode)
	return credential.ReadValue(input)
}
