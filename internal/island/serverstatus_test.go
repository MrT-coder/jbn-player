//go:build windows

package island

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"jbnplayer/internal/ipc"
	"jbnplayer/internal/library"
	"jbnplayer/internal/offline"
)

// downService returns a service whose server is unreachable, with a synced
// library of 4 songs (2 albums + 1 song outside them) of which s1, s2 and s4
// are stored offline.
func downService(t *testing.T) *service {
	t.Helper()
	root := useRoot(t)
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close() // nothing listens there any more
	dir := t.TempDir()
	s := &service{path: filepath.Join(dir, "servers.json"), ipcPath: filepath.Join(dir, "ipc.json"), libDir: dir}
	if msg := s.SaveServer(serverInput{URL: url, User: "u", Password: "p"}); msg != "" {
		t.Fatal(msg)
	}
	sv, _ := s.activeServer()
	c := &library.Cache{Version: 1, Songs: []library.Song{
		{ID: "s1", Title: "Alpha", Artist: "Ann", Album: "Day", AlbumID: "d", CoverArt: "cd", Track: 2},
		{ID: "s2", Title: "Beta", Artist: "Ann", Album: "Day", AlbumID: "d", CoverArt: "cd", Track: 1},
		{ID: "s3", Title: "Gamma", Artist: "Bob", Album: "Night", AlbumID: "n", CoverArt: "cn", Track: 1},
		{ID: "s4", Title: "Delta", Artist: "Cid", Album: "Dawn", AlbumID: "w", CoverArt: "cw", Track: 1},
	}, Playlists: []library.Playlist{
		{ID: "p1", Name: "Mixed", SongIDs: []string{"s3", "s1"}},
		{ID: "p2", Name: "Remote only", SongIDs: []string{"s3"}},
	}}
	if err := c.Save(s.libPath(sv.ID)); err != nil {
		t.Fatal(err)
	}
	st, err := offline.Open(offline.Dir(root, sv.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s1", "s2", "s4"} {
		if _, err := st.Save(id, "mp3", strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
	}
	return s
}

func ids(ts []trackView) string {
	out := make([]string, len(ts))
	for i, t := range ts {
		out[i] = t.ID
	}
	return strings.Join(out, ",")
}

func TestServerStatusOnlineAndOffline(t *testing.T) {
	if st := libService(t).ServerStatus(); st.Offline || st.Error != "" {
		t.Fatalf("reachable server: %+v", st)
	}
	if st := downService(t).ServerStatus(); !st.Offline || st.Error != "" {
		t.Fatalf("unreachable server: %+v", st)
	}
}

func TestServerStatusWithoutServerIsNotOffline(t *testing.T) {
	dir := t.TempDir()
	s := &service{path: filepath.Join(dir, "servers.json"), libDir: dir}
	if st := s.ServerStatus(); st.Offline {
		t.Fatalf("no server is a setup problem, not offline: %+v", st)
	}
}

func TestOfflineShowsOnlyStoredSongsWithoutCovers(t *testing.T) {
	s := downService(t)
	s.ServerStatus()

	songs := s.Songs("", 0, 100)
	if songs.Error != "" || songs.Total != 3 || ids(songs.Items) != "s1,s2,s4" {
		t.Fatalf("songs: %+v", songs)
	}
	for _, tv := range songs.Items {
		if tv.Cover != "" {
			t.Fatalf("offline covers must be empty (the server cannot serve them): %+v", tv)
		}
	}
	if q := s.Songs("delta", 0, 100); q.Total != 1 || ids(q.Items) != "s4" {
		t.Fatalf("search inside songs: %+v", q)
	}

	pl := s.Playlists()
	if pl.Error != "" || len(pl.Items) != 1 || pl.Items[0].ID != "p1" || pl.Items[0].Count != 1 || pl.Items[0].Cover != "" {
		t.Fatalf("playlists: %+v", pl)
	}
	if tr := s.PlaylistTracks("p1"); tr.Error != "" || ids(tr.Items) != "s1" {
		t.Fatalf("playlist tracks: %+v", tr)
	}
}

func TestOfflineAlbumsAndSearchComeFromTheLocalCopy(t *testing.T) {
	s := downService(t)
	// No ServerStatus call first: the failed live request finds out by itself.
	al := s.Albums("newest", 0)
	if al.Error != "" || !al.Offline || len(al.Items) != 2 {
		t.Fatalf("albums: %+v", al)
	}
	if al.Items[0].Name != "Dawn" || al.Items[1].Name != "Day" || al.Items[1].SongCount != 2 || al.Items[1].Cover != "" {
		t.Fatalf("albums order/content: %+v", al.Items)
	}
	if more := s.Albums("newest", 30); len(more.Items) != 0 || more.Error != "" {
		t.Fatalf("second page: %+v", more)
	}
	if tr := s.AlbumTracks("d"); tr.Error != "" || ids(tr.Items) != "s2,s1" {
		t.Fatalf("album tracks in track order: %+v", tr)
	}
	sr := s.SearchLibrary("ann")
	if sr.Error != "" || !sr.Offline || ids(sr.Songs) != "s1,s2" || len(sr.Albums) != 1 || sr.Albums[0].ID != "d" {
		t.Fatalf("search: %+v", sr)
	}
	if sr := s.SearchLibrary("gamma"); len(sr.Songs) != 0 || len(sr.Albums) != 0 {
		t.Fatalf("a song that is not stored is not offered: %+v", sr)
	}
}

func TestOfflinePlayAllQueuesOnlyStoredSongs(t *testing.T) {
	s := downService(t)
	s.ServerStatus()
	c := &ctl{}
	stop, err := ipc.Serve(s.ipcPath, c)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()
	if st := s.PlayAll(false); st.Error != "" {
		t.Fatal(st.Error)
	}
	if len(c.queue) != 3 {
		t.Fatalf("queued %+v, want the 3 stored songs", c.queue)
	}
}

func TestOfflineAllRefusesWhileOffline(t *testing.T) {
	s := downService(t)
	s.ServerStatus()
	if r := s.OfflineAll(""); r.Error == "" {
		t.Fatalf("downloading needs the server: %+v", r)
	}
}

func TestOnlineLibraryIsUnchanged(t *testing.T) {
	s := libService(t)
	if st := s.ServerStatus(); st.Offline {
		t.Fatal("should be online")
	}
	if al := s.Albums("newest", 30); al.Offline || len(al.Items) != 1 || al.Items[0].Cover == "" {
		t.Fatalf("online albums: %+v", al)
	}
}
