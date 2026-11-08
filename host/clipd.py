#!/usr/bin/env python3
"""ClipBridge host: keeps the X clipboard and a small HTTP endpoint in sync.

Text and PNG images both travel. A Windows poller pulls changes and posts
its own, so nothing has to accept inbound connections on the PC.
"""

import base64
import hashlib
import json
import os
import subprocess
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

PORT = 1717
POLL = 0.4
DISPLAY = os.environ.get("DISPLAY", ":0")
XCLIP = os.path.expanduser("~/.local/opt/xclip/usr/bin/xclip")
HERE = os.path.dirname(os.path.abspath(__file__))

lock = threading.Lock()
state = {"rev": 0, "type": None, "hash": None, "text": None, "png": None}
applied = {"hash": None}  # last hash we wrote into X, to stop an echo


def env():
    e = dict(os.environ)
    e["DISPLAY"] = DISPLAY
    return e


def run(args, stdin=None):
    return subprocess.run(args, input=stdin, capture_output=True, env=env(), timeout=10)


def targets():
    p = run([XCLIP, "-selection", "clipboard", "-o", "-t", "TARGETS"])
    if p.returncode != 0:
        return []
    return p.stdout.decode("utf-8", "replace").split()


def read_clip():
    """Return (type, text, png_bytes) describing the current X clipboard."""
    t = targets()
    if any(x in ("image/png", "image/bmp", "image/jpeg") for x in t):
        for mime in ("image/png", "image/jpeg", "image/bmp"):
            if mime in t:
                p = run([XCLIP, "-selection", "clipboard", "-o", "-t", mime])
                if p.returncode == 0 and p.stdout:
                    return "image", None, p.stdout
    p = run([XCLIP, "-selection", "clipboard", "-o"])
    if p.returncode == 0:
        return "text", p.stdout.decode("utf-8", "replace"), None
    return None, None, None


def write_clip(kind, text, png):
    if kind == "image":
        tmp = "/tmp/clipbridge.png"
        with open(tmp, "wb") as f:
            f.write(png)
        run([XCLIP, "-selection", "clipboard", "-t", "image/png", "-i", tmp])
    else:
        run([XCLIP, "-selection", "clipboard", "-i"], stdin=text.encode("utf-8"))


def digest(kind, text, png):
    h = hashlib.sha256()
    h.update(kind.encode())
    h.update(text.encode("utf-8") if text is not None else png or b"")
    return h.hexdigest()


def adopt(kind, text, png, from_remote=False):
    h = digest(kind, text, png)
    with lock:
        if h == state["hash"]:
            return
        state.update({"rev": state["rev"] + 1, "type": kind, "hash": h,
                      "text": text, "png": png})
        if from_remote:
            applied["hash"] = h


def watcher():
    while True:
        try:
            kind, text, png = read_clip()
            if kind:
                h = digest(kind, text, png)
                with lock:
                    skip = h == state["hash"] or h == applied["hash"]
                if not skip:
                    adopt(kind, text, png)
        except Exception:
            pass
        time.sleep(POLL)


def page():
    with lock:
        snap = dict(state)
    kind = snap["type"] or "nothing yet"
    preview = ""
    if snap["type"] == "text" and snap["text"]:
        preview = snap["text"][:300].replace("<", "&lt;").replace(">", "&gt;")
    elif snap["type"] == "image":
        preview = f"image, {len(snap['png'] or b'') // 1024} KB"
    html = f"""<!doctype html><meta charset=utf-8><title>ClipBridge</title>
<style>
body{{margin:0;background:#0e1116;color:#e6edf3;font:15px/1.5 system-ui,Segoe UI,sans-serif}}
main{{max-width:680px;margin:48px auto;padding:0 24px}}
h1{{font-size:22px;font-weight:600;margin:0 0 4px}}
p.sub{{color:#8b98a8;margin:0 0 28px}}
.card{{background:#151a21;border:1px solid #26303c;border-radius:12px;padding:18px 20px;margin-bottom:16px}}
.k{{color:#8b98a8;font-size:12px;text-transform:uppercase;letter-spacing:.08em}}
.v{{margin-top:6px;word-break:break-word}}
a.btn{{display:inline-block;background:#1c2632;border:1px solid #2f3d4c;color:#e6edf3;
text-decoration:none;padding:9px 14px;border-radius:8px;margin:4px 8px 0 0;font-size:14px}}
a.btn:hover{{background:#243040}}
.dot{{display:inline-block;width:8px;height:8px;border-radius:50%;background:#22c55e;margin-right:8px}}
</style>
<main>
<h1>ClipBridge</h1>
<p class=sub><span class=dot></span>host running on port {PORT}</p>
<div class=card><div class=k>Current clipboard</div><div class=v>{kind}: {preview}</div></div>
<div class=card><div class=k>Windows: one download</div>
<div class=v style="margin-bottom:12px">Extract the zip, double click <b>Install ClipBridge.cmd</b>. It installs itself and runs hidden from then on.</div>
<a class=btn href="/download/ClipBridge-Setup.zip">Download ClipBridge Setup</a></div>
<div class=card><div class=k>Other builds</div>
<a class=btn href="/download/probe.exe">probe.exe</a>
<a class=btn href="/download/ClipBridge.zip">ClipBridge folder build</a>
<a class=btn href="/clipbridge.ps1">PowerShell client</a></div>
</main>"""
    return html.encode("utf-8")


class Handler(BaseHTTPRequestHandler):

    def log_message(self, *a):
        pass

    def _send(self, code, body=b"", ctype="application/json"):
        self.send_response(code)
        self.send_header("Content-Type", ctype)
        self.send_header("Content-Length", str(len(body)))
        self.send_header("Access-Control-Allow-Origin", "*")
        self.end_headers()
        if body:
            self.wfile.write(body)

    def do_GET(self):
        path = self.path.split("?")[0]
        if path == "/health":
            return self._send(200, b'{"ok":true}')
        if path == "/":
            return self._send(200, page(), "text/html; charset=utf-8")
        if path.startswith("/download/"):
            name = os.path.basename(path)
            p = os.path.join(HERE, "dist", name)
            if os.path.exists(p):
                return self._send(200, open(p, "rb").read(), "application/octet-stream")
            return self._send(404, b'{"error":"missing"}')
        if path.endswith(".ps1") and path.count("/") == 1:
            p = os.path.join(HERE, os.path.basename(path))
            if os.path.exists(p):
                return self._send(200, open(p, "rb").read(), "text/plain")
            return self._send(404, b'{"error":"missing"}')
        if path in ("/state", "/image"):
            with lock:
                snap = dict(state)
            if path == "/image":
                if snap["type"] != "image" or not snap["png"]:
                    return self._send(404, b'{"error":"no image"}')
                return self._send(200, snap["png"], "image/png")
            body = {"rev": snap["rev"], "type": snap["type"], "hash": snap["hash"]}
            if snap["type"] == "image":
                body["size"] = len(snap["png"] or b"")
            else:
                body["text"] = snap["text"] or ""
            return self._send(200, json.dumps(body).encode())
        return self._send(404, b'{"error":"not found"}')

    def do_POST(self):
        if self.path.split("?")[0] != "/set":
            return self._send(404, b'{"error":"not found"}')
        n = int(self.headers.get("Content-Length") or 0)
        raw = self.rfile.read(n) if n else b""
        ctype = (self.headers.get("Content-Type") or "").lower()
        try:
            if ctype.startswith("image/"):
                write_clip("image", None, raw)
                adopt("image", None, raw, from_remote=True)
            else:
                payload = json.loads(raw.decode("utf-8"))
                text = payload.get("text") or ""
                write_clip("text", text, None)
                adopt("text", text, None, from_remote=True)
            with lock:
                rev = state["rev"]
            return self._send(200, json.dumps({"ok": True, "rev": rev}).encode())
        except Exception as exc:
            return self._send(500, json.dumps({"error": str(exc)}).encode())


if __name__ == "__main__":
    threading.Thread(target=watcher, daemon=True).start()
    srv = ThreadingHTTPServer(("0.0.0.0", PORT), Handler)
    srv.serve_forever()
