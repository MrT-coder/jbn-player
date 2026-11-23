# jbn-player — project context

Read this file fully before doing anything. It is the hand-off from the session
that created the project.

## What we are building

A **very light Windows desktop music player** that is a client of the owner's
**Navidrome** server (Subsonic API). It is a "port" to the PC of the Android app
the owner already uses, **Symfonium** (https://symfonium.app/).

Hard priorities, in order:

1. **Super light as a process.** Low idle RAM and ~0% CPU while playing in the
   background. This beats features.
2. **Simple to use.** Small surface, few clicks.
3. **Works offline.** When the server is off, downloaded music still plays.

## UX model (decided by the user)

- **Background listening = system tray.** A minimal always-resident process that
  only plays audio: tray icon + menu (play/pause, next, previous, open, quit).
  No UI toolkit loaded while idle.
- **Interaction = a small "island" window, not a full app.** When the user wants
  to browse, change song, manage downloads, etc., a small compact window opens.
  It is created on open and **destroyed on close** so no WebView stays in memory.
- Contents of the island (v1): library browse (artists / albums / playlists),
  search, queue, now playing with cover art, offline/download management.

## v1 scope

- Authenticate against Navidrome via the Subsonic API and browse the library.
- Stream songs from the server.
- **Offline cache, Symfonium style:** mark a song, album or playlist as offline;
  files are stored in a local folder and played without the server. Playback
  should prefer the local copy automatically, and fall back to streaming.
- Tray + island UX above.

### Explicitly OUT of v1

- Downloading from YouTube / adding new music to the server. Later idea: call
  the owner's `jbn-music` service (`GET /add?url=...&token=...`) from the island.
  Do not build it now.
- Mobile, Linux, macOS. Target is **Windows 11** only.
- Remote access (Tailscale etc.). Assume the server is reachable by URL.

## The server side (already exists, do not modify)

- **Navidrome** exposes the Subsonic API (`/rest/...`). The owner's playlist
  `DETODO` holds the whole library.
- Auth is Subsonic token auth: `t = md5(password + salt)`, random `s` (salt) per
  request, plus `u`, `v=1.16.1`, `c=<client name>`, `f=json`.
- Endpoints relevant to this app: `ping`, `getArtists`, `getArtist`, `getAlbum`,
  `getAlbumList2`, `getPlaylists`, `getPlaylist`, `search3`, `stream`
  (supports `format`/`maxBitRate` transcoding), `download` (original file),
  `getCoverArt`, `scrobble`. Verify each against the Navidrome docs before
  relying on it (https://www.navidrome.org/docs/developers/subsonic-api/).
- Reference implementation of the auth + envelope decoding in Go lives in the
  sibling project: `../jbn-music/internal/navidrome/client.go`. Reuse the
  approach; do not import it (separate module).

## Related project in this repo

`../jbn-music` (module "songkey") is a Go HTTP gate that checks for duplicates
before MeTube downloads a song and files it into Navidrome. It is **not** a
player. Read its `README.md` to see the owner's style: dependency-free Go,
tiny footprint, built for very weak hardware, plain and well-documented code.

## Stack (decided and measured; see README.md for the numbers)

- **Go, no cgo.** Audio through `libmpv-2.dll` loaded at runtime with
  `ebitengine/purego` (all formats, gapless, ~0.2% CPU while playing).
- **Tray:** `energye/systray`. Left click opens the popup, right click the menu.
- **Popup UI:** WebView2 via `jchv/go-webview2`, Preact + htm vendored (no build
  step). It runs as its own short-lived process (`jbn-player.exe --island`) that
  exits when the window closes; the tray process is the only long-lived one and
  the popup talks to it over a loopback HTTP + token IPC (`internal/ipc`).
- Keep dependencies few. Prefer the standard library where reasonable.

## Where we left off (updated 2026-11-23)

**Working copy: `C:\workspace\JBMLLNUBE\jbn-player`.** Origin is the private repo
`MrT-coder/jbn-player` (ssh). The old copy on `E:` is stale: that USB drive logs
disk controller errors, do not work from it.

Done: tray popup (opens in the bottom-right corner via a WH_CBT creation hook, no
jump), one play queue (play next, enqueue, shuffle, drag to reorder), volume /
output device / equalizer, accent + background tone, album browse and search,
all-songs list, playlists, library sync, server setup (DPAPI-encrypted
passwords), and **offline copies**: per song, album or playlist; bulk "Download
all" and "Select" mode on the Songs tab (only fetches what is not stored yet);
Stop button; custom offline folder (Settings > Offline); playback prefers the
local file and falls back to streaming; a batch aborts after 5 consecutive
failures.

Not verified yet (ask the owner to test): playing a downloaded song with the
server OFF, and the visual result of Select mode.

Ideas, nothing decided: re-sync an offline playlist when it changes on the
server, a storage cap, moving files when the offline folder changes (today the
old files stay where they are and each folder keeps its own index).

How to run: copy `libmpv-2.dll` into `bin/` (gitignored), add a server from the
Settings screen or copy a `.env` (see `env.example`), run `./build.ps1`, start
`bin/jbn-player.exe`. Settings live in `%APPDATA%\jbn-player`, offline songs in
`%LOCALAPPDATA%\jbn-player\offline\<server id>` unless a custom folder is set.

Gotchas:
- `.gitignore` rules for `bin/`, `data/`, `offline/` are anchored to the root on
  purpose: an unanchored `offline/` once hid `internal/offline`.
- `go test -race` needs a C compiler that is not installed; run plain `go test ./...`.
- When changing the popup UI you cannot click it from here unless the Claude in
  Chrome extension is connected; headless Edge (`--dump-dom` with a script that
  writes measurements into the DOM) works to measure layout.
- Engram: the decisions of this project were saved under the project name
  `jbn-server` (topic keys `jbn-player/*`), not `jbn-player`. Search there:
  `mem_search` with `project: "jbn-server"` and query `jbn-player`.

## Working agreements

- Talk to the owner in **Spanish** (Rioplatense, warm). Code, identifiers,
  comments, UI strings, docs and commit messages in **English**.
- Conventional commits, **no AI attribution / Co-Authored-By** lines.
- Ask at most one question at a time, then wait. Keep answers short.
- Never commit secrets. Verify technical claims against docs, not memory.
- Offline downloads must never overwrite or delete anything on the server.
