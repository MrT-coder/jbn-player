# jbn-player

A very light Windows music player that lives in the system tray and plays from
your own [Navidrome](https://www.navidrome.org/) server (Subsonic API).

Think of it as a desktop companion for people who already stream their library
from Navidrome on their phone and want the same thing on the PC without running
a heavy app all day.

> Early version (v0.x): expect rough edges.

## What problem it solves

Most desktop players are either a full app that sits on hundreds of MB of RAM,
or a browser tab you have to keep open. jbn-player takes a different approach:

- **Light as a process.** The background process only plays audio: a tray icon
  and a menu. No UI toolkit is loaded while it is idle, and CPU stays near 0%
  while a song plays.
- **A small popup instead of a full app.** Left-click the tray icon and a compact
  window opens in the bottom-right corner. When you close it, the window process
  exits and its memory is released.
- **Works when the server is off.** Mark songs, albums or playlists as offline
  and they play from local files. If a local copy exists it is used; otherwise
  the song is streamed.

## Features

- Browse albums, search, all-songs list and playlists
- One play queue: play next, add to queue, shuffle, drag to reorder; clicking a
  song plays it inside the queue you already have instead of replacing it
- Seek bar in Now playing and a progress line on the mini player
- Offline copies per song, album or playlist, with "Download all" and a select
  mode; only fetches what is not stored yet; custom offline folder
- Add music from YouTube through [songkey](https://github.com/MrT-coder/songkey):
  paste a link in Playlists and follow it until the song reaches your library
  playlist (set the songkey URL and API token in Settings; the token is stored
  encrypted and never reaches the window's web page)
- A global keyboard shortcut (Ctrl+Alt+J by default) opens and closes the
  popup from anywhere, even inside a game; change it or turn it off in
  Settings > Shortcut, which refuses a combination another application already
  registered
- Volume, output device selection and equalizer
- Accent color and background tone
- Multiple formats and gapless playback (through libmpv)
- Passwords and the songkey token are stored encrypted with Windows DPAPI

## Install

Requires Windows 11 (64-bit) and a Navidrome server you can reach by URL.

Open PowerShell and run:

```powershell
irm https://raw.githubusercontent.com/MrT-coder/jbn-player/main/install.ps1 | iex
```

It installs to `%LOCALAPPDATA%\Programs\jbn-player` (no admin needed), downloads
the audio engine (`libmpv-2.dll`) from its upstream build project, adds a Start
menu shortcut and starts the player. Run it again to update.

Prefer to read before you run? The script is [`install.ps1`](install.ps1). You can
also download `jbn-player-windows-x64.zip` from the Releases page and put
`libmpv-2.dll` next to `jbn-player.exe` yourself.

## Use

1. Start jbn-player and look for the tray icon.
2. Left-click the icon, or press Ctrl+Alt+J, to open the popup (right-click for
   the menu: play/pause, next, previous, open, quit).
3. First run opens the server setup: enter your Navidrome URL, user and password.
4. Run a library sync, then browse or search and play.
5. For offline use, use the download action on a song, album or playlist, or the
   "Download all" / "Select" buttons in the Songs tab.

Settings live in `%APPDATA%\jbn-player`. Offline songs are stored in
`%LOCALAPPDATA%\jbn-player\offline\<server id>` unless you choose another folder
in Settings > Offline.

## Privacy and safety

- It only talks to the server you configure. No telemetry, no accounts, no
  third-party services.
- Server passwords are encrypted with DPAPI, so they are tied to your Windows
  user. They are never stored in the repo.
- The popup and the tray process talk over a loopback-only HTTP endpoint that
  requires a random token readable only by your Windows user.
- Offline downloads are read-only: the player never modifies or deletes anything
  on the server.

## How it works

- **Go, no cgo.** One static executable.
- **Audio:** `libmpv-2.dll`, loaded at runtime with
  [purego](https://github.com/ebitengine/purego).
- **Tray:** [energye/systray](https://github.com/energye/systray).
- **Popup UI:** WebView2 ([go-webview2](https://github.com/jchv/go-webview2)) with
  Preact + htm vendored (no build step), running as a short-lived process
  (`jbn-player.exe --island`).

Rough numbers from early measurements (10 s samples, noisy): libmpv playing is
about 45 MB RAM and 0.2-0.3% CPU.

```
cmd/jbn-player        entry point, wiring
internal/store        saved servers (DPAPI-encrypted passwords)
internal/subsonic     Subsonic API client (token auth)
internal/settings     volume, output device, equalizer, appearance
internal/library      local copy of songs + playlists (sync), search
internal/player       libmpv wrapper + queue
internal/offline      offline copies
internal/ipc          loopback HTTP + token between the popup and the player
internal/tray         tray icon and menu
internal/hotkey       global shortcut that toggles the popup (RegisterHotKey)
internal/island       popup window (WebView2) and its UI
spikes/               throwaway measurements; own modules
```

## Build from source

Requires Go and the WebView2 runtime (included in Windows 11).

1. Put `libmpv-2.dll` in `bin/` (from the
   [shinchiro mpv-winbuild](https://github.com/shinchiro/mpv-winbuild-cmake/releases)
   `mpv-dev-x86_64` archive).
2. Run `./build.ps1` (runs the tests, then builds `bin/jbn-player.exe`).
3. Optional: `./package.ps1` builds the release zip in `dist/`.

Instead of the UI setup you can copy `env.example` to `.env` (next to the exe);
it is imported once as the first server. Never commit `.env`.

## Not planned for now

Linux, macOS and mobile; downloading new music into the server; remote access
(the server just needs to be reachable by URL).

## Licenses

jbn-player is released under the [MIT License](LICENSE).

`libmpv` is **not** part of this repository and is not redistributed here. The
installer downloads it from its own project, and it has its own license (mpv
builds are typically GPL/LGPL; check the build you use).

This project is not affiliated with Navidrome or Symfonium.
