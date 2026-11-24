//go:build windows

package island

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// skWorld is a fake Navidrome plus a fake songkey whose job "j1" moves
// through the states the test sets.
type skWorld struct {
	mu       sync.Mutex
	state    string // state of j1
	filedIn  bool   // the new song is in the DETODO playlist
	statusOK bool
	s        *service
}

func newSkWorld(t *testing.T) *skWorld {
	t.Helper()
	w := &skWorld{state: "received", statusOK: true}
	nav := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		defer w.mu.Unlock()
		switch {
		case strings.Contains(r.URL.Path, "search3"):
			if r.URL.Query().Get("songOffset") != "0" {
				io.WriteString(rw, `{"subsonic-response":{"status":"ok","searchResult3":{}}}`)
				return
			}
			io.WriteString(rw, `{"subsonic-response":{"status":"ok","searchResult3":{"song":[{"id":"s1","title":"Old","artist":"A"}]}}}`)
		case strings.Contains(r.URL.Path, "getPlaylists"):
			io.WriteString(rw, `{"subsonic-response":{"status":"ok","playlists":{"playlist":[{"id":"pl-all","name":"DETODO"}]}}}`)
		case strings.Contains(r.URL.Path, "getPlaylist"):
			entries := `{"id":"s1","title":"Old","artist":"A"}`
			if w.filedIn {
				entries += `,{"id":"s9","title":"Tití Me Preguntó","artist":"Bad Bunny"}`
			}
			io.WriteString(rw, `{"subsonic-response":{"status":"ok","playlist":{"entry":[`+entries+`]}}}`)
		default:
			io.WriteString(rw, `{"subsonic-response":{"status":"ok"}}`)
		}
	}))
	t.Cleanup(nav.Close)
	sk := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
		w.mu.Lock()
		defer w.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer tok" {
			rw.WriteHeader(http.StatusUnauthorized)
			io.WriteString(rw, `{"error":{"code":"unauthorized","message":"wrong or missing token"}}`)
			return
		}
		switch {
		case r.URL.Path == "/add":
			_ = r.ParseForm()
			switch r.PostForm.Get("url") {
			case "https://youtu.be/have":
				rw.WriteHeader(http.StatusConflict)
				io.WriteString(rw, `{"result":"already_have","message":"ALREADY HAVE IT\nOld"}`)
			case "bad":
				rw.WriteHeader(http.StatusBadRequest)
				io.WriteString(rw, `{"error":{"code":"invalid_url","message":"could not read a YouTube video id from that link"}}`)
			default:
				rw.WriteHeader(http.StatusAccepted)
				io.WriteString(rw, `{"result":"queued","message":"QUEUED\nTití","job":{"id":"j1","state":"received","title":"Tití Me Preguntó","artist":"Bad Bunny"}}`)
			}
		case r.URL.Path == "/status" && r.URL.Query().Get("job") == "j1":
			reason := ""
			if w.state == "failed" {
				reason = `,"reason":"MeTube said no"`
			}
			io.WriteString(rw, `{"job":{"id":"j1","state":"`+w.state+`","title":"Tití Me Preguntó","artist":"Bad Bunny"`+reason+`}}`)
		case r.URL.Path == "/status" && r.URL.Query().Get("job") != "":
			rw.WriteHeader(http.StatusNotFound)
			io.WriteString(rw, `{"error":{"code":"job_not_found","message":"no job with that id"}}`)
		case r.URL.Path == "/status":
			io.WriteString(rw, `{"playlist":{"id":"pl-all","name":"DETODO"},"indexed":1,"active":[],"recent":[]}`)
		}
	}))
	t.Cleanup(sk.Close)

	dir := t.TempDir()
	w.s = &service{path: filepath.Join(dir, "servers.json"), ipcPath: filepath.Join(dir, "ipc.json"), libDir: dir}
	if msg := w.s.SaveServer(serverInput{URL: nav.URL, User: "u", Password: "p"}); msg != "" {
		t.Fatal(msg)
	}
	if r := w.s.SaveSongkey(songkeyInput{URL: sk.URL + "/", Token: "tok"}); r.Error != "" {
		t.Fatal(r.Error)
	}
	return w
}

func (w *skWorld) set(state string, filed bool) {
	w.mu.Lock()
	w.state, w.filedIn = state, filed
	w.mu.Unlock()
}

func TestSongkeySettingsKeepTheTokenOutOfThePage(t *testing.T) {
	w := newSkWorld(t)
	v := w.s.GetSongkey()
	if !v.Configured || !v.HasToken || strings.HasSuffix(v.URL, "/") {
		t.Fatalf("got %+v", v)
	}
	// An empty token keeps the saved one; a wrong one is refused unless forced.
	if r := w.s.SaveSongkey(songkeyInput{URL: v.URL}); r.Error != "" || !r.HasToken {
		t.Fatalf("keep token: %+v", r)
	}
	if r := w.s.SaveSongkey(songkeyInput{URL: v.URL, Token: "wrong"}); !strings.Contains(r.Error, "wrong or missing token") {
		t.Fatalf("bad token must be reported: %+v", r)
	}
	if r := w.s.SaveSongkey(songkeyInput{URL: "ftp://x"}); r.Error == "" {
		t.Fatal("only http(s) URLs")
	}
	if r := w.s.SaveSongkey(songkeyInput{}); r.Error != "" || r.Configured {
		t.Fatalf("empty URL removes it: %+v", r)
	}
	if r := w.s.SongkeyAdd("https://youtu.be/x"); r.Error != "songkey is not set up" {
		t.Fatalf("got %+v", r)
	}
}

func TestSongkeyAlreadyHaveAndErrors(t *testing.T) {
	w := newSkWorld(t)
	if r := w.s.SongkeyAdd("https://youtu.be/have"); r.Error != "" || r.Result != "already_have" || r.Message == "" {
		t.Fatalf("already_have is a normal outcome: %+v", r)
	}
	if r := w.s.SongkeyAdd("bad"); r.Error != "could not read a YouTube video id from that link" {
		t.Fatalf("got %+v", r)
	}
	if r := w.s.SongkeyAdd("   "); r.Error == "" {
		t.Fatal("an empty link must not reach the server")
	}
	if j := w.s.SongkeyJobs(); len(j.Jobs) != 0 {
		t.Fatalf("nothing to follow: %+v", j)
	}
}

func TestSongkeyJobIsFollowedUntilFiledThenThePlaylistIsRefreshed(t *testing.T) {
	w := newSkWorld(t)
	if info := w.s.SyncLibrary(); info.Error != "" {
		t.Fatal(info.Error)
	}
	r := w.s.SongkeyAdd("https://youtu.be/Cr8K88UcO5s")
	if r.Error != "" || r.Result != "queued" || r.Job == nil || r.Job.ID != "j1" {
		t.Fatalf("add: %+v", r)
	}
	w.set("downloaded", false)
	j := w.s.SongkeyJobs()
	if j.Error != "" || len(j.Jobs) != 1 || j.Jobs[0].State != "downloaded" || len(j.Filed) != 0 {
		t.Fatalf("in progress: %+v", j)
	}
	// The list survives the window closing (a new service = a new window).
	w.s = &service{path: w.s.path, ipcPath: w.s.ipcPath, libDir: w.s.libDir}
	w.set("filed", true)
	j = w.s.SongkeyJobs()
	if j.Error != "" || len(j.Jobs) != 0 || len(j.Filed) != 1 || j.Filed[0] != "Bad Bunny - Tití Me Preguntó" || !j.Refreshed {
		t.Fatalf("filed: %+v", j)
	}
	pl := w.s.PlaylistTracks("pl-all")
	if len(pl.Items) != 2 || pl.Items[1].Title != "Tití Me Preguntó" {
		t.Fatalf("DETODO must hold the new song: %+v", pl)
	}
	if s := w.s.Songs("titi", 0, 10); s.Total != 1 {
		t.Fatalf("the new song must be searchable: %+v", s)
	}
}

func TestSongkeyFailedJobStaysUntilDismissed(t *testing.T) {
	w := newSkWorld(t)
	w.s.SongkeyAdd("https://youtu.be/Cr8K88UcO5s")
	w.set("failed", false)
	j := w.s.SongkeyJobs()
	if len(j.Jobs) != 1 || j.Jobs[0].State != "failed" || j.Jobs[0].Reason != "MeTube said no" {
		t.Fatalf("failed: %+v", j)
	}
	if j = w.s.SongkeyJobs(); len(j.Jobs) != 1 {
		t.Fatal("a failed job stays visible")
	}
	if j = w.s.SongkeyDismiss("j1"); len(j.Jobs) != 0 {
		t.Fatalf("dismissed: %+v", j)
	}
	if _, err := os.Stat(filepath.Join(w.s.libDir, "songkey-jobs.json")); !os.IsNotExist(err) {
		t.Fatal(err)
	}
}

func TestSongkeyForgottenJobBecomesFailed(t *testing.T) {
	w := newSkWorld(t)
	w.s.SongkeyAdd("https://youtu.be/Cr8K88UcO5s")
	w.s.skJobsMu.Lock()
	jobs := w.s.loadSkJobs()
	jobs[0].ID = "gone"
	w.s.saveSkJobs(jobs)
	w.s.skJobsMu.Unlock()
	j := w.s.SongkeyJobs()
	if len(j.Jobs) != 1 || j.Jobs[0].State != "failed" || j.Jobs[0].Reason == "" {
		t.Fatalf("got %+v", j)
	}
}

func TestSongkeyTokenIsNotCarriedToAnotherHost(t *testing.T) {
	w := newSkWorld(t)
	old := w.s.GetSongkey().URL
	if r := w.s.SaveSongkey(songkeyInput{URL: old + "/sub", Force: true}); r.Error != "" || !r.HasToken {
		t.Fatalf("same host keeps the token: %+v", r)
	}
	r := w.s.SaveSongkey(songkeyInput{URL: "https://elsewhere.example", Force: true})
	if r.Error != "" || r.HasToken {
		t.Fatalf("a new host must not get the old token: %+v", r)
	}
}
