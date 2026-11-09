package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Clip is one row in the window: what moved, when, and from which device.
type Clip struct {
	When    time.Time `json:"when"`
	Kind    string    `json:"kind"`
	Origin  string    `json:"origin"`
	Snippet string    `json:"snippet"`
	Text    string    `json:"-"`
	PNG     []byte    `json:"-"`
}

// Peer is another device on the network. Every device serves, and a link is
// simply an agreement to exchange clips, so there is no host and no guest.
type Peer struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Addr      string `json:"addr"`
	Connected bool   `json:"connected"`
	Reachable bool   `json:"reachable"`

	lastRev  int
	lastSeen time.Time
}

type remoteState struct {
	Rev  int    `json:"rev"`
	Type string `json:"type"`
	Hash string `json:"hash"`
	Text string `json:"text"`
	Size int    `json:"size"`
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Engine owns the clipboard, the peers, and the loop that moves clips between
// them.
type Engine struct {
	mu sync.Mutex

	Name string
	Port int
	ID   string

	peers map[string]*Peer

	rev  int
	kind string
	hash string
	text string
	png  []byte

	// localHash is what this device believes the local clipboard holds, so its
	// own writes and the echo of a peer's clip never travel back.
	localHash string

	Paused     bool
	SyncText   bool
	SyncImages bool

	history []Clip
	client  *http.Client

	server *http.Server
	stop   chan struct{}
	self   string
}

func NewEngine(port int) *Engine {
	return &Engine{
		Name:       deviceName(),
		Port:       port,
		ID:         deviceID(),
		peers:      make(map[string]*Peer),
		SyncText:   true,
		SyncImages: true,
		client:     &http.Client{Timeout: 8 * time.Second},
		stop:       make(chan struct{}),
	}
}

func (e *Engine) SelfAddr() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.self
}

// AddPeer registers a device seen on the network. A remembered address comes
// back connected, so links survive a restart.
func (e *Engine) AddPeer(id, name, addr string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if addr == "" || addr == e.self {
		return
	}
	if p, ok := e.peers[addr]; ok {
		if name != "" {
			p.Name = name
		}
		if id != "" {
			p.ID = id
		}
		p.lastSeen = time.Now()
		p.Reachable = true
		return
	}
	e.peers[addr] = &Peer{ID: id, Name: name, Addr: addr, lastSeen: time.Now()}
}

func (e *Engine) Connect(addr string) {
	addr = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(addr, "http://"), "https://"))
	addr = strings.TrimRight(addr, "/")
	if addr == "" || addr == e.SelfAddr() {
		return
	}
	e.mu.Lock()
	p, ok := e.peers[addr]
	if !ok {
		p = &Peer{ID: addr, Name: addr, Addr: addr}
		e.peers[addr] = p
	}
	p.Connected = true
	p.lastRev = -1
	p.lastSeen = time.Now()
	e.mu.Unlock()
	logf("connected to %s", addr)
}

func (e *Engine) Disconnect(addr string) {
	e.mu.Lock()
	if p, ok := e.peers[addr]; ok {
		p.Connected = false
		p.lastRev = -1
	}
	e.mu.Unlock()
	logf("disconnected from %s", addr)
}

// ConnectedAddrs is what the config saves, so a restart keeps the links.
func (e *Engine) ConnectedAddrs() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]string, 0, len(e.peers))
	for _, p := range e.peers {
		if p.Connected {
			out = append(out, p.Addr)
		}
	}
	return out
}

func (e *Engine) SnapshotJSON() []byte {
	e.mu.Lock()
	hist := make([]Clip, len(e.history))
	copy(hist, e.history)

	peers := make([]Peer, 0, len(e.peers))
	connected := 0
	for _, p := range e.peers {
		fresh := time.Since(p.lastSeen) < 6*time.Second
		if !fresh && !p.Connected {
			continue
		}
		cp := *p
		cp.Reachable = fresh
		peers = append(peers, cp)
		if p.Connected {
			connected++
		}
	}
	name, port, self := e.Name, e.Port, e.self
	paused, textOn, imageOn := e.Paused, e.SyncText, e.SyncImages
	e.mu.Unlock()

	sortPeers(peers)

	out := map[string]any{
		"device":     name,
		"port":       port,
		"selfAddr":   self,
		"paused":     paused,
		"syncText":   textOn,
		"syncImages": imageOn,
		"connected":  connected,
		"peers":      peers,
		"history":    hist,
		"version":    "ClipBridge " + appVersion,
		"platform":   platformName(),
	}
	b, _ := json.Marshal(out)
	return b
}

func sortPeers(peers []Peer) {
	for i := 1; i < len(peers); i++ {
		for j := i; j > 0 && strings.ToLower(peers[j].Name) < strings.ToLower(peers[j-1].Name); j-- {
			peers[j], peers[j-1] = peers[j-1], peers[j]
		}
	}
}

// Loop is the whole engine: watch the local clipboard, and poll every
// connected device.
func (e *Engine) Loop() {
	localTick := time.NewTicker(450 * time.Millisecond)
	peerTick := time.NewTicker(600 * time.Millisecond)
	defer localTick.Stop()
	defer peerTick.Stop()
	for {
		select {
		case <-e.stop:
			return
		case <-localTick.C:
			e.readLocal()
		case <-peerTick.C:
			e.pollPeers()
		}
	}
}

func (e *Engine) Stop() {
	select {
	case <-e.stop:
	default:
		close(e.stop)
	}
	if e.server != nil {
		e.server.Close()
	}
}

func (e *Engine) readLocal() {
	e.mu.Lock()
	paused := e.Paused
	textOn, imageOn := e.SyncText, e.SyncImages
	e.mu.Unlock()
	if paused {
		return
	}

	clip := localClip(textOn, imageOn)
	if clip == nil {
		return
	}
	h := hashOf(clip.Kind, clip.PNG, clip.Text)

	e.mu.Lock()
	if h == e.localHash {
		e.mu.Unlock()
		return
	}
	e.localHash = h
	e.rev++
	e.kind, e.text, e.png, e.hash = clip.Kind, clip.Text, clip.PNG, h
	clip.Origin = "this device"
	clip.When = time.Now()
	e.history = append([]Clip{*clip}, e.history...)
	if len(e.history) > 40 {
		e.history = e.history[:40]
	}
	targets := make([]string, 0, len(e.peers))
	for _, p := range e.peers {
		if p.Connected {
			targets = append(targets, p.Addr)
		}
	}
	e.mu.Unlock()

	for _, addr := range targets {
		if !e.push(addr, clip) {
			logf("push to %s failed", addr)
		}
	}
}

func (e *Engine) pollPeers() {
	e.mu.Lock()
	paused := e.Paused
	targets := make([]Peer, 0, len(e.peers))
	for _, p := range e.peers {
		if p.Connected {
			targets = append(targets, *p)
		}
	}
	e.mu.Unlock()

	if paused {
		return
	}

	for _, p := range targets {
		st, err := e.getState(p.Addr)
		if err != nil {
			continue
		}
		e.mu.Lock()
		real, ok := e.peers[p.Addr]
		if ok {
			real.lastSeen = time.Now()
			real.Reachable = true
			if st.Name != "" {
				real.Name = st.Name
			}
		}
		lastRev := p.lastRev
		e.mu.Unlock()

		if st.ID != "" && st.ID == e.ID {
			continue
		}
		if st.Rev == lastRev {
			continue
		}

		switch st.Type {
		case "image":
			if png, err := e.getImage(p.Addr); err == nil && len(png) > 0 {
				e.applyRemote(p, "image", "", png)
			}
		case "text":
			e.applyRemote(p, "text", st.Text, nil)
		}

		e.mu.Lock()
		if real, ok := e.peers[p.Addr]; ok {
			real.lastRev = st.Rev
		}
		e.mu.Unlock()
	}
}

func (e *Engine) applyRemote(p Peer, kind, text string, png []byte) {
	h := hashOf(kind, png, text)

	e.mu.Lock()
	if h == e.localHash {
		e.mu.Unlock()
		return
	}
	e.localHash = h
	e.rev++
	e.kind, e.text, e.png, e.hash = kind, text, png, h
	clip := Clip{When: time.Now(), Kind: kind, Origin: p.Name}
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

	if kind == "image" {
		writePNG(png)
	} else {
		writeText(text)
	}
}

func (e *Engine) push(addr string, c *Clip) bool {
	var resp *http.Response
	var err error
	url := "http://" + addr + "/set"
	if c.Kind == "image" {
		resp, err = e.client.Post(url, "image/png", bytes.NewReader(c.PNG))
	} else {
		body, _ := json.Marshal(map[string]string{"text": c.Text})
		resp, err = e.client.Post(url, "application/json", bytes.NewReader(body))
	}
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)
	return resp.StatusCode == 200
}

func (e *Engine) getState(addr string) (*remoteState, error) {
	resp, err := e.client.Get("http://" + addr + "/state")
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

func (e *Engine) getImage(addr string) ([]byte, error) {
	resp, err := e.client.Get("http://" + addr + "/image")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// SendCurrent pushes whatever sits on this clipboard right now, on demand.
func (e *Engine) SendCurrent() int {
	e.mu.Lock()
	textOn, imageOn := e.SyncText, e.SyncImages
	targets := make([]string, 0, len(e.peers))
	for _, p := range e.peers {
		if p.Connected {
			targets = append(targets, p.Addr)
		}
	}
	e.mu.Unlock()

	clip := localClip(textOn, imageOn)
	if clip == nil || len(targets) == 0 {
		return 0
	}
	sent := 0
	for _, addr := range targets {
		if e.push(addr, clip) {
			sent++
		}
	}
	if sent > 0 {
		e.mu.Lock()
		e.localHash = hashOf(clip.Kind, clip.PNG, clip.Text)
		e.mu.Unlock()
	}
	return sent
}

// CopyAt puts a clip from the recent list back on the clipboard.
func (e *Engine) CopyAt(i int) bool {
	e.mu.Lock()
	if i < 0 || i >= len(e.history) {
		e.mu.Unlock()
		return false
	}
	c := e.history[i]
	e.localHash = hashOf(c.Kind, c.PNG, c.Text)
	e.mu.Unlock()
	if c.Kind == "image" {
		return writePNG(c.PNG)
	}
	return writeText(c.Text)
}

func localClip(textOn, imageOn bool) *Clip {
	if imageOn {
		if png := readPNG(); len(png) > 0 {
			return &Clip{When: time.Now(), Kind: "image", Origin: "this device",
				Snippet: sizeText(len(png)), PNG: png}
		}
	}
	if textOn {
		if t := readText(); t != "" {
			return &Clip{When: time.Now(), Kind: "text", Origin: "this device",
				Snippet: snippet(t), Text: t}
		}
	}
	return nil
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

func sizeText(n int) string {
	if n < 1024 {
		return "image " + strconv.Itoa(n) + " B"
	}
	return "image " + strconv.Itoa(n/1024) + " KB"
}

func deviceName() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "This device"
}

// deviceID is stable per install, so a device never links to itself when its
// address changes.
func deviceID() string {
	path := configDir() + "/device.id"
	if data, err := os.ReadFile(path); err == nil && len(data) > 8 {
		return strings.TrimSpace(string(data))
	}
	id := hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	id = id[:16]
	_ = os.MkdirAll(configDir(), 0o755)
	_ = os.WriteFile(path, []byte(id), 0o644)
	return id
}

func localIP() string {
	conn, err := net.Dial("udp4", "8.8.8.8:80")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	if addr, ok := conn.LocalAddr().(*net.UDPAddr); ok {
		return addr.IP.String()
	}
	return "127.0.0.1"
}
