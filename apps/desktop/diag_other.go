//go:build !windows

package main

// prepareWindow is Windows only; the other platforms serve the page directly.
func prepareWindow() {}
