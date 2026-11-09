package main

import (
	"encoding/json"
	"fmt"
	"time"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

type Config struct {
	Server      string `json:"server"`
	Host        bool   `json:"host"`
	SyncText    bool   `json:"syncText"`
	SyncImages  bool   `json:"syncImages"`
	StartOnBoot bool   `json:"startOnBoot"`
}

func configDir() string {
	if runtime.GOOS == "windows" {
		if dir := os.Getenv("LOCALAPPDATA"); dir != "" {
			return filepath.Join(dir, "ClipBridge")
		}
	}
	if dir, err := os.UserConfigDir(); err == nil {
		return filepath.Join(dir, "clipbridge")
	}
	return "."
}

func configPath() string { return filepath.Join(configDir(), "config.json") }

// logf appends a line to app.log next to the config. The Windows build has no
// console, so this is the only place a problem can be seen.
func logf(format string, args ...any) {
	_ = os.MkdirAll(configDir(), 0o755)
	f, err := os.OpenFile(filepath.Join(configDir(), "app.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	line := time.Now().Format("2006-01-02 15:04:05 ") + fmt.Sprintf(format, args...) + "\n"
	_, _ = f.WriteString(line)
}

func uiPortPath() string { return filepath.Join(configDir(), "ui.port") }

func writeUIPort(port int) {
	_ = os.MkdirAll(configDir(), 0o755)
	_ = os.WriteFile(uiPortPath(), []byte(fmt.Sprintf("%d", port)), 0o644)
}

func readUIPort() int {
	data, err := os.ReadFile(uiPortPath())
	if err != nil {
		return 0
	}
	var p int
	_, err = fmt.Sscanf(string(data), "%d", &p)
	if err != nil {
		return 0
	}
	return p
}

func LoadConfig() Config {
	cfg := Config{SyncText: true, SyncImages: true}
	data, err := os.ReadFile(configPath())
	if err != nil {
		return cfg
	}
	_ = json.Unmarshal(data, &cfg)
	return cfg
}

func SaveConfig(cfg Config) {
	_ = os.MkdirAll(configDir(), 0o755)
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(configPath(), data, 0o644)
}

// SetStartOnBoot registers or removes the current user's autostart entry.
func SetStartOnBoot(on bool) {
	exe, err := os.Executable()
	if err != nil {
		return
	}
	switch runtime.GOOS {
	case "windows":
		if on {
			_ = exec.Command("reg", "add", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
				"/v", "ClipBridge", "/t", "REG_SZ", "/d", exe, "/f").Run()
		} else {
			_ = exec.Command("reg", "delete", `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`,
				"/v", "ClipBridge", "/f").Run()
		}
	default:
		dir, err := os.UserConfigDir()
		if err != nil {
			return
		}
		entry := filepath.Join(dir, "autostart", "clipbridge.desktop")
		if !on {
			_ = os.Remove(entry)
			return
		}
		_ = os.MkdirAll(filepath.Dir(entry), 0o755)
		content := "[Desktop Entry]\nType=Application\nName=ClipBridge\nExec=" + exe +
			" -hidden\nTerminal=false\nX-GNOME-Autostart-enabled=true\n"
		_ = os.WriteFile(entry, []byte(content), 0o644)
	}
}
