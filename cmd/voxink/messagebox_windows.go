//go:build windows

package main

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

const messageBoxIconError = 0x00000010

var (
	messageBoxUser32 = windows.NewLazySystemDLL("user32.dll")
	procMessageBoxW  = messageBoxUser32.NewProc("MessageBoxW")
)

func showWindowsMessage(message string) {
	text, err := windows.UTF16PtrFromString(message)
	if err != nil {
		return
	}
	title, err := windows.UTF16PtrFromString("VoxInk")
	if err != nil {
		return
	}
	procMessageBoxW.Call(
		0,
		uintptr(unsafe.Pointer(text)),
		uintptr(unsafe.Pointer(title)),
		messageBoxIconError,
	)
}
