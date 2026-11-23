# jbn-player

A very light Windows tray music player for a Navidrome (Subsonic API) server.

## Status

Working: tray popup (bottom-right corner, draggable), libmpv playback with ONE
queue (play next, add to queue, shuffle, reorder by drag and drop), own volume,
output device, equalizer, accent color and background tone, album browse/search, all-songs list, playlists, library sync, server setup, offline copies (download a song, album or playlist; playback prefers the local file and falls back to streaming).

## Stack

- Go, no cgo. Audio via `libmpv-2.dll` loaded at runtime with `purego`
  (all formats, gapless, ~0.2% CPU while playing in the spikes).
- Tray: `energye/systray`.
- Island UI (planned): WebView2, created on open and destroyed on close.

## Layout

```
cmd/jbn-player        entry point, wiring
internal/config       legacy .env import
internal/store        saved servers (DPAPI-encrypted passwords)
internal/subsonic     Subsonic API client (token auth)
internal/settings     volume, output device, equalizer (settings.json), accent/background (appearance.json)
internal/library      local copy of songs + playlists (Sync), search, A-Z
internal/player       libmpv wrapper + queue (shuffle, play next, enqueue)
internal/ipc          loopback HTTP + token: island process -> player process
internal/tray         tray icon and menu
internal/island       tray popup: WebView2 window (own process, Preact+htm UI in ui/)
spikes/               throwaway measurements (audio, tray, mpv); own modules
```

## Run

1. Put `libmpv-2.dll` in `bin/` (shinchiro `mpv-dev-x86_64` release).
2. `./build.ps1`, then run `bin/jbn-player.exe`.
3. Left-click the tray icon: the popup opens next to it (right-click for the menu).
   The first run opens the server setup. (An old `.env` with
   `NAVIDROME_*` values is imported once as the first server.)

## Measured (spikes, 10 s samples, noisy)

| | RAM (working set) | CPU |
|---|---|---|
| beep/oto, playing | ~28-33 MB | ~1.4-1.6% |
| libmpv, playing | ~45 MB | ~0.2-0.3% |
| scaffold app, idle | ~62 MB | 0% |
