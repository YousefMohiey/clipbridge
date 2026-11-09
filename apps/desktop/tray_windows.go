//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"
)

// A notification-area icon with its own hidden window and message loop. It runs
// on a locked thread beside the app window, and a failure here never takes the
// window or the sync down with it.

const (
	wmDestroy      = 0x0002
	wmTrayCallback = 0x8000 + 1
	wmLButtonDbl   = 0x0203
	wmRButtonUp    = 0x0205

	nimAdd    = 0x00000000
	nimModify = 0x00000001
	nifMsg    = 0x00000001
	nifIcon   = 0x00000002
	nifTip    = 0x00000004

	imageIcon         = 1
	lrLoadFromFile    = 0x00000010
	swShow            = 5
	tpmRightButton    = 0x0002
	mfString          = 0x00000000
	mfGrayed          = 0x00000001
	mfSeparator       = 0x00000800
	idShow            = 1
	idPause           = 2
	idQuit            = 3
	trayID            = 7
	hwndMessageWindow = ^uintptr(2) // (HWND)-3
)

var (
	shell32 = syscall.NewLazyDLL("shell32.dll")

	procRegisterClassEx    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW    = user32.NewProc("CreateWindowExW")
	procDefWindowProcW     = user32.NewProc("DefWindowProcW")
	procGetMessageW        = user32.NewProc("GetMessageW")
	procTranslateMessage   = user32.NewProc("TranslateMessage")
	procDispatchMessageW   = user32.NewProc("DispatchMessageW")
	procCreatePopupMenu    = user32.NewProc("CreatePopupMenu")
	procAppendMenuW        = user32.NewProc("AppendMenuW")
	procTrackPopupMenu     = user32.NewProc("TrackPopupMenu")
	procSetForegroundWnd   = user32.NewProc("SetForegroundWindow")
	procGetCursorPos       = user32.NewProc("GetCursorPos")
	procDestroyMenu        = user32.NewProc("DestroyMenu")
	procShowWindow         = user32.NewProc("ShowWindow")
	procFindWindowW        = user32.NewProc("FindWindowW")
	procLoadImageW         = user32.NewProc("LoadImageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procDestroyWindow      = user32.NewProc("DestroyWindow")
	procShellNotifyIconW   = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW   = kernel32.NewProc("GetModuleHandleW")
	procWndProcCallbackPtr uintptr
)

type point struct{ X, Y int32 }

type msgT struct {
	HWnd    syscall.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
	private uint32
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     syscall.Handle
	HIcon         syscall.Handle
	HCursor       syscall.Handle
	HbrBackground syscall.Handle
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       syscall.Handle
}

type notifyIconData struct {
	CbSize           uint32
	_                uint32
	HWnd             syscall.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	_                uint32
	HIcon            syscall.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         [16]byte
	HBalloonIcon     syscall.Handle
}

func utf16Ptr(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func windowTitle() string { return "ClipBridge " + appVersion }

func showAppWindow() {
	hwnd, _, _ := procFindWindowW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(windowTitle()))))
	if hwnd == 0 {
		return
	}
	procShowWindow.Call(hwnd, swShow)
	procSetForegroundWnd.Call(hwnd)
}

// startTray installs the notification-area icon on its own thread and returns
// immediately. Any failure is logged and nothing else is affected.
func startTray(e *Engine) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logf("tray failed: %v", r)
			}
		}()
		runtime.LockOSThread()
		if err := runTray(e); err != nil {
			logf("tray unavailable: %v", err)
		}
	}()
}

func runTray(e *Engine) error {
	hInst, _, _ := procGetModuleHandleW.Call(0)
	className := utf16Ptr("ClipBridgeTray")

	wc := wndClassEx{
		LpfnWndProc:   procWndProcCallbackPtr,
		HInstance:     syscall.Handle(hInst),
		LpszClassName: className,
	}
	wc.CbSize = uint32(unsafe.Sizeof(wc))

	if r, _, err := procRegisterClassEx.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return err
	}

	hwnd, _, err := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(utf16Ptr(""))),
		0, 0, 0, 0, 0, hwndMessageWindow, 0, hInst, 0)
	if hwnd == 0 {
		return err
	}

	icon := loadTrayIcon(hInst)
	nid := notifyIconData{
		HWnd:             syscall.Handle(hwnd),
		UID:              trayID,
		UFlags:           nifMsg | nifIcon | nifTip,
		UCallbackMessage: wmTrayCallback,
		HIcon:            syscall.Handle(icon),
	}
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	copy(nid.SzTip[:], syscall.StringToUTF16("ClipBridge - clipboard on this network"))
	if r, _, err := procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid))); r == 0 {
		return err
	}
	logf("tray icon ready")

	trayData = &nid
	trayIconHwnd = hwnd
	engineForTray = e

	var m msgT
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

var (
	trayData      *notifyIconData
	trayIconHwnd  uintptr
	engineForTray *Engine
)

func loadTrayIcon(hInst uintptr) uintptr {
	if len(iconBytes) == 0 {
		return 0
	}
	path := filepath.Join(configDir(), "clipbridge.ico")
	if err := os.WriteFile(path, iconBytes, 0o644); err != nil {
		return 0
	}
	h, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(utf16Ptr(path))), imageIcon, 0, 0, lrLoadFromFile)
	if h == 0 {
		h, _, _ = procLoadImageW.Call(hInst, uintptr(unsafe.Pointer(utf16Ptr(path))), imageIcon, 0, 0, lrLoadFromFile)
	}
	return h
}

func showMenu() {
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	appendItem := func(flags uintptr, id int, label string) {
		procAppendMenuW.Call(menu, flags, uintptr(id), uintptr(unsafe.Pointer(utf16Ptr(label))))
	}

	paused := false
	if engineForTray != nil {
		engineForTray.mu.Lock()
		paused = engineForTray.Paused
		engineForTray.mu.Unlock()
	}

	appendItem(mfString, idShow, "Open ClipBridge")
	if paused {
		appendItem(mfString, idPause, "Resume sync")
	} else {
		appendItem(mfString, idPause, "Pause sync")
	}
	appendItem(mfSeparator, 0, "")
	appendItem(mfString, idQuit, "Quit")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWnd.Call(trayIconHwnd)
	cmd, _, _ := procTrackPopupMenu.Call(menu, tpmRightButton, uintptr(pt.X), uintptr(pt.Y), 0, trayIconHwnd, 0)

	switch cmd {
	case idShow:
		showAppWindow()
	case idPause:
		if engineForTray != nil {
			engineForTray.mu.Lock()
			engineForTray.Paused = !engineForTray.Paused
			engineForTray.mu.Unlock()
			logf("paused=%v from tray", !paused)
		}
	case idQuit:
		if trayData != nil {
			procShellNotifyIconW.Call(1 /* NIM_DELETE */, uintptr(unsafe.Pointer(trayData)))
		}
		os.Exit(0)
	}
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case wmTrayCallback:
		switch uint32(lParam) & 0xffff {
		case wmRButtonUp:
			showMenu()
		case wmLButtonDbl:
			showAppWindow()
		}
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func init() {
	procWndProcCallbackPtr = syscall.NewCallback(wndProc)
}
