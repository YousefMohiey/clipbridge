//go:build !windows

package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// acquireInstance takes an exclusive lock so a second copy of the app cannot
// run alongside the first and fight it over the clipboard.
func acquireInstance() bool {
	path := filepath.Join(configDir(), "instance.lock")
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return true
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return false
	}
	// Deliberately never closed: the lock lives as long as the process.
	return true
}
