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

## Proposed stack (a starting recommendation, verify before committing)

- **Go**, same as `jbn-music`. Cross-compile/build with a `build.ps1` like the
  sibling project.
- **Audio:** the key open question. Candidates to spike quickly:
  - pure-Go decoders + `oto` (e.g. `gopxl/beep`): no DLLs, but limited formats
    (no native Opus/AAC) — Navidrome can transcode to MP3/Opus via `stream`.
  - `libmpv` via DLL: all formats and gapless, but ships a DLL.
  Pick by measuring idle RAM/CPU and format coverage on this machine.
- **Tray:** a small Go tray library (e.g. `energye/systray`), verify maintenance.
- **Island UI:** WebView2 (preinstalled on Windows 11), opened on demand and
  destroyed on close. Candidates: Wails v2, or a thin `go-webview2` wrapper.
  Compare idle memory of the tray-only state vs. the open window.
- Keep dependencies few. Prefer the standard library where reasonable.

## First steps for the agent

1. Run `mem_current_project` and `mem_search` for "reproductor subsonic" /
   "jbn-player" in Engram: the scope decision is saved there
   (topic `jbn-player/scope-v1`).
2. Check the Go version (`go version`) and that WebView2 is present.
3. Do a **short audio spike** (decode + play a file from Navidrome) and a
   **tray spike**; report idle RAM/CPU of each and recommend the stack.
4. Only then scaffold the module and propose the package layout
   (`cmd/`, `internal/subsonic`, `internal/player`, `internal/offline`,
   `internal/tray`, `internal/ui`).
5. Ask the owner for the Navidrome URL/user/password (never commit them; use a
   `.env`/config file listed in `.gitignore`, plus an `env.example`).

## Working agreements

- Talk to the owner in **Spanish** (Rioplatense, warm). Code, identifiers,
  comments, UI strings, docs and commit messages in **English**.
- Conventional commits, **no AI attribution / Co-Authored-By** lines.
- Ask at most one question at a time, then wait. Keep answers short.
- Never commit secrets. Verify technical claims against docs, not memory.
- Offline downloads must never overwrite or delete anything on the server.
