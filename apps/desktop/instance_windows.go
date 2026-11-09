//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// kernel32 is declared in clip_windows.go; this file only needs one more call.
var procCreateMtx = kernel32.NewProc("CreateMutexW")

// acquireInstance takes a named mutex so a second copy of the app cannot run
// alongside the first and fight it over the clipboard.
func acquireInstance() bool {
	name, err := syscall.UTF16PtrFromString("Local\\ClipBridge.SingleInstance")
	if err != nil {
		return true
	}
	h, _, callErr := procCreateMtx.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true
	}
	// ERROR_ALREADY_EXISTS
	if errno, ok := callErr.(syscall.Errno); ok && errno == 183 {
		return false
	}
	return true
}
