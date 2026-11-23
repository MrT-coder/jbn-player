//go:build windows

package island

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"jbnplayer/internal/ipc"
)

// syncService returns a service whose server serves 3 songs and 1 playlist.
func syncService(t *testing.T) *service {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "search3"):
			if r.URL.Query().Get("query") != "" {
				t.Errorf("sync must use an empty query: %v", r.URL.Query())
			}
			if r.URL.Query().Get("songOffset") != "0" {
				_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","searchResult3":{}}}`))
				return
			}
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","searchResult3":{"song":[
				{"id":"s1","title":"Zeta","artist":"A","coverArt":"c1","duration":10},
				{"id":"s2","title":"Águila","artist":"B","coverArt":"c2","duration":20},
				{"id":"s3","title":"Mar","artist":"C","coverArt":"c3","duration":30}]}}}`))
		case strings.Contains(r.URL.Path, "getPlaylists"):
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","playlists":{"playlist":[{"id":"p1","name":"Mix","songCount":2}]}}}`))
		case strings.Contains(r.URL.Path, "getPlaylist"):
			_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","playlist":{"entry":[{"id":"s3","title":"Mar","coverArt":"c3"},{"id":"s1","title":"Zeta","coverArt":"c1"}]}}}`))
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

func TestBeforeFirstSyncNothingIsAvailable(t *testing.T) {
	s := syncService(t)
	if info := s.SyncInfo(); info.Synced || info.Error != "" {
		t.Fatalf("info: %+v", info)
	}
	if r := s.Songs("", 0, 10); r.Synced || len(r.Items) != 0 || r.Error != "" {
		t.Fatalf("songs: %+v", r)
	}
	if r := s.Playlists(); r.Synced || len(r.Items) != 0 {
		t.Fatalf("playlists: %+v", r)
	}
	if r := s.PlayAll(true); r.Error != "library not synced yet" {
		t.Fatalf("play all: %+v", r)
	}
}

func TestSyncThenBrowseSearchAndPlaylists(t *testing.T) {
	s := syncService(t)
	info := s.SyncLibrary()
	if info.Error != "" || !info.Synced || info.Songs != 3 || info.Playlists != 1 || info.At == "" {
		t.Fatalf("sync: %+v", info)
	}
	all := s.Songs("", 0, 10)
	if all.Total != 3 || all.Items[0].Title != "Águila" || all.Items[2].Title != "Zeta" {
		t.Fatalf("songs must be listed A-Z regardless of album: %+v", all)
	}
	if r := s.Songs("aguila", 0, 10); r.Total != 1 || r.Items[0].ID != "s2" || r.Items[0].Cover == "" {
		t.Fatalf("search: %+v", r)
	}
	pls := s.Playlists()
	if len(pls.Items) != 1 || pls.Items[0].Name != "Mix" || pls.Items[0].Count != 2 || pls.Items[0].Cover == "" {
		t.Fatalf("playlists: %+v", pls)
	}
	tr := s.PlaylistTracks("p1")
	if tr.Error != "" || len(tr.Items) != 2 || tr.Items[0].ID != "s3" || tr.Items[1].ID != "s1" {
		t.Fatalf("playlist order: %+v", tr)
	}
	if r := s.PlaylistTracks("nope"); r.Error == "" {
		t.Fatal("unknown playlist must report an error")
	}
}

func TestSyncPersistsAcrossServiceRestarts(t *testing.T) {
	s := syncService(t)
	_ = s.SyncLibrary()
	again := &service{path: s.path, ipcPath: s.ipcPath, libDir: s.libDir} // a new island process
	if info := again.SyncInfo(); !info.Synced || info.Songs != 3 {
		t.Fatalf("copy not found on disk: %+v", info)
	}
}

func TestFailedSyncKeepsThePreviousCopy(t *testing.T) {
	s := syncService(t)
	_ = s.SyncLibrary()
	// Point the same server entry at a dead address: the next sync must fail but keep the old copy.
	d := s.GetServers()[0]
	if msg := s.SaveServer(serverInput{ID: d.ID, URL: "http://127.0.0.1:1", User: "u"}); msg != "" {
		t.Fatal(msg)
	}
	if info := s.SyncLibrary(); info.Error == "" {
		t.Fatal("sync against a dead server must fail")
	}
	if r := s.Songs("", 0, 10); r.Total != 3 {
		t.Fatalf("previous copy lost: %+v", r)
	}
}

func TestPlayAllReplacesTheSingleQueueAndShuffles(t *testing.T) {
	s := syncService(t)
	_ = s.SyncLibrary()
	c := &ctl{}
	stop, err := ipc.Serve(s.ipcPath, c)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	st := s.PlayAll(true)
	if st.Error != "" || st.QueueLen != 3 || !st.Shuffle {
		t.Fatalf("shuffle all: %+v", st)
	}
	st = s.PlayAll(false) // plays again: still ONE queue, now in order with shuffle off
	if st.Error != "" || st.QueueLen != 3 || st.Shuffle || st.Index != 0 || st.Current == nil || st.Current.Title != "Águila" {
		t.Fatalf("play all in order: %+v", st)
	}
}
