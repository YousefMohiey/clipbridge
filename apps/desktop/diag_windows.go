//go:build windows

package main

import (
	"os"
	"path/filepath"

	"golang.org/x/sys/windows/registry"
)

const webviewClient = `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`

// prepareWindow points WebView2 at a user data folder this app owns, so a
// read-only install location cannot stop the window from starting, and writes
// the runtime version to the log for diagnosis.
func prepareWindow() {
	dir := filepath.Join(configDir(), "webview2")
	_ = os.MkdirAll(dir, 0o755)
	_ = os.Setenv("WEBVIEW2_USER_DATA_FOLDER", dir)
	logf("webview2 runtime: %s", webviewRuntimeVersion())
}

func webviewRuntimeVersion() string {
	paths := []string{webviewClient, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`}
	for _, root := range []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER} {
		for _, path := range paths {
			k, err := registry.OpenKey(root, path, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			v, _, err := k.GetStringValue("pv")
			k.Close()
			if err == nil && v != "" {
				return v
			}
		}
	}
	return "not installed"
}
