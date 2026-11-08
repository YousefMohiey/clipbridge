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
	"time"
)

//go:embed ui.html
var uiHTML []byte

var (
	flagServer = flag.String("server", "", "host address, for example http://192.168.1.8:1717")
	flagUI     = flag.Int("ui", 1718, "local port for the window")
	flagOpen   = flag.Bool("open", true, "open the window in the browser")
	flagHidden = flag.Bool("hidden", false, "start without opening the window")
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

// discover listens for the host announcement and adopts the first one heard.
func discover(e *Engine, stop <-chan struct{}) {
	addr, err := net.ResolveUDPAddr("udp4", ":1719")
	if err != nil {
		return
	}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return
	}
	defer conn.Close()

	buf := make([]byte, 2048)
	for {
		select {
		case <-stop:
			return
		default:
		}
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil {
			continue
		}
		var msg struct {
			App  string `json:"app"`
			Port int    `json:"port"`
		}
		if json.Unmarshal(buf[:n], &msg) != nil || msg.App != "clipbridge" || msg.Port == 0 {
			continue
		}
		url := fmt.Sprintf("http://%s:%d", from.IP.String(), msg.Port)
		if ok, msgText, current := e.snapshot(); ok && current != "" {
			_ = msgText
			continue
		}
		e.SetServer(url)
	}
}

func main() {
	flag.Parse()

	e := NewEngine(*flagServer)
	cfg := LoadConfig()
	server := *flagServer
	if server == "" {
		server = cfg.Server
	}
	if server != "" {
		e.SetServer(server)
	}
	e.SyncText = cfg.SyncText
	e.SyncImages = cfg.SyncImages
	if cfg.Host {
		if err := e.StartHost(); err != nil {
			e.setStatus(false, "Cannot host: "+err.Error())
		}
	}

	stop := make(chan struct{})
	go e.Loop(stop)
	go discover(e, stop)

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(uiHTML)
	})
	mux.HandleFunc("/api/state", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(e.SnapshotJSON())
	})
	mux.HandleFunc("/api/server", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Server string `json:"server"` }
		json.NewDecoder(r.Body).Decode(&body)
		if s := strings.TrimSpace(body.Server); s != "" {
			if !strings.HasPrefix(s, "http") {
				s = "http://" + s
			}
			e.SetServer(s + "/")
			e.SetServer(s)
		}
		w.WriteHeader(200)
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
		case "host":
			if body.On {
				if err := e.StartHost(); err != nil {
					e.setStatus(false, "Cannot host: "+err.Error())
					w.WriteHeader(500)
					return
				}
				cfg.Host = true
			} else {
				e.StopHost()
				cfg.Host = false
			}
		case "boot":
			SetStartOnBoot(body.On)
			cfg.StartOnBoot = body.On
		}
		SaveConfig(cfg)
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/send", func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		textOn, imageOn, server := e.SyncText, e.SyncImages, e.Server
		e.mu.Unlock()
		clip := localClip(textOn, imageOn)
		if clip == nil || !e.push(server, clip) {
			w.WriteHeader(409)
			return
		}
		e.mu.Lock()
		e.lastHash = hashOf(clip.Kind, clip.PNG, clip.Text)
		e.mu.Unlock()
		e.remember(*clip)
		w.WriteHeader(200)
	})
	mux.HandleFunc("/api/copy", func(w http.ResponseWriter, r *http.Request) {
		var body struct{ Index int `json:"index"` }
		json.NewDecoder(r.Body).Decode(&body)
		e.mu.Lock()
		var pick *Clip
		if body.Index >= 0 && body.Index < len(e.history) {
			c := e.history[body.Index]
			pick = &c
		}
		e.mu.Unlock()
		if pick == nil {
			w.WriteHeader(404)
			return
		}
		if pick.Kind == "image" {
			writePNG(pick.PNG)
		} else {
			writeText(pick.Text)
		}
		w.WriteHeader(200)
	})

	addr := fmt.Sprintf("127.0.0.1:%d", *flagUI)
	go http.ListenAndServe(addr, mux)

	if *flagOpen && !*flagHidden {
		openBrowser("http://" + addr)
	}

	select {}
}
