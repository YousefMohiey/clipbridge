package main

import (
	"bytes"
	"os"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

type Clip struct {
	When    time.Time `json:"when"`
	Kind    string    `json:"kind"`
	Origin  string    `json:"origin"`
	Snippet string    `json:"snippet"`
	Text    string    `json:"-"`
	PNG     []byte    `json:"-"`
}

type remoteState struct {
	Rev  int    `json:"rev"`
	Type string `json:"type"`
	Hash string `json:"hash"`
	Text string `json:"text"`
	Size int    `json:"size"`
}

type Engine struct {
	mu sync.Mutex

	Server     string
	Paused     bool
	SyncText   bool
	SyncImages bool

	connected bool
	message   string
	lastHash  string
	lastRev   int
	history   []Clip
	busy      bool

	hosting  bool
	host     *Host
	hostAddr string

	client *http.Client
}

func NewEngine(server string) *Engine {
	return &Engine{
		Server:     server,
		SyncText:   true,
		SyncImages: true,
		message:    "Starting",
		client:     &http.Client{Timeout: 12 * time.Second},
	}
}

func (e *Engine) SetServer(url string) {
	e.mu.Lock()
	e.Server = strings.TrimRight(strings.TrimSpace(url), "/")
	e.lastRev = -1
	e.lastHash = ""
	e.mu.Unlock()
}

func (e *Engine) snapshot() (bool, string, string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.connected, e.message, e.Server
}

func (e *Engine) SnapshotJSON() []byte {
	e.mu.Lock()
	hist := make([]Clip, len(e.history))
	copy(hist, e.history)
	out := map[string]any{
		"connected":  e.connected,
		"message":    e.message,
		"server":     e.Server,
		"paused":     e.Paused,
		"syncText":   e.SyncText,
		"syncImages": e.SyncImages,
		"hosting":    e.hosting,
		"hostAddr":   e.hostAddr,
		"version":    "ClipBridge " + appVersion,
		"device":     deviceName(),
		"history":    hist,
	}
	e.mu.Unlock()
	b, _ := json.Marshal(out)
	return b
}

// StartHost makes this device the clipboard server for the network.
func (e *Engine) StartHost() error {
	h := NewHost()
	h.OnClip = func(kind, text string, png []byte, fromRemote bool) {
		origin := "this device"
		if fromRemote {
			origin = "guest"
		}
		body := text
		if kind == "image" {
			body = sizeText(len(png))
		}
		e.remember(Clip{When: time.Now(), Kind: kind, Origin: origin,
			Snippet: snippet(body), Text: text, PNG: png})
	}
	if err := h.Start(); err != nil {
		return err
	}
	e.mu.Lock()
	e.host = h
	e.hosting = true
	e.hostAddr = h.Addr()
	e.connected = true
	e.message = "Hosting"
	e.mu.Unlock()
	return nil
}

// StopHost stops serving; the device goes back to being a client.
func (e *Engine) StopHost() {
	e.mu.Lock()
	h := e.host
	e.host = nil
	e.hosting = false
	e.hostAddr = ""
	e.connected = false
	e.message = "Stopped hosting"
	e.mu.Unlock()
	if h != nil {
		h.Stop()
	}
}

func hashOf(kind string, png []byte, text string) string {
	h := sha256.New()
	h.Write([]byte(kind))
	if png != nil {
		h.Write(png)
	} else {
		h.Write([]byte(text))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func snippet(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 90 {
		return s[:87] + "..."
	}
	return s
}

func (e *Engine) remember(c Clip) {
	e.mu.Lock()
	e.history = append([]Clip{c}, e.history...)
	if len(e.history) > 40 {
		e.history = e.history[:40]
	}
	e.mu.Unlock()
}

func (e *Engine) Loop(stop <-chan struct{}) {
	tick := time.NewTicker(600 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-stop:
			return
		case <-tick.C:
			e.poll()
		}
	}
}

func (e *Engine) poll() {
	e.mu.Lock()
	if e.busy || e.Paused {
		e.mu.Unlock()
		return
	}
	if e.hosting {
		addr := e.hostAddr
		e.mu.Unlock()
		e.setStatus(true, "Hosting on "+addr)
		return
	}
	e.busy = true
	server := e.Server
	textOn, imageOn := e.SyncText, e.SyncImages
	e.mu.Unlock()
	defer func() {
		e.mu.Lock()
		e.busy = false
		e.mu.Unlock()
	}()

	// local to host
	if clip := localClip(textOn, imageOn); clip != nil {
		h := hashOf(clip.Kind, clip.PNG, clip.Text)
		e.mu.Lock()
		seen := h == e.lastHash
		e.mu.Unlock()
		if !seen {
			if e.push(server, clip) {
				e.mu.Lock()
				e.lastHash = h
				e.mu.Unlock()
				e.remember(*clip)
			}
		}
	}

	// host to local
	st, err := e.getState(server)
	if err != nil {
		e.setStatus(false, "Not connected")
		return
	}
	if st.Type != "" {
		e.mu.Lock()
		fresh := st.Rev != e.lastRev
		e.mu.Unlock()
		if fresh {
			switch st.Type {
			case "image":
				if png, err := e.getImage(server); err == nil && len(png) > 0 {
					h := hashOf("image", png, "")
					e.mu.Lock()
					known := h == e.lastHash
					e.mu.Unlock()
					if !known {
						writePNG(png)
						e.remember(Clip{When: time.Now(), Kind: "image", Origin: "host",
							Snippet: sizeText(len(png)), PNG: png})
						e.mu.Lock()
						e.lastHash = h
						e.mu.Unlock()
					}
				}
			case "text":
				h := hashOf("text", nil, st.Text)
				e.mu.Lock()
				known := h == e.lastHash
				e.mu.Unlock()
				if !known {
					writeText(st.Text)
					e.remember(Clip{When: time.Now(), Kind: "text", Origin: "host",
						Snippet: snippet(st.Text), Text: st.Text})
					e.mu.Lock()
					e.lastHash = h
					e.mu.Unlock()
				}
			}
		}
		e.mu.Lock()
		e.lastRev = st.Rev
		e.mu.Unlock()
	}
	e.setStatus(true, "Connected")
}

func (e *Engine) setStatus(ok bool, msg string) {
	e.mu.Lock()
	e.connected = ok
	e.message = msg
	e.mu.Unlock()
}

func localClip(textOn, imageOn bool) *Clip {
	if imageOn {
		if png := readPNG(); len(png) > 0 {
			return &Clip{When: time.Now(), Kind: "image", Origin: "this PC",
				Snippet: sizeText(len(png)), PNG: png}
		}
	}
	if textOn {
		if t := readText(); t != "" {
			return &Clip{When: time.Now(), Kind: "text", Origin: "this PC",
				Snippet: snippet(t), Text: t}
		}
	}
	return nil
}

func (e *Engine) push(server string, c *Clip) bool {
	var resp *http.Response
	var err error
	if c.Kind == "image" {
		resp, err = e.client.Post(server+"/set", "image/png", bytes.NewReader(c.PNG))
	} else {
		body, _ := json.Marshal(map[string]string{"text": c.Text})
		resp, err = e.client.Post(server+"/set", "application/json", bytes.NewReader(body))
	}
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == 200
}

func (e *Engine) getState(server string) (*remoteState, error) {
	resp, err := e.client.Get(server + "/state")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var st remoteState
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		return nil, err
	}
	return &st, nil
}

func (e *Engine) getImage(server string) ([]byte, error) {
	resp, err := e.client.Get(server + "/image")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	return string(buf[i:])
}

func sizeText(n int) string {
	if n < 1024 {
		return "image " + itoa(n) + " B"
	}
	return "image " + itoa(n/1024) + " KB"
}

// SendCurrent pushes whatever is on the local clipboard right now.
func (e *Engine) SendCurrent() bool {
	e.mu.Lock()
	textOn, imageOn, server := e.SyncText, e.SyncImages, e.Server
	e.mu.Unlock()
	clip := localClip(textOn, imageOn)
	if clip == nil || server == "" {
		return false
	}
	if !e.push(server, clip) {
		return false
	}
	e.mu.Lock()
	e.lastHash = hashOf(clip.Kind, clip.PNG, clip.Text)
	e.mu.Unlock()
	e.remember(*clip)
	return true
}

// deviceName is what the rail calls this computer.
func deviceName() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "This device"
}
