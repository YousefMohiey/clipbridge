# ClipBridge protocol v1

Everything runs on the local network. One device is the host, the rest are
clients. Clients poll, so no client has to accept an inbound connection.

Port: **1717**

## Endpoints

`GET /health`
Returns `{"ok": true}`. Used to confirm the host is awake.

`GET /state`
Reports the current clipboard revision.

```json
{"rev": 12, "type": "text", "hash": "…", "text": "hello"}
{"rev": 13, "type": "image", "hash": "…", "size": 184320}
```

`type` is `text`, `image` or null. `rev` increases on every change. `hash` is
the SHA-256 of the content, hex encoded, with the type prepended.

`GET /image`
Returns the current PNG bytes when `type` is `image`.

`POST /set`
Sets the host clipboard. Content type decides what it is.

- `application/json` with `{"text": "..."}` for text
- `image/png` with raw bytes for an image

Returns `{"ok": true, "rev": N}`.

`GET /download/<name>`
Serves files from the `dist/` directory. Used to hand out installers.

## Client loop

Every 400 to 700 ms:

1. Read the local clipboard.
2. Hash it. If the hash differs from the last one handled, `POST /set`.
3. `GET /state`. If `rev` changed and the hash differs from the last one
   handled, apply it locally, then remember that hash.

Remembering hashes on both ends is what stops a clip from bouncing back and
forth between the machines. It also means no origin flag is needed in the
protocol.

## Sizes

Screenshots are commonly 1 to 5 MB. The host keeps the current clip in memory
only. It does not keep a history, which keeps the privacy story simple: the
clipboard is as private as the clipboard it mirrors.
