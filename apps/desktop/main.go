package main

import (
	_ "embed"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
)

//go:embed app.html
var uiHTML []byte

const appVersion = "0.11.0"

var (
	flagPort   = flag.Int("port", clipPort, "port this device serves its clipboard on")
	flagUI     = flag.Int("ui", 1770, "local port for the window")
	flagOpen   = flag.Bool("open", false, "open the window when the app starts")
	flagHidden = flag.Bool("hidden", false, "start without opening the window")
	flagTray   = flag.Bool("trayonly", false, "start only the notification icon, used for testing")
	flagPeer    = flag.String("peer", "", "connect to this address at startup, for testing")
	flagProfile = flag.String("profile", "", "run beside the main copy with its own settings, for testing")
)

func openBrowser(url string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		_ = exec.Command("open", url).Start()
	default:
		_ = exec.Command("xdg-open", url).Start()
	}
}

func main() {
	flag.Parse()
	profileSuffix = *flagProfile

	if !acquireInstance() {
		// Already running: bring the existing window forward instead of
		// doing nothing, which is what a second click on the icon expects.
		if port := readUIPort(); port > 0 {
			url := fmt.Sprintf("http://127.0.0.1:%d", port)
			if !openAppWindow(url) {
				openBrowser(url)
			}
			return
		}
		messageBox("ClipBridge", "ClipBridge is already running. Look for its icon in the notification area.")
		return
	}
	logf("ClipBridge %s starting", appVersion)

	cfg := LoadConfig()
	e := NewEngine(*flagPort)
	e.SyncText, e.SyncImages = cfg.SyncText, cfg.SyncImages
	for _, addr := range cfg.Peers {
		e.Connect(addr)
	}
	if *flagPeer != "" {
		e.Connect(*flagPeer)
	}

	if err := e.serve(); err != nil {
		logf("cannot serve the clipboard on port %d: %v", e.Port, err)
		messageBox("ClipBridge", fmt.Sprintf("Port %d is already in use, so other devices cannot reach this one.\n\n%s", e.Port, err))
	}
	logf("serving on %s", e.SelfAddr())

	go e.announce()
	go e.discover()
	go e.Loop()

	if *flagTray {
		startTray(e)
		select {}
	}

	mux := newMux(e, &cfg)
	port, err := serveUI(mux, *flagUI)
	if err != nil {
		logf("cannot open the window: %v", err)
		return
	}
	if port != *flagUI {
		logf("port %d was taken, window moved to %d", *flagUI, port)
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", port)
	writeUIPort(port)
	logf("window on %s", url)

	prepareWindow()
	if startNative(e, &cfg, url) {
		return
	}

	if *flagOpen && !*flagHidden {
		openBrowser(url)
	}

	select {}
}

// serveUI binds the window to the first free port, so a busy port cannot
// leave the app running with no window at all.
func serveUI(mux *http.ServeMux, want int) (int, error) {
	for p := want; p < want+40; p++ {
		if restrictedPort(p) {
			continue
		}
		ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", p))
		if err != nil {
			continue
		}
		go http.Serve(ln, mux)
		return p, nil
	}
	return 0, fmt.Errorf("no free port near %d", want)
}

// restrictedPort lists the ports Chrome and Edge refuse to load, which would
// show the user ERR_UNSAFE_PORT instead of the app window.
func restrictedPort(p int) bool {
	switch p {
	case 1719, 1720, 1723, 2049, 3659, 4045, 5060, 5061, 6000, 6566, 6697, 10080:
		return true
	}
	return p >= 6665 && p <= 6669
}

// newMux builds the window's own API. The window talks to the app over
// loopback, so the same page works in WebView2 and in a browser.
func newMux(e *Engine, cfg *Config) *http.ServeMux {
	mux := http.NewServeMux()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(uiHTML)
	})

	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(e.SnapshotJSON())
	})

	// connect and disconnect are the whole point: a device is listed, you
	// link it, and from then on clips travel both ways.
	mux.HandleFunc("/api/connect", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Addr string `json:"addr"`
			Name string `json:"name"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		addr := strings.TrimSpace(body.Addr)
		if addr == "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		e.Connect(addr)
		savePeers(e, cfg)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/disconnect", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Addr string `json:"addr"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		e.Disconnect(strings.TrimSpace(body.Addr))
		savePeers(e, cfg)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/toggle", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Name string `json:"name"`
			On   bool   `json:"on"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		switch body.Name {
		case "text":
			e.mu.Lock()
			e.SyncText = body.On
			e.mu.Unlock()
			cfg.SyncText = body.On
		case "images":
			e.mu.Lock()
			e.SyncImages = body.On
			e.mu.Unlock()
			cfg.SyncImages = body.On
		case "pause":
			e.mu.Lock()
			e.Paused = body.On
			e.mu.Unlock()
		case "boot":
			SetStartOnBoot(body.On)
			cfg.StartOnBoot = body.On
		}
		SaveConfig(*cfg)
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		if e.SendCurrent() == 0 {
			w.WriteHeader(http.StatusConflict)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	mux.HandleFunc("/api/copy", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Index int `json:"index"`
		}
		json.NewDecoder(r.Body).Decode(&body)
		if !e.CopyAt(body.Index) {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	return mux
}

func savePeers(e *Engine, cfg *Config) {
	cfg.Peers = e.ConnectedAddrs()
	SaveConfig(*cfg)
}
