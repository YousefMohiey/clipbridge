package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// Host serves the clipboard to other devices on the LAN. It is the only
// component that owns shared state, so the whole protocol stays in one place.
type Host struct {
	mu   sync.Mutex
	rev  int
	kind string
	hash string
	text string
	png  []byte

	// lastHash is the content this host itself put on the clipboard, so the
	// watcher does not treat a clip from a client as a new local clip.
	lastHash string

	server *http.Server
	stop   chan struct{}
	addr   string

	// OnClip is called whenever the shared clipboard changes, so the window
	// can show a history without knowing about the transport.
	OnClip func(kind, text string, png []byte, fromRemote bool)
}

func NewHost() *Host {
	return &Host{stop: make(chan struct{})}
}

func (h *Host) Addr() string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.addr
}

func (h *Host) set(kind string, text string, png []byte, fromRemote bool) {
	h.mu.Lock()
	h.rev++
	h.kind = kind
	h.text = text
	h.png = png
	h.hash = hashOf(kind, png, text)
	if fromRemote {
		h.lastHash = h.hash
	}
	h.mu.Unlock()
	if h.OnClip != nil {
		h.OnClip(kind, text, png, fromRemote)
	}
}

func (h *Host) Start() error {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true})
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		out := map[string]any{"rev": h.rev, "type": h.kind, "hash": h.hash}
		if h.kind == "image" {
			out["size"] = len(h.png)
		} else {
			out["text"] = h.text
		}
		h.mu.Unlock()
		writeJSON(w, out)
	})
	mux.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		png := h.png
		kind := h.kind
		h.mu.Unlock()
		if kind != "image" || len(png) == 0 {
			http.Error(w, "no image", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	})
	mux.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		ctype := r.Header.Get("Content-Type")
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "bad body", http.StatusBadRequest)
			return
		}
		if len(ctype) >= 5 && ctype[:5] == "image" {
			if !writePNG(body) {
				http.Error(w, "cannot set image", http.StatusInternalServerError)
				return
			}
			h.set("image", "", body, true)
		} else {
			var payload struct{ Text string `json:"text"` }
			if err := json.Unmarshal(body, &payload); err != nil {
				http.Error(w, "bad json", http.StatusBadRequest)
				return
			}
			if !writeText(payload.Text) {
				http.Error(w, "cannot set text", http.StatusInternalServerError)
				return
			}
			h.set("text", payload.Text, nil, true)
		}
		writeJSON(w, map[string]any{"ok": true})
	})

	ln, err := net.Listen("tcp", ":1717")
	if err != nil {
		return err
	}
	h.server = &http.Server{Handler: mux}
	go h.server.Serve(ln)

	h.mu.Lock()
	h.addr = localIP() + ":1717"
	h.mu.Unlock()

	go h.announce()
	go h.watch()
	return nil
}

func (h *Host) Stop() {
	close(h.stop)
	if h.server != nil {
		h.server.Close()
	}
}

func (h *Host) announce() {
	conn, err := net.DialUDP("udp4", nil, &net.UDPAddr{IP: net.IPv4bcast, Port: 1719})
	if err != nil {
		return
	}
	defer conn.Close()
	payload, _ := json.Marshal(map[string]any{"app": "clipbridge", "port": 1717})
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-tick.C:
			conn.Write(payload)
		}
	}
}

// watch turns local clipboard changes into host state.
func (h *Host) watch() {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-h.stop:
			return
		case <-tick.C:
			var kind, text string
			var png []byte
			if b := readPNG(); len(b) > 0 {
				kind, png = "image", b
			} else if t := readText(); t != "" {
				kind, text = "text", t
			} else {
				continue
			}
			hash := hashOf(kind, png, text)
			h.mu.Lock()
			seen := hash == h.hash || hash == h.lastHash
			h.mu.Unlock()
			if !seen {
				h.set(kind, text, png, false)
			}
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func localIP() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	addr, ok := conn.LocalAddr().(*net.UDPAddr)
	if !ok {
		return "127.0.0.1"
	}
	return addr.IP.String()
}

func portString(p int) string { return strconv.Itoa(p) }
