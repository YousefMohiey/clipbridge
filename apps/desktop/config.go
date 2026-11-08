package main

import (
	"encoding/json"
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
