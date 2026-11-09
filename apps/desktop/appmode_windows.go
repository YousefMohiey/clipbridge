//go:build windows

package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// openAppWindow shows the interface in a chromeless app window using the Edge
// or Chrome already on the machine. It is the fallback for a PC where the
// WebView2 runtime will not start: same page, same colours, no address bar and
// no tabs, and a real taskbar entry.
func openAppWindow(url string) bool {
	var candidates []string
	for _, root := range []string{os.Getenv("ProgramFiles(x86)"), os.Getenv("ProgramFiles"), os.Getenv("LocalAppData")} {
		if root == "" {
			continue
		}
		candidates = append(candidates,
			filepath.Join(root, "Microsoft", "Edge", "Application", "msedge.exe"),
			filepath.Join(root, "Google", "Chrome", "Application", "chrome.exe"),
		)
	}

	for _, exe := range candidates {
		if _, err := os.Stat(exe); err != nil {
			continue
		}
		cmd := exec.Command(exe,
			"--app="+url,
			"--window-size=900,740",
			"--user-data-dir="+filepath.Join(configDir(), "browser"),
			"--window-position=220,120",
			"--no-first-run",
			"--no-default-browser-check",
			"--disable-features=Translate",
		)
		if err := cmd.Start(); err == nil {
			logf("fallback window launched with %s", exe)
			return true
		}
	}
	return false
}
