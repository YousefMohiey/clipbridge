# ClipBridge

Shared clipboard over your own network. Text and screenshots between your PC,
your laptop and your phone. No account, no cloud, nothing leaves the LAN.

## How it works

One app, on every device. One of them is the **host**: it serves the clipboard
on port 1717. The rest are **guests** and find the host by themselves with a
UDP broadcast, so there is nothing to configure and no address to type.

Every app has a switch called **Host for the network**. Turn it on once, on the
machine that stays on, and leave the others alone.

## Install

### Windows

Download `ClipBridge.exe` and run it. SmartScreen will warn because the binary
is not signed: choose **More info**, then **Run anyway**.

### Linux

Download `clipbridge_<version>_amd64.deb` and open it with your software
installer. It installs the app and its clipboard dependency, and adds
**ClipBridge** to your applications menu. No terminal needed.

### Android

Download `ClipBridge.apk` and open it. Allow installing from your browser when
asked. Android does not let any app read the clipboard in the background, so on
the phone you use two buttons: **Send my clipboard** and **Get from host**.

## First run

1. On the machine that should be the host, open ClipBridge and switch on
   **Host for the network**. The window shows the address it is serving on.
2. On every other device, open ClipBridge. It finds the host and connects.
3. Copy something. Paste it on the other device.

Turn on **Start with system** on each device so it is there after a reboot.

## The window

Status in the top right, the host address, toggles for text and images, and a
list of recent clips. Click a row to put it back on your clipboard. Closing the
window keeps it running in the background.

## Privacy

Clips travel directly between your devices on your own network. There is no
account, no server on the internet, and no telemetry. Only the current clip is
kept in memory, never a history on disk.

## Licence

MIT.
