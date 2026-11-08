# ClipBridge: plan for a real clipboard app

Goal: a program with a KDE Connect style interface that shares the clipboard,
text and images, between Windows, Linux and Android. Install once, use it
without reading a manual.

## What the research found

Building this from zero means solving five hard things before the first pixel:
LAN discovery, pairing, end to end encryption, clipboard capture on three
operating systems, and installers. Two projects already solved all five.

- **ClipCascade** (GPL-3.0, Python, 1.9k stars, last push 8 Nov 2026)
  Windows, macOS, Linux and Android. Text, images and files. Peer to peer mode
  with no server, or self hosted. MSI installer on Windows, APK on Android,
  GUI and CLI on Linux.

- **CrossPaste** (AGPL-3.0, Kotlin + Compose Multiplatform, 2.7k stars, last
  push 8 Nov 2026)
  Windows, macOS, Linux, plus Android and iOS apps. Text, HTML, RTF, images,
  files, colours, URLs. LAN only, no cloud, no account. OCR, CLI, MCP server.
  The most polished interface of the two.

KDE Connect cannot be the base. Its clipboard plugin drops images on Windows
(bug 519637, filed against 25.12.3, still open) and its Windows binaries need
the full Qt and KDE build. Rebuilding it is strictly more work than starting
from one of the two projects above.

## Three routes

**A. Adopt and ship. Hours.**
Install the existing app on all three devices, verify it, then publish our own
installer bundle: our icon, our first run guide, a pinned version, served from
our own repo. No source changes. Fastest, zero maintenance.

**B. Fork and rebrand. One to three weeks.**
Take ClipCascade or CrossPaste, change the interface to the device list layout
we want, add our identity, build installers from our own repository through
GitHub Actions. We own the product. We inherit GPL-3.0 or AGPL-3.0 duties, so
the source stays public.

**C. Build from scratch. Four to eight weeks.**
Rust or Go core, Tauri or Compose interface, our own protocol, our own pairing
and encryption, three clients and an Android app. Total control, highest cost,
and we would spend most of that time rebuilding what already exists.

## Recommendation

Route A as a test, then B if the interface is not good enough. Do not start with C.

Order of work:

**M0. Trial day, about one hour.**
Install both apps on this laptop and the Windows PC. Send text and a
screenshot each way. Write down four numbers: minutes to install, clicks to
pair, whether images arrive, and how the app feels. The clipboard is the only
thing being judged.

**M1. Pick the base.**
Decide on the evidence from M0. The tie breaker is image clipboard between
Windows and Linux, because that is the exact thing that failed in KDE Connect.

**M2. Our product around it.**
Even without touching the source: our installer, our icon, a one page setup
guide, a settings preset that locks it to the LAN with no account, and a
release page we control. This is what makes it feel like a real app rather
than someone else's utility.

**M3. Interface work, only if M2 still feels foreign.**
Fork, replace the main screen with a device list carrying the device name, a
status dot, per device toggles, and a history pane. Build through GitHub
Actions on real Windows runners so we never fight a cross compiler again.

**M4. Android from source, only if the upstream APK is not good enough.**
Build the APK ourselves, sign it with our key, sideload it over the LAN.

## Constraints to respect

- GPL-3.0 and AGPL-3.0 both require publishing the source of anything we
  distribute, including our changes. That is fine, it just has to be decided
  up front.
- Installers stay unsigned until a signing route exists, so SmartScreen will
  warn on first run. Same situation as QuotaVPN.
- Android is sideloaded, not published to a store.
- No account and no cloud should be a hard rule. Both projects can run that
  way, which is the whole point.

## Open questions

1. Is no account, LAN only, a hard requirement, or is a hosted account fine?
2. Clipboard only, or do we also want file transfer between machines?
3. Is it acceptable that the base is a public GPL or AGPL project that we
   rebrand and extend, with our source published?

## What already exists in this repository

`clipd.py` is a working clipboard server for Linux that speaks text and PNG
over HTTP, tested end to end. It is not the product, it is the proof that the
protocol works, and it stays as a fallback while the trials run.
