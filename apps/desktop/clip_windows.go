//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/draw"
	"image/png"
	"syscall"
	"unsafe"

	"golang.org/x/image/bmp"
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
	procGlobalFree   = kernel32.NewProc("GlobalFree")
	procMessageBox   = user32.NewProc("MessageBoxW")
)

const (
	cfDIB         = 8
	cfUnicodeText = 13
	gmemMoveable  = 0x0002
)

// pngFormat returns the clipboard format id for "PNG", registering it if needed.
func pngFormat() uintptr {
	name, _ := syscall.UTF16PtrFromString("PNG")
	id, _, _ := procRegisterFmt.Call(uintptr(unsafe.Pointer(name)))
	return id
}

func readClipboardBytes(format uintptr) []byte {
	if ok, _, _ := procIsClipboardFmtAv.Call(format); ok == 0 {
		return nil
	}
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return nil
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(format)
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
	copy(out, unsafe.Slice((*byte)(unsafe.Pointer(p)), int(size)))
	return out
}

// readText returns the clipboard text, or "" when there is none.
func readText() string {
	if ok, _, _ := procIsClipboardFmtAv.Call(cfUnicodeText); ok == 0 {
		return ""
	}
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return ""
	}
	defer procCloseClipboard.Call()

	h, _, _ := procGetClipboardData.Call(cfUnicodeText)
	if h == 0 {
		return ""
	}
	size, _, _ := procGlobalSize.Call(h)
	if size < 2 {
		return ""
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		return ""
	}
	defer procGlobalUnlock.Call(h)

	units := unsafe.Slice((*uint16)(unsafe.Pointer(p)), int(size)/2)
	return syscall.UTF16ToString(units)
}

// readPNG returns image bytes as PNG. Many Windows tools only offer a device
// independent bitmap, so that is converted when no PNG format is present.
func readPNG() []byte {
	if raw := readClipboardBytes(pngFormat()); len(raw) > 0 {
		if _, err := png.Decode(bytes.NewReader(raw)); err == nil {
			return raw
		}
	}
	dib := readClipboardBytes(cfDIB)
	if len(dib) < 40 {
		return nil
	}
	img, err := decodeDIB(dib)
	if err != nil {
		return nil
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		return nil
	}
	return out.Bytes()
}

func decodeDIB(dib []byte) (image.Image, error) {
	headerSize := binary.LittleEndian.Uint32(dib[0:4])
	if headerSize < 40 || int(headerSize) > len(dib) {
		return nil, syscall.EINVAL
	}
	offset := 14 + int(headerSize)
	fileSize := 14 + len(dib)
	file := make([]byte, 14, fileSize)
	file[0], file[1] = 'B', 'M'
	binary.LittleEndian.PutUint32(file[2:6], uint32(fileSize))
	binary.LittleEndian.PutUint32(file[10:14], uint32(offset))
	file = append(file, dib...)
	return bmp.Decode(bytes.NewReader(file))
}

func putClipboardRaw(format uintptr, data []byte) bool {
	h, _, _ := procGlobalAlloc.Call(gmemMoveable, uintptr(len(data)))
	if h == 0 {
		return false
	}
	p, _, _ := procGlobalLock.Call(h)
	if p == 0 {
		procGlobalFree.Call(h)
		return false
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(p)), len(data)), data)
	procGlobalUnlock.Call(h)

	if r, _, _ := procSetClipboardData.Call(format, h); r == 0 {
		procGlobalFree.Call(h)
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
		binary.LittleEndian.PutUint16(buf[i*2:], v)
	}
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return false
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()
	return putClipboardRaw(cfUnicodeText, buf)
}

// writePNG puts the image on the clipboard as both DIB and PNG, so that every
// receiving application finds a format it understands.
func writePNG(data []byte) bool {
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return false
	}
	dib := encodeDIB(img)
	if len(dib) == 0 {
		return false
	}
	if r, _, _ := procOpenClipboard.Call(0); r == 0 {
		return false
	}
	defer procCloseClipboard.Call()
	procEmptyClipboard.Call()

	okDIB := putClipboardRaw(cfDIB, dib)
	putClipboardRaw(pngFormat(), data)
	return okDIB
}

func encodeDIB(img image.Image) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w == 0 || h == 0 {
		return nil
	}
	rgba := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(rgba, rgba.Bounds(), img, b.Min, draw.Src)

	stride := w * 4
	header := make([]byte, 40)
	binary.LittleEndian.PutUint32(header[0:4], 40)
	binary.LittleEndian.PutUint32(header[4:8], uint32(w))
	binary.LittleEndian.PutUint32(header[8:12], uint32(h))
	binary.LittleEndian.PutUint16(header[12:14], 1)
	binary.LittleEndian.PutUint16(header[14:16], 32)
	binary.LittleEndian.PutUint32(header[20:24], uint32(stride*h))

	pixels := make([]byte, stride*h)
	for y := 0; y < h; y++ {
		srcRow := rgba.Pix[(h-1-y)*rgba.Stride:]
		dstRow := pixels[y*stride:]
		for x := 0; x < w; x++ {
			dstRow[x*4+0] = srcRow[x*4+2]
			dstRow[x*4+1] = srcRow[x*4+1]
			dstRow[x*4+2] = srcRow[x*4+0]
			dstRow[x*4+3] = 255
		}
	}
	return append(header, pixels...)
}

// messageBox shows a native dialog. Used when the window cannot start, so the
// reason is visible instead of silent.
func messageBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(title)
	m, _ := syscall.UTF16PtrFromString(text)
	procMessageBox.Call(0, uintptr(unsafe.Pointer(m)), uintptr(unsafe.Pointer(t)), 0x10)
}
