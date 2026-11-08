# ClipBridge

Shared clipboard over your own network. Text and screenshots between your PC,
your laptop and your phone. No account, no cloud, nothing leaves the LAN.

## How it works

One device runs the **host**, usually the machine that stays on. It owns the
clipboard and answers on port 1717. Every other device runs the **client**,
which watches its own clipboard and talks to the host. Clients find the host
by themselves with a UDP broadcast, so there is nothing to configure.

## Install

### Linux host

```bash
tar xzf clipbridge-host.tar.gz
cd clipbridge
./install.sh
```

Needs `python3` and `xclip`. On Debian and Ubuntu: `sudo apt install -y xclip`.
The host starts with your desktop session and keeps running after logout.

### Windows client

Download `ClipBridge.exe` and run it. It opens a small window, finds the host
on your network, and starts syncing.

The window is the settings screen: host address, toggles for text, images and
pause, and a list of recent clips. Click any row to put it back on your
clipboard. Closing the window leaves it running in the background.

First run: Windows SmartScreen will warn because the binary is not signed.
Choose **More info**, then **Run anyway**.

### Linux client

Download `clipbridge-linux`, make it executable, and run it:

```bash
chmod +x clipbridge-linux
./clipbridge-linux
```

It opens the same window in your browser on `http://127.0.0.1:1718`.

## Options

`-server http://192.168.1.8:1717` skips discovery and pins the host.
`-ui 1718` changes the local window port.
`-hidden` starts without opening the window.

## Privacy

Clips travel directly between your devices on your own network. There is no
account, no server on the internet, and no telemetry. The host keeps only the
current clip in memory, never a history on disk.

## Status

Text and images, both directions, verified between two machines.
Android is not built yet.

## Licence

MIT.
