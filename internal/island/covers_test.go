//go:build windows

package island

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"jbnplayer/internal/library"
	"jbnplayer/internal/offline"
)

// coverService is online, with s1 and s2 (same album cover "al-1") stored and
// s3 (cover "al-3") only on the server. It counts getCoverArt requests.
func coverService(t *testing.T) (*service, *atomic.Int32, string) {
	t.Helper()
	root := useRoot(t)
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "getCoverArt") {
			hits.Add(1)
			_, _ = w.Write([]byte("jpeg:" + r.URL.Query().Get("id")))
			return
		}
		_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok"}}`))
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	s := &service{path: filepath.Join(dir, "servers.json"), ipcPath: filepath.Join(dir, "ipc.json"), libDir: dir}
	if msg := s.SaveServer(serverInput{URL: srv.URL, User: "u", Password: "p"}); msg != "" {
		t.Fatal(msg)
	}
	sv, _ := s.activeServer()
	c := &library.Cache{Version: 1, Songs: []library.Song{
		{ID: "s1", Title: "One", AlbumID: "a1", CoverArt: "al-1"},
		{ID: "s2", Title: "Two", AlbumID: "a1", CoverArt: "al-1"},
		{ID: "s3", Title: "Three", AlbumID: "a3", CoverArt: "al-3"},
	}}
	if err := c.Save(s.libPath(sv.ID)); err != nil {
		t.Fatal(err)
	}
	st, err := offline.Open(offline.Dir(root, sv.ID))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"s1", "s2"} {
		if _, err := st.Save(id, "mp3", strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
	}
	return s, &hits, offline.CoversDir(offline.Dir(root, sv.ID))
}

func TestBackfillStoresOneCoverPerStoredAlbumOnce(t *testing.T) {
	s, hits, covers := coverService(t)
	s.backfillCovers()
	raw, err := os.ReadFile(filepath.Join(covers, "al-1.jpg"))
	if err != nil || string(raw) != "jpeg:al-1" {
		t.Fatalf("cover: %q %v", raw, err)
	}
	if _, err := os.Stat(filepath.Join(covers, "al-3.jpg")); !os.IsNotExist(err) {
		t.Fatalf("a song that is not stored needs no cover: %v", err)
	}
	if hits.Load() != 1 {
		t.Fatalf("requests: %d, want 1 (two songs share the cover)", hits.Load())
	}
	s.backfillCovers()
	if hits.Load() != 1 {
		t.Fatalf("a stored cover is not fetched again: %d requests", hits.Load())
	}
}

func TestOfflineCoversAreServedFromTheLocalCopy(t *testing.T) {
	s := downService(t)
	sv, _ := s.activeServer()
	covers := offline.CoversDir(offline.Dir(offline.LoadRoot(), sv.ID))
	if err := os.MkdirAll(covers, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(covers, "cd.jpg"), []byte("day-cover"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.ServerStatus()
	t.Cleanup(s.closeCovers)

	songs := s.Songs("", 0, 100) // s1, s2 (album "cd"), s4 (cover "cw", not stored)
	if songs.Items[0].Cover == "" || !strings.HasPrefix(songs.Items[0].Cover, "http://127.0.0.1:") {
		t.Fatalf("stored cover must be served locally: %+v", songs.Items[0])
	}
	if songs.Items[2].Cover != "" {
		t.Fatalf("no local cover means the placeholder: %+v", songs.Items[2])
	}
	res, err := http.Get(songs.Items[0].Cover)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(res.Body)
	res.Body.Close()
	if res.StatusCode != 200 || string(body) != "day-cover" {
		t.Fatalf("GET cover: %d %q", res.StatusCode, body)
	}
	if al := s.Albums("newest", 0); al.Items[1].Cover == "" {
		t.Fatalf("offline albums get the local cover too: %+v", al.Items)
	}

	base := songs.Items[0].Cover[:strings.LastIndex(songs.Items[0].Cover, "/")+1]
	for _, bad := range []string{"..%2Fservers", "cw", "a%5Cb"} {
		res, err := http.Get(base + bad)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != http.StatusNotFound {
			t.Fatalf("%s: status %d, want 404", bad, res.StatusCode)
		}
	}
}
