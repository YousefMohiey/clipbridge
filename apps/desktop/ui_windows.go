//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"runtime"
	"runtime/debug"
	"time"

	"github.com/jchv/go-webview2"
)

//go:embed icon.ico
var iconBytes []byte

// startNative opens the real app window. WebView2 draws the same page the
// Linux build serves, so both platforms share one design and one code path.
//
// The window is built on its own locked thread with a watchdog, because the
// WebView2 runtime can take seconds or fail silently on a machine that has no
// runtime or a damaged user data folder. A failed window must never leave the
// app running invisibly: it falls back to an app-mode window at the same page.
func startNative(e *Engine, cfg *Config, url string) (ok bool) {
	defer func() {
		if r := recover(); r != nil {
			logf("window failed: %v\n%s", r, debug.Stack())
			ok = false
		}
	}()

	// The tray comes first, so there is always something to click.
	startTray(e)

	ready := make(chan struct{})
	closed := make(chan struct{})
	failure := make(chan error, 1)

	go func() {
		runtime.LockOSThread()
		defer func() {
			if r := recover(); r != nil {
				logf("webview2 panic: %v\n%s", r, debug.Stack())
				select {
				case failure <- fmt.Errorf("%v", r):
				default:
				}
			}
		}()

		logf("webview2: creating the window")
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			Debug:     false,
			AutoFocus: true,
			WindowOptions: webview2.WindowOptions{
				Title:  "ClipBridge " + appVersion,
				Width:  900,
				Height: 700,
				Center: true,
			},
		})
		if w == nil {
			failure <- fmt.Errorf("the WebView2 runtime did not answer")
			return
		}
		defer w.Destroy()

		logf("webview2: window created, loading the page")
		w.Navigate(url)
		close(ready)
		w.Run()
		close(closed)
		logf("webview2: window closed")
	}()

	select {
	case <-ready:
		logf("webview2: running")
		<-closed // the user closed the window: quit like any desktop app
		return true

	case err := <-failure:
		logf("webview2 unavailable: %v", err)
		if openAppWindow(url) {
			logf("opened the fallback window")
			select {}
		}
		messageBox("ClipBridge",
			"The app window could not start on this PC:\n\n"+err.Error()+
				"\n\nSync keeps running in the background. Details are in "+configDir()+"\\app.log")
		return false

	case <-time.After(10 * time.Second):
		logf("webview2 did not finish starting within 10s")
		if openAppWindow(url) {
			logf("opened the fallback window after the timeout")
			select {}
		}
		messageBox("ClipBridge",
			"The app window did not start within 10 seconds.\n\n"+
				"Sync keeps running in the background. Details are in "+configDir()+"\\app.log")
		return false
	}
}
