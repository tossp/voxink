//go:build windows

package windows

import (
	"testing"
	"unsafe"
)

func TestWinInputLayoutMatchesWindowsABI(t *testing.T) {
	want := uintptr(28)
	if unsafe.Sizeof(uintptr(0)) == 8 {
		want = 40
	}
	if got := unsafe.Sizeof(winInput{}); got != want {
		t.Fatalf("sizeof(INPUT) = %d, want %d", got, want)
	}
}
