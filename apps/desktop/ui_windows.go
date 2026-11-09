//go:build windows

package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

//go:embed icon.ico
var iconBytes []byte

type nativeUI struct {
	mw       *walk.MainWindow
	ni       *walk.NotifyIcon
	icon     *walk.Icon

	status   *walk.Label
	hostEdit *walk.LineEdit
	log      *walk.TextEdit

	cbText   *walk.CheckBox
	cbImages *walk.CheckBox
	cbPause  *walk.CheckBox
	cbHost   *walk.CheckBox
	cbBoot   *walk.CheckBox

	e   *Engine
	cfg *Config
}

func startNative(e *Engine, cfg *Config) (ok bool) {
	// A crash inside the window must not take the sync down with it.
	defer func() {
		if r := recover(); r != nil {
			logf("window failed: %v", r)
			ok = false
		}
	}()

	ui := &nativeUI{e: e, cfg: cfg}
	if err := ui.create(); err != nil {
		logf("cannot open the window: %v", err)
		return false
	}
	logf("window ready")
	ui.loop()
	ui.mw.Run()
	return true
}

func (u *nativeUI) create() error {
	iconPath := filepath.Join(os.TempDir(), "clipbridge-app.ico")
	if err := os.WriteFile(iconPath, iconBytes, 0o644); err == nil {
		if icon, err := walk.NewIconFromFile(iconPath); err == nil {
			u.icon = icon
		}
	}

	err := (MainWindow{
		AssignTo: &u.mw,
		Title:    "ClipBridge",
		Size:     Size{Width: 560, Height: 660},
		MinSize:  Size{Width: 460, Height: 520},
		Layout:   VBox{},
		Children: []Widget{
			Composite{
				Layout: HBox{},
				Children: []Widget{
					Label{Text: "Status"},
					Label{AssignTo: &u.status, Text: "Starting"},
				},
			},
			Composite{
				Layout: HBox{},
				Children: []Widget{
					LineEdit{AssignTo: &u.hostEdit, Text: u.cfg.Server},
					PushButton{
						Text: "Apply",
						OnClicked: func() {
							addr := strings.TrimSpace(u.hostEdit.Text())
							if addr == "" {
								return
							}
							if !strings.HasPrefix(addr, "http") {
								addr = "http://" + addr
							}
							u.e.SetServer(addr)
							u.cfg.Server = addr
							SaveConfig(*u.cfg)
						},
					},
					PushButton{
						Text: "Send now",
						OnClicked: func() {
							go func() {
								ok := u.e.SendCurrent()
								u.mw.Synchronize(func() {
									if ok {
										u.status.SetText("Sent to host")
									} else {
										u.status.SetText("Nothing to send")
									}
								})
							}()
						},
					},
				},
			},
			GroupBox{
				Title:  "Sync",
				Layout: HBox{},
				Children: []Widget{
					CheckBox{AssignTo: &u.cbText, Text: "Text", Checked: u.cfg.SyncText,
						OnCheckedChanged: func() { u.setToggle("text", u.cbText.Checked()) }},
					CheckBox{AssignTo: &u.cbImages, Text: "Images", Checked: u.cfg.SyncImages,
						OnCheckedChanged: func() { u.setToggle("images", u.cbImages.Checked()) }},
					CheckBox{AssignTo: &u.cbPause, Text: "Pause",
						OnCheckedChanged: func() { u.setToggle("pause", u.cbPause.Checked()) }},
				},
			},
			GroupBox{
				Title:  "This device",
				Layout: VBox{},
				Children: []Widget{
					CheckBox{AssignTo: &u.cbHost, Text: "Host for the network", Checked: u.cfg.Host,
						OnCheckedChanged: func() { u.setHost(u.cbHost.Checked()) }},
					CheckBox{AssignTo: &u.cbBoot, Text: "Start with system", Checked: u.cfg.StartOnBoot,
						OnCheckedChanged: func() {
							SetStartOnBoot(u.cbBoot.Checked())
							u.cfg.StartOnBoot = u.cbBoot.Checked()
							SaveConfig(*u.cfg)
						}},
				},
			},
			Label{Text: "Recent"},
			TextEdit{AssignTo: &u.log, ReadOnly: true, VScroll: true},
			PushButton{
				Text: "Hide to tray",
				OnClicked: func() { u.mw.Hide() },
			},
		},
	}).Create()
	if err != nil {
		return err
	}

	if u.icon != nil {
		u.mw.SetIcon(u.icon)
	}
	u.mw.Closing().Attach(func(canceled *bool, reason walk.CloseReason) {
		*canceled = true
		u.mw.Hide()
	})

	u.createTray()
	return nil
}

func (u *nativeUI) createTray() {
	ni, err := walk.NewNotifyIcon(u.mw)
	if err != nil {
		logf("no tray icon: %v", err)
		return
	}
	u.ni = ni
	if u.icon != nil {
		_ = ni.SetIcon(u.icon)
	}
	_ = ni.SetToolTip("ClipBridge")
	_ = ni.SetVisible(true)

	open := walk.NewAction()
	_ = open.SetText("Open")
	open.Triggered().Attach(func() {
		u.mw.Show()
		u.mw.Activate()
	})

	send := walk.NewAction()
	_ = send.SetText("Send clipboard now")
	send.Triggered().Attach(func() { go u.e.SendCurrent() })

	exit := walk.NewAction()
	_ = exit.SetText("Exit")
	exit.Triggered().Attach(func() {
		_ = ni.SetVisible(false)
		ni.Dispose()
		u.mw.Dispose()
	})

	_ = ni.ContextMenu().Actions().Add(open)
	_ = ni.ContextMenu().Actions().Add(send)
	_ = ni.ContextMenu().Actions().Add(walk.NewSeparatorAction())
	_ = ni.ContextMenu().Actions().Add(exit)

	ni.MouseDown().Attach(func(x, y int, button walk.MouseButton) {
		if button == walk.LeftButton {
			u.mw.Show()
			u.mw.Activate()
		}
	})
}

func (u *nativeUI) setToggle(name string, on bool) {
	switch name {
	case "text":
		u.e.mu.Lock()
		u.e.SyncText = on
		u.e.mu.Unlock()
		u.cfg.SyncText = on
	case "images":
		u.e.mu.Lock()
		u.e.SyncImages = on
		u.e.mu.Unlock()
		u.cfg.SyncImages = on
	case "pause":
		u.e.mu.Lock()
		u.e.Paused = on
		u.e.mu.Unlock()
	}
	SaveConfig(*u.cfg)
}

func (u *nativeUI) setHost(on bool) {
	if on {
		if err := u.e.StartHost(); err != nil {
			u.status.SetText("Cannot host: " + err.Error())
			u.cbHost.SetChecked(false)
			return
		}
		u.cfg.Host = true
	} else {
		u.e.StopHost()
		u.cfg.Host = false
	}
	SaveConfig(*u.cfg)
}

// loop refreshes the window from the engine on a timer.
func (u *nativeUI) loop() {
	go func() {
		tick := time.NewTicker(700 * time.Millisecond)
		defer tick.Stop()
		for range tick.C {
			u.mw.Synchronize(u.refresh)
		}
	}()
}

func (u *nativeUI) refresh() {
	connected, message, server := u.e.snapshot()
	u.e.mu.Lock()
	hosting := u.e.hosting
	hostAddr := u.e.hostAddr
	hist := make([]Clip, len(u.e.history))
	copy(hist, u.e.history)
	u.e.mu.Unlock()

	if hosting {
		u.status.SetText("Hosting on " + hostAddr)
	} else if connected {
		u.status.SetText("Connected to " + server)
	} else {
		u.status.SetText(message)
	}

	if !hosting && u.hostEdit.Text() == "" && server != "" {
		u.hostEdit.SetText(server)
	}

	var b strings.Builder
	for i, c := range hist {
		if i >= 30 {
			break
		}
		b.WriteString(fmt.Sprintf("%s  %-5s  %-11s  %s\r\n",
			c.When.Format("15:04:05"), strings.ToUpper(c.Kind), c.Origin, c.Snippet))
	}
	if b.Len() == 0 {
		b.WriteString("Nothing yet. Copy something on either device.")
	}
	if u.log.Text() != b.String() {
		u.log.SetText(b.String())
	}
}
