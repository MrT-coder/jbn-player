//go:build windows

package island

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"jbnplayer/internal/ipc"
	"jbnplayer/internal/player"
	"jbnplayer/internal/settings"
)

// libService returns a service whose active server is a fake Subsonic API.
func libService(t *testing.T) *service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "getAlbumList2"):
			if r.URL.Query().Get("offset") != "30" {
				t.Errorf("offset not forwarded: %v", r.URL.Query())
			}
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","albumList2":{"album":[{"id":"a1","name":"Alb","artist":"Art","coverArt":"al-a1","year":2021}]}}}`))
		case strings.Contains(r.URL.Path, "getAlbum"):
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","album":{"song":[{"id":"s1","title":"One","artist":"Art","album":"Alb","coverArt":"al-a1","duration":90}]}}}`))
		case strings.Contains(r.URL.Path, "search3"):
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","searchResult3":{"album":[{"id":"a1","name":"Alb"}],"song":[{"id":"s1","title":"One"}]}}}`))
		default:
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok"}}`))
		}
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	s := &service{path: filepath.Join(dir, "servers.json"), ipcPath: filepath.Join(dir, "ipc.json"), libDir: dir}
	if msg := s.SaveServer(serverInput{URL: srv.URL, User: "u", Password: "p"}); msg != "" {
		t.Fatal(msg)
	}
	return s
}

func TestLibraryCallsReturnCoverURLsAndTracks(t *testing.T) {
	s := libService(t)
	al := s.Albums("newest", 30)
	if al.Error != "" || len(al.Items) != 1 || !strings.Contains(al.Items[0].Cover, "getCoverArt") {
		t.Fatalf("albums: %+v", al)
	}
	tr := s.AlbumTracks("a1")
	if tr.Error != "" || len(tr.Items) != 1 || tr.Items[0].Title != "One" || tr.Items[0].Cover == "" || tr.Items[0].Duration != 90 {
		t.Fatalf("tracks: %+v", tr)
	}
	sr := s.SearchLibrary("one")
	if sr.Error != "" || len(sr.Albums) != 1 || len(sr.Songs) != 1 {
		t.Fatalf("search: %+v", sr)
	}
}

func TestLibraryWithoutServerReportsError(t *testing.T) {
	dir := t.TempDir()
	s := &service{path: filepath.Join(dir, "servers.json"), ipcPath: filepath.Join(dir, "ipc.json"), libDir: dir}
	if r := s.Albums("newest", 0); r.Error != "no server configured" || r.Items == nil {
		t.Fatalf("got %+v", r)
	}
}

func TestCoverURLsStayStableAcrossCalls(t *testing.T) {
	s := libService(t)
	a := s.Albums("newest", 30).Items[0].Cover
	b := s.Albums("newest", 30).Items[0].Cover
	if a != b {
		t.Fatal("cover URL changes between calls, the browser could not cache it")
	}
}

type ctl struct {
	st    player.State
	queue []player.Track
}

func (c *ctl) State() player.State {
	s := c.st
	s.QueueLen = len(c.queue)
	if len(c.queue) > 0 && c.st.Index < len(c.queue) {
		cur := c.queue[c.st.Index]
		s.Current = &cur
	}
	return s
}
func (c *ctl) Queue(from, n int) player.QueuePage {
	end := from + n
	if end > len(c.queue) {
		end = len(c.queue)
	}
	return player.QueuePage{Tracks: c.queue[from:end], From: from, Total: len(c.queue)}
}
func (c *ctl) TogglePause() error { return nil }
func (c *ctl) Next()              {}
func (c *ctl) Prev()              {}
func (c *ctl) Play(ts []player.Track, start int) error {
	if start < 0 || start >= len(ts) { // the real queue: "any start" is a random track with shuffle, else the first
		start = 0
	}
	c.queue, c.st.Index = ts, start
	return nil
}
func (c *ctl) Enqueue(ts []player.Track) error  { c.queue = append(c.queue, ts...); return nil }
func (c *ctl) PlayNext(ts []player.Track) error { return nil }
func (c *ctl) Jump(i int) error                 { return nil }
func (c *ctl) Remove(i int) error               { return nil }
func (c *ctl) Move(from, to int) error          { return nil }
func (c *ctl) Audio() player.AudioInfo {
	return player.AudioInfo{Volume: c.st.Volume, Device: "auto", Devices: []player.Device{{Name: "auto", Description: "System default"}}}
}
func (c *ctl) SetVolume(l int) error     { c.st.Volume = l; return nil }
func (c *ctl) SetDevice(n string) error  { return nil }
func (c *ctl) SetEQ(e settings.EQ) error { return nil }
func (c *ctl) ClearUpcoming()            {}
func (c *ctl) SetShuffle(on bool)        { c.st.Shuffle = on }

func TestPlayerBridgeRoundTrip(t *testing.T) {
	s := libService(t)
	stop, err := ipc.Serve(s.ipcPath, &ctl{})
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	// Tracks coming from the page carry an extra "cover" field; it must be tolerated.
	tracks := s.AlbumTracks("a1").Items
	st := s.PlayerDo("play", ipc.Request{Tracks: []player.Track{tracks[0].Track}, Start: 0})
	if st.Error != "" || st.QueueLen != 1 || st.Current == nil || st.Current.Title != "One" || st.Cover == "" || st.Current.Cover == "" {
		t.Fatalf("play: %+v", st)
	}
	if st = s.PlayerDo("shuffle", ipc.Request{On: true}); !st.Shuffle {
		t.Fatalf("shuffle: %+v", st)
	}
	if st = s.PlayerState(); st.Error != "" || st.QueueLen != 1 {
		t.Fatalf("state: %+v", st)
	}
	q := s.PlayerQueue(0, 10)
	if q.Error != "" || q.Total != 1 || len(q.Items) != 1 || q.Items[0].Cover == "" {
		t.Fatalf("queue: %+v", q)
	}
}

func TestPlayerBridgeWhenPlayerNotRunning(t *testing.T) {
	s := libService(t) // no ipc server started
	if st := s.PlayerState(); st.Error != "player is not running" {
		t.Fatalf("got %+v", st)
	}
}
