package main

import (
	"syscall"
	"unsafe"
)

var (
	user32   = syscall.NewLazyDLL("user32.dll")
	kernel32 = syscall.NewLazyDLL("kernel32.dll")

	procOpenClipboard    = user32.NewProc("OpenClipboard")
	procCloseClipboard   = user32.NewProc("CloseClipboard")
	procEmptyClipboard   = user32.NewProc("EmptyClipboard")
	procGetClipboardData = user32.NewProc("GetClipboardData")
	procSetClipboardData = user32.NewProc("SetClipboardData")
	procIsClipboardFmtAv = user32.NewProc("IsClipboardFormatAvailable")
	procRegisterFmt      = user32.NewProc("RegisterClipboardFormatW")

	procGlobalAlloc  = kernel32.NewProc("GlobalAlloc")
	procGlobalLock   = kernel32.NewProc("GlobalLock")
	procGlobalUnlock = kernel32.NewProc("GlobalUnlock")
	procGlobalSize   = kernel32.NewProc("GlobalSize")

	procMessageBox = user32.NewProc("MessageBoxW")
)

const (
	cfUnicodeText = 13
	cfDIB         = 8
	gmemMoveable  = 0x0002
)

// pngFormat returns the clipboard format id for "PNG", registering it if needed.
func pngFormat() uintptr {
	name, _ := syscall.UTF16PtrFromString("PNG")
	id, _, _ := procRegisterFmt.Call(uintptr(unsafe.Pointer(name)))
	return id
}

func hasText() bool {
	ok, _, _ := procIsClipboardFmtAv.Call(cfUnicodeText)
	return ok != 0
}

// readText returns the clipboard text, or "" when there is none.
func readText() string {
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return ""
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(h)
	return syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(p)), 1<<20))
}

// readPNG returns PNG bytes from the clipboard if a PNG format is present.
func readPNG() []byte {
	f := pngFormat()
	if ok, _, _ := procIsClipboardFmtAv.Call(f); ok == 0 {
		return nil
	}
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return nil
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(f)
	if h == 0 {
		return nil
	}
	size, _, _ := procGlobalSize.Call(h)
	if size == 0 {
		return nil
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return nil
	}
	defer procGlobalUnlock.Call(h)
	out := make([]byte, size)
	copy(out, unsafe.Slice((*byte)(unsafe.Pointer(p)), size))
	return out
}

func writeClipboard(format uintptr, data []byte) bool {
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return false
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()

	h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return false
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return false
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
	procGlobalUnlock.Call(h)

	if r, _, _ := procSetClipboardData.Call(format, h); r == 0 {
		return false
	}
	return true
}

func writeText(s string) bool {
	u, err := syscall.UTF16FromString(s)
	if err != nil {
		return false
	}
	buf := make([]byte, len(u)*2)
	for i, v := range u {
		buf[i*2] = byte(v)
		buf[i*2+1] = byte(v >> 8)
	}
	return writeClipboard(cfUnicodeText, buf)
}

func writePNG(b []byte) bool {
	return writeClipboard(pngFormat(), b)
}

func notify(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x40)
}
