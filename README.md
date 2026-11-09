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

The window draws with the WebView2 runtime. Windows 10 and 11 ship it, so there
is nothing to install. If your PC is one of the rare ones without it, the app
says so and links you to Microsoft's one-time installer.

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

A native window with a tray icon, not a web page. Your other devices appear on
the left as soon as their app is open on the same network. Press **Connect**
next to one and it says **connected**; from then on clips travel both ways with
every device you linked. Disconnect is the same button.

There is no host and no guest. Every device serves and every device can connect
to the others, so two laptops and a phone can all be linked at once.

The tray menu has Open ClipBridge, Pause or Resume sync, and Quit.

On Linux the same page opens in your browser on port 1770 when you launch the
app from your applications menu. Nothing opens on its own at login.

## Privacy

Clips travel directly between your devices on your own network. There is no
account, no server on the internet, and no telemetry. Only the current clip is
kept in memory, never a history on disk.

## Licence

MIT.
