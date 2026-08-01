//go:build windows && cgo

package windows

import (
	"testing"
	"unsafe"
)

func TestWindowsCredentialABI(t *testing.T) {
	wantSize := uintptr(52)
	wantBlobOffset := uintptr(28)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		wantSize = 80
		wantBlobOffset = 40
	}
	var value windowsCredential
	if got := unsafe.Sizeof(value); got != wantSize {
		t.Fatalf("windowsCredential size = %d, want %d", got, wantSize)
	}
	if got := unsafe.Offsetof(value.credentialBlob); got != wantBlobOffset {
		t.Fatalf("credentialBlob offset = %d, want %d", got, wantBlobOffset)
	}
	for name, proc := range map[string]uintptr{
		"CredReadW": procCredRead.Addr(), "CredWriteW": procCredWrite.Addr(),
		"CredDeleteW": procCredDelete.Addr(), "CredFree": procCredFree.Addr(),
	} {
		if proc == 0 {
			t.Fatalf("%s address is zero", name)
		}
	}
}
