package main

import (
	"syscall"
	"unsafe"
)

var (
	user32  = syscall.NewLazyDLL("user32.dll")
	msgBox  = user32.NewProc("MessageBoxW")
	kernel  = syscall.NewLazyDLL("kernel32.dll")
	getMem  = kernel.NewProc("GlobalMemoryStatusEx")
)

type memStatus struct {
	Length               uint32
	MemoryLoad           uint32
	TotalPhys            uint64
	AvailPhys            uint64
	TotalPageFile        uint64
	AvailPageFile        uint64
	TotalVirtual         uint64
	AvailVirtual         uint64
	AvailExtendedVirtual uint64
}

func main() {
	var m memStatus
	m.Length = uint32(unsafe.Sizeof(m))
	getMem.Call(uintptr(unsafe.Pointer(&m)))

	text := "This binary does not use .NET.\r\n\r\n" +
		"If you can read this, the machine runs ordinary Windows programs fine " +
		"and the problem is specific to the .NET build.\r\n\r\n" +
		"RAM: " + human(m.TotalPhys) + " total\r\n" +
		"Page file: " + human(m.TotalPageFile) + " total, " + human(m.AvailPageFile) + " free"

	title := "ClipBridge probe"
	textPtr, _ := syscall.UTF16PtrFromString(text)
	titlePtr, _ := syscall.UTF16PtrFromString(title)
	msgBox.Call(0, uintptr(unsafe.Pointer(textPtr)), uintptr(unsafe.Pointer(titlePtr)), 0x40)
}

func human(b uint64) string {
	const mb = 1024 * 1024
	const gb = 1024 * mb
	if b >= gb {
		return itoa(b/gb) + " GB"
	}
	return itoa(b/mb) + " MB"
}

func itoa(v uint64) string {
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
