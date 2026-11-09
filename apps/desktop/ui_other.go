//go:build !windows && !linux

package main

// startNative reports whether a native window took over. Only Windows has one;
// the other platforms keep the local web window.
func startNative(e *Engine, cfg *Config, url string) bool { return false }

func messageBox(title, text string) {}

func startTray(e *Engine) {}

// openAppWindow has no app mode on this platform.
func openAppWindow(url string) bool { return false }
