//go:build windows

// Command jbn-player is a very light tray music player for a Subsonic server.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime/debug"
	"sync/atomic"
	"time"

	"jbnplayer/internal/config"
	"jbnplayer/internal/ipc"
	"jbnplayer/internal/island"
	"jbnplayer/internal/player"
	"jbnplayer/internal/settings"
	"jbnplayer/internal/store"
	"jbnplayer/internal/subsonic"
	"jbnplayer/internal/tray"
)

func main() {
	// A windowsgui process has no console: send logs to a file instead.
	if cache, err := os.UserCacheDir(); err == nil {
		_ = os.MkdirAll(filepath.Join(cache, "jbn-player"), 0o755)
		if f, err := os.OpenFile(filepath.Join(cache, "jbn-player", "jbn-player.log"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			log.SetOutput(f)
		}
	}

	// The popup window runs as a separate, short-lived process.
	if len(os.Args) > 1 && os.Args[1] == "--island" {
		if err := island.Run(); err != nil {
			log.Fatal(err)
		}
		return
	}

	debug.SetGCPercent(20) // trade a little CPU for a smaller resident heap

	exe, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	dir := filepath.Dir(exe)

	serversPath, err := store.DefaultPath()
	if err != nil {
		log.Fatal(err)
	}
	if err := importLegacyEnv(serversPath, dir); err != nil {
		log.Println("import .env:", err)
	}

	// The active server's client; swapped after the island window closes.
	var current atomic.Pointer[subsonic.Client]
	off := newOfflineCtl()
	reload := func() {
		d, err := store.Load(serversPath)
		if err != nil {
			log.Println("load servers:", err)
			current.Store(nil)
			off.bind(nil, nil)
			return
		}
		if s, ok := d.ActiveServer(); ok {
			c := subsonic.New(s.URL, s.User, s.Password)
			current.Store(c)
			off.bind(&s, c)
		} else {
			current.Store(nil)
			off.bind(nil, nil)
		}
	}
	reload()

	settingsPath, _ := settings.DefaultPath()
	cfg, err := settings.Load(settingsPath)
	if err != nil {
		log.Println("settings:", err) // the defaults are used
	}

	p, err := player.New(filepath.Join(dir, "libmpv-2.dll"), func(t player.Track) string {
		if path, ok := off.path(t.ID); ok {
			return path // the downloaded copy plays without the server
		}
		if c := current.Load(); c != nil {
			return c.StreamURL(t.ID)
		}
		return ""
	}, cfg, settingsPath)
	if err != nil {
		log.Fatal(err)
	}
	defer p.Close()
	p.OnTrack(func(t player.Track) { tray.SetTooltip(fmt.Sprintf("%s - %s", t.Artist, t.Title)) })

	// Let the island window (another process) control this player.
	if ipcPath, err := ipc.DefaultPath(); err == nil {
		if stopIPC, err := ipc.Serve(ipcPath, controller{Player: p, off: off}); err == nil {
			defer stopIPC()
		} else {
			log.Println("ipc:", err)
		}
	}

	// The window may change the servers and the accent color: pick both up when it closes.
	appearancePath, _ := settings.AppearancePath()
	accent := func() string {
		a, _ := settings.LoadAppearance(appearancePath) // defaults if missing or corrupt
		return a.Accent
	}
	isl := &islandCtl{exe: exe, onExit: func() {
		reload()
		tray.SetAccent(accent())
	}}

	// Until the island has a library UI, "Play / Pause" on an empty queue starts a random album.
	startRandomAlbum := func() error {
		c := current.Load()
		if c == nil {
			isl.open() // no server yet: go straight to the setup screen
			return fmt.Errorf("no server configured")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		albums, err := c.RandomAlbums(ctx, 1)
		if err != nil || len(albums) == 0 {
			return fmt.Errorf("random album: %v", err)
		}
		songs, err := c.AlbumSongs(ctx, albums[0].ID)
		if err != nil {
			return err
		}
		tracks := make([]player.Track, len(songs))
		for i, s := range songs {
			tracks[i] = player.Track{ID: s.ID, Title: s.Title, Artist: s.Artist, Album: s.Album, CoverArt: s.CoverArt, Duration: s.Duration}
		}
		return p.Play(tracks, 0)
	}

	if current.Load() == nil {
		isl.open() // first run: show the server setup right away
	}

	tray.Run(tray.Handlers{
		PlayPause: func() {
			if !p.HasQueue() {
				if err := startRandomAlbum(); err != nil {
					log.Println(err)
				}
				return
			}
			if err := p.TogglePause(); err != nil {
				log.Println(err)
			}
		},
		Next:   p.Next,
		Prev:   p.Prev,
		Open:   func() { isl.open() },
		Click:  isl.toggle,
		Accent: accent(),
	}, isl.kill)
}

// importLegacyEnv turns the old .env file into the first saved server (once,
// only while no server is configured), so existing setups keep working.
func importLegacyEnv(serversPath, exeDir string) error {
	d, err := store.Load(serversPath)
	if err != nil || len(d.Servers) > 0 {
		return err
	}
	cfg, err := config.Load(filepath.Join(exeDir, ".env"))
	if err != nil {
		cfg, err = config.Load(".env")
	}
	if err != nil {
		return nil // nothing to import
	}
	id := store.NewID()
	return store.Save(serversPath, store.Data{
		Servers: []store.Server{{ID: id, Type: store.TypeSubsonic, Name: "Navidrome", URL: cfg.URL, User: cfg.User, Password: cfg.Password}},
		Active:  id,
	})
}
