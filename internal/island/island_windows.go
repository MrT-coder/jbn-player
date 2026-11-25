//go:build windows

// Package island is the small popup opened from the tray icon. It runs in its
// own short-lived process (jbn-player.exe --island): when the window closes
// the process exits and every WebView2 child process goes with it.
package island

import (
	"embed"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/jchv/go-webview2"

	"jbnplayer/internal/ipc"
	"jbnplayer/internal/settings"
	"jbnplayer/internal/store"
)

//go:embed ui/index.html ui/style.css ui/app.js ui/vendor/preact-htm.js
var uiFS embed.FS

// page inlines CSS and scripts into one document: SetHtml has no origin, so
// relative file references would not load.
func page() (string, error) {
	read := func(name string) (string, error) {
		b, err := uiFS.ReadFile("ui/" + name)
		return string(b), err
	}
	index, err := read("index.html")
	if err != nil {
		return "", err
	}
	for _, a := range [][2]string{{"/*__CSS__*/", "style.css"}, {"/*__VENDOR__*/", "vendor/preact-htm.js"}, {"/*__APP__*/", "app.js"}} {
		body, err := read(a[1])
		if err != nil {
			return "", err
		}
		index = strings.Replace(index, a[0], body, 1)
	}
	return index, nil
}

// Run opens the popup and blocks until it is closed.
func Run() error {
	runtime.LockOSThread() // the window, its hook and its message loop must share one OS thread
	path, err := store.DefaultPath()
	if err != nil {
		return err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	// A popup that was just closed may still be shutting its WebView2 down and
	// holding the data folder: retry for a few seconds before giving up.
	unhook := placeAtCreation() // the window is born in its final place, no jump from the left
	var w webview2.WebView
	for attempt := 0; attempt < 8 && w == nil; attempt++ {
		if attempt > 0 {
			time.Sleep(400 * time.Millisecond)
		}
		w = webview2.NewWithOptions(webview2.WebViewOptions{
			AutoFocus: true,
			DataPath:  filepath.Join(cache, "jbn-player", "webview"),
			WindowOptions: webview2.WindowOptions{
				Title: "jbn-player", Width: popupW, Height: popupH,
			},
		})
	}
	unhook()
	if w == nil {
		return errors.New("WebView2 runtime is not available")
	}
	defer w.Destroy()

	ipcPath, err := ipc.DefaultPath()
	if err != nil {
		return err
	}
	closeWindow := func() { w.Dispatch(w.Terminate) }
	if err := w.Bind("closeIsland", closeWindow); err != nil {
		return err
	}
	if err := w.Bind("startDrag", func() { dragWindow(uintptr(w.Window())) }); err != nil {
		return err
	}
	stylePopup(uintptr(w.Window()))
	watchFocus(uintptr(w.Window()), closeWindow)

	appearancePath, err := settings.AppearancePath()
	if err != nil {
		return err
	}
	svc := &service{path: path, ipcPath: ipcPath, libDir: filepath.Join(cache, "jbn-player"), appearancePath: appearancePath}
	for name, fn := range map[string]any{
		"getServers":     svc.GetServers,
		"testServer":     svc.TestServer,
		"saveServer":     svc.SaveServer,
		"deleteServer":   svc.DeleteServer,
		"setActive":      svc.SetActive,
		"albums":         svc.Albums,
		"albumTracks":    svc.AlbumTracks,
		"searchLibrary":  svc.SearchLibrary,
		"playerState":    svc.PlayerState,
		"playerQueue":    svc.PlayerQueue,
		"getAppearance":  svc.GetAppearance,
		"setAppearance":  svc.SetAppearance,
		"audioInfo":      svc.AudioInfo,
		"audioDo":        svc.AudioDo,
		"syncInfo":       svc.SyncInfo,
		"syncLibrary":    svc.SyncLibrary,
		"songs":          svc.Songs,
		"playlists":      svc.Playlists,
		"playlistTracks": svc.PlaylistTracks,
		"playAll":        svc.PlayAll,
		"playerDo":       svc.PlayerDo,
		"offlineStatus":  svc.OfflineStatus,
		"offlineIDs":     svc.OfflineIDs,
		"offlineDo":      svc.OfflineDo,
		"offlineAll":     svc.OfflineAll,
		"getSongkey":     svc.GetSongkey,
		"saveSongkey":    svc.SaveSongkey,
		"songkeyAdd":     svc.SongkeyAdd,
		"songkeyJobs":    svc.SongkeyJobs,
		"songkeyDismiss": svc.SongkeyDismiss,
		"getHotkey":      svc.GetHotkey,
		"hotkeyDo":       svc.HotkeyDo,
	} {
		if err := w.Bind(name, fn); err != nil {
			return err
		}
	}
	doc, err := page()
	if err != nil {
		return err
	}
	w.SetHtml(doc)
	w.Run()
	return nil
}
