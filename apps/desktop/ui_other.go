//go:build !windows

package main

// startNative reports whether a native window took over. Only Windows has one;
// the other platforms keep the local web window.
func startNative(e *Engine, cfg *Config) bool { return false }

func messageBox(title, text string) {}
