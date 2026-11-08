# ClipBridge

Shared clipboard over your own network. Text and screenshots, between your PC,
your laptop and your phone. No account, no cloud, nothing leaves the LAN.

## Why it exists

KDE Connect is a good program with one broken piece: image clipboard transfer
dies on Windows (KDE bug 519637, still open against 25.12.3). Everything else
about it is fine, but screenshots do not cross.

ClipBridge does one job and does it properly: copy text or an image on one
machine, paste it on another, a moment later.

## Shape of the thing

- The **host** runs on one machine, usually the desktop that stays on. It owns
  the clipboard and serves a tiny HTTP API on the LAN.
- Every other device runs a **client** that watches its own clipboard and
  talks to the host.
- Discovery is a UDP broadcast on the local network. No configuration, no
  account, no pairing server.
- Everything is local. There is no internet dependency and no telemetry.

## Status

Work in progress. What exists today:

- `host/clipd.py` - Linux host, text and PNG, tested end to end
- `clients/windows.ps1` - working Windows client, no runtime needed
- `clients/windows/go` - native Windows client, in progress

## Licence

MIT.
