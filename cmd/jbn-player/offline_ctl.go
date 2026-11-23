//go:build windows

package main

import (
	"context"
	"io"
	"log"
	"sync"
	"sync/atomic"

	"jbnplayer/internal/offline"
	"jbnplayer/internal/player"
	"jbnplayer/internal/store"
	"jbnplayer/internal/subsonic"
)

// controller is what the island window controls: the player plus the offline copies.
type controller struct {
	*player.Player
	off *offlineCtl
}

func (c controller) OfflineStatus() offline.Status {
	s := c.off.mgr.Status()
	s.Dir = offline.LoadRoot()
	s.CustomDir = s.Dir != offline.DefaultRoot()
	return s
}
func (c controller) OfflineAdd(items []offline.Item) error { c.off.mgr.Add(items); return nil }
func (c controller) OfflineRemove(ids []string) error      { return c.off.mgr.Remove(ids) }
func (c controller) OfflineClear() error                   { return c.off.mgr.Clear() }
func (c controller) OfflineCancel()                        { c.off.mgr.Cancel() }
func (c controller) OfflineSetDir(dir string) error        { return c.off.setRoot(dir) }

// offlineCtl keeps the offline folder and downloader pointed at the active server.
type offlineCtl struct {
	mgr   *offline.Manager
	store atomic.Pointer[offline.Store]

	mu     sync.Mutex
	key    string // identifies the server and folder the manager is bound to
	server *store.Server
	client *subsonic.Client
}

func newOfflineCtl() *offlineCtl { return &offlineCtl{mgr: offline.NewManager()} }

// bind follows the active server (nil = none). It rebinds only when the server
// or the folder changed, so closing the window does not cancel downloads in progress.
func (o *offlineCtl) bind(s *store.Server, c *subsonic.Client) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.server, o.client = s, c
	o.rebindLocked()
}

func (o *offlineCtl) rebindLocked() {
	unbind := func() {
		o.key = ""
		o.store.Store(nil)
		o.mgr.Bind(nil, nil)
	}
	s, c := o.server, o.client
	if s == nil {
		unbind()
		return
	}
	root := offline.LoadRoot()
	key := s.ID + "|" + s.URL + "|" + s.User + "|" + s.Password + "|" + root
	if key == o.key {
		return
	}
	st, err := offline.Open(offline.Dir(root, s.ID))
	if err != nil {
		log.Println("offline:", err)
		unbind()
		return
	}
	o.key = key
	o.store.Store(st)
	o.mgr.Bind(st, func(ctx context.Context, id string) (io.ReadCloser, string, error) {
		return c.Download(ctx, id)
	})
}

// setRoot switches the folder offline copies live in ("" = default). Songs
// already stored in the old folder stay there; each folder keeps its own list.
func (o *offlineCtl) setRoot(dir string) error {
	if err := offline.SaveRoot(dir); err != nil {
		return err
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.rebindLocked()
	return nil
}

// path returns the local file of a song, if it was downloaded.
func (o *offlineCtl) path(id string) (string, bool) {
	if st := o.store.Load(); st != nil {
		return st.Path(id)
	}
	return "", false
}
