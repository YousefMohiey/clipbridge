//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/jchv/go-webview2"
)

//go:embed icon.ico
var iconBytes []byte

// startNative opens the real app window. WebView2 draws the same page the
// Linux build serves, so both platforms share one design and one code path.
func startNative(e *Engine, cfg *Config, url string) (ok bool) {
	// The window must own the thread it was created on.
	runtime.LockOSThread()

	// A crash inside the window must not take the sync down with it.
	defer func() {
		if r := recover(); r != nil {
			logf("window failed: %v\n%s", r, debug.Stack())
			messageBox("ClipBridge", fmt.Sprintf("The window crashed: %v\n\nSync keeps running. Details are in %s\\app.log", r, configDir()))
			ok = false
		}
	}()

	w := webview2.NewWithOptions(webview2.WebViewOptions{
		Debug:     false,
		AutoFocus: true,
		WindowOptions: webview2.WindowOptions{
			Title:  "ClipBridge " + appVersion,
			Width:  720,
			Height: 690,
			Center: true,
		},
	})
	if w == nil {
		logf("webview2 runtime is missing")
		messageBox("ClipBridge", "The window needs the WebView2 runtime, which this PC does not have.\n\nInstall it from Microsoft once, then start ClipBridge again. Sync keeps running meanwhile. Details are in "+configDir()+"\\app.log")
		return false
	}
	defer w.Destroy()

	logf("window ready")
	w.Navigate(url)
	startTray(e)
	w.Run()
	return true
}
