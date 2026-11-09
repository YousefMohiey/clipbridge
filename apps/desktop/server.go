package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"time"
)

const (
	clipPort     = 1717
	discoverPort = 1719
)

// serve makes this device reachable to the others on the network. Every device
// serves; being reachable is not a mode you switch on.
func (e *Engine) serve() error {
	ln, err := net.Listen("tcp", ":"+strconv.Itoa(e.Port))
	if err != nil {
		return err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "name": e.Name, "id": e.ID})
	})
	mux.HandleFunc("/state", func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		out := map[string]any{"rev": e.rev, "type": e.kind, "hash": e.hash, "name": e.Name, "id": e.ID}
		if e.kind == "image" {
			out["size"] = len(e.png)
		} else {
			out["text"] = e.text
		}
		e.mu.Unlock()
		writeJSON(w, out)
	})
	mux.HandleFunc("/image", func(w http.ResponseWriter, r *http.Request) {
		e.mu.Lock()
		png, kind := e.png, e.kind
		e.mu.Unlock()
		if kind != "image" || len(png) == 0 {
			http.Error(w, "no image", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(png)
	})
	mux.HandleFunc("/set", e.handleSet)

	e.server = &http.Server{Handler: mux}
	go e.server.Serve(ln)

	e.mu.Lock()
	e.self = localIP() + ":" + strconv.Itoa(e.Port)
	e.mu.Unlock()
	return nil
}

// handleSet takes a clip from a peer, puts it on this clipboard, and marks it
// as known before writing, so the watcher does not send it back.
func (e *Engine) handleSet(w http.ResponseWriter, r *http.Request) {
	ctype := r.Header.Get("Content-Type")
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad body", http.StatusBadRequest)
		return
	}

	if len(ctype) >= 5 && ctype[:5] == "image" {
		if len(body) == 0 {
			http.Error(w, "empty image", http.StatusBadRequest)
			return
		}
		e.accept("image", "", body)
		if !writePNG(body) {
			http.Error(w, "cannot set image", http.StatusInternalServerError)
			return
		}
		writeJSON(w, map[string]any{"ok": true})
		return
	}

	var payload struct {
		Text string `json:"text"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	e.accept("text", payload.Text, nil)
	if !writeText(payload.Text) {
		http.Error(w, "cannot set text", http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"ok": true})
}

// accept stores a clip that arrived from another device.
func (e *Engine) accept(kind, text string, png []byte) {
	h := hashOf(kind, png, text)
	e.mu.Lock()
	e.localHash = h
	e.rev++
	e.kind, e.text, e.png, e.hash = kind, text, png, h
	clip := Clip{When: time.Now(), Kind: kind, Origin: "another device"}
	if kind == "image" {
		clip.Snippet = sizeText(len(png))
		clip.PNG = png
	} else {
		clip.Snippet = snippet(text)
		clip.Text = text
	}
	e.history = append([]Clip{clip}, e.history...)
	if len(e.history) > 40 {
		e.history = e.history[:40]
	}
	e.mu.Unlock()
}

// announce tells the network this device exists, so others can list it.
func (e *Engine) announce() {
	conn, err := net.ListenPacket("udp4", ":0")
	if err != nil {
		logf("announce socket: %v", err)
		return
	}
	defer conn.Close()
	if udp, ok := conn.(*net.UDPConn); ok {
		enableBroadcast(udp)
	}

	payload, _ := json.Marshal(map[string]any{
		"app":  "clipbridge",
		"port": e.Port,
		"id":   e.ID,
		"name": e.Name,
	})
	dst := &net.UDPAddr{IP: net.IPv4bcast, Port: discoverPort}
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-tick.C:
			if _, err := conn.WriteTo(payload, dst); err != nil {
				logf("announce: %v", err)
			}
		}
	}
}

// discover listens for other devices and keeps the rail up to date.
func (e *Engine) discover() {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: discoverPort})
	if err != nil {
		logf("discovery listener: %v", err)
		return
	}
	defer conn.Close()

	buf := make([]byte, 2048)
	for {
		select {
		case <-e.stop:
			return
		default:
		}
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		n, from, err := conn.ReadFromUDP(buf)
		if err != nil || n == 0 {
			continue
		}
		var msg struct {
			App  string `json:"app"`
			Port int    `json:"port"`
			ID   string `json:"id"`
			Name string `json:"name"`
		}
		if json.Unmarshal(buf[:n], &msg) != nil || msg.App != "clipbridge" || msg.Port == 0 {
			continue
		}
		if msg.ID != "" && msg.ID == e.ID {
			continue
		}
		addr := from.IP.String() + ":" + strconv.Itoa(msg.Port)
		name := msg.Name
		if name == "" {
			name = from.IP.String()
		}
		e.AddPeer(msg.ID, name, addr)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func platformName() string {
	if isWindows() {
		return "windows"
	}
	return "linux"
}
