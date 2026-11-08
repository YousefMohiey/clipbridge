//go:build linux

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func xclipPath() string {
	for _, p := range []string{
		"/usr/bin/xclip",
		"/usr/local/bin/xclip",
		filepath.Join(os.Getenv("HOME"), ".local/opt/xclip/usr/bin/xclip"),
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}

// capture runs a command and returns its stdout. Used for reads, where the
// program exits on its own.
func capture(args ...string) []byte {
	if xclipPath() == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, xclipPath(), args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return nil
	}
	return out.Bytes()
}

// send runs a command that takes over the clipboard. xclip forks a child to
// serve the selection and leaves it running, so stdout must not be a pipe:
// waiting on that pipe would hang until the child dies, which is never.
func send(stdin []byte, args ...string) bool {
	if xclipPath() == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, xclipPath(), args...)
	if stdin != nil {
		cmd.Stdin = bytes.NewReader(stdin)
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run() == nil
}

func readPNG() []byte {
	targets := capture("-selection", "clipboard", "-o", "-t", "TARGETS")
	if !bytes.Contains(targets, []byte("image/png")) {
		return nil
	}
	return capture("-selection", "clipboard", "-o", "-t", "image/png")
}

func readText() string {
	return string(capture("-selection", "clipboard", "-o"))
}

func writeText(s string) bool {
	return send([]byte(s), "-selection", "clipboard", "-i")
}

func writePNG(b []byte) bool {
	tmp := filepath.Join(os.TempDir(), "clipbridge-local.png")
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return false
	}
	return send(nil, "-selection", "clipboard", "-t", "image/png", "-i", tmp)
}
