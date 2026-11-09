//go:build linux

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// startNative opens the interface in a chromeless app window when a Chromium
// based browser is installed. The process keeps serving either way, so the
// page also stays reachable in a normal browser tab.
func startNative(e *Engine, cfg *Config, url string) bool {
	if *flagHidden {
		return false
	}
	if openAppWindow(url) {
		logf("app window opened")
	} else {
		logf("no chromium based browser found, open %s yourself", url)
	}
	return false
}

// windowPIDPath remembers the browser process that shows our window, so a
// second launch does not stack a duplicate window on top of the first.
func windowPIDPath() string { return filepath.Join(configDir(), "window.pid") }

func windowAlive() bool {
	data, err := os.ReadFile(windowPIDPath())
	if err != nil {
		return false
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil || pid <= 0 {
		return false
	}
	return syscall.Kill(pid, 0) == nil
}

func openAppWindow(url string) bool {
	if windowAlive() {
		logf("app window is already open")
		return true
	}
	for _, name := range []string{
		"chromium", "chromium-browser", "google-chrome", "google-chrome-stable",
		"brave-browser", "brave-browser-stable", "microsoft-edge", "vivaldi-stable",
	} {
		path, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		cmd := exec.Command(path,
			"--app="+url,
			"--window-size=900,740",
			"--window-position=120,90",
			"--class=ClipBridge",
			"--user-data-dir="+filepath.Join(configDir(), "browser"),
			"--no-first-run",
			"--no-default-browser-check",
		)
		if err := cmd.Start(); err == nil {
			_ = os.WriteFile(windowPIDPath(), []byte(strconv.Itoa(cmd.Process.Pid)), 0o644)
			logf("app window started with %s", path)
			return true
		}
	}
	return false
}

// messageBox has no native dialog here, so the message goes to the log.
func messageBox(title, text string) {
	logf("%s: %s", title, strings.ReplaceAll(text, "\n", " "))
}

// startTray has no tray on this platform; the window is the app.
func startTray(e *Engine) {}
