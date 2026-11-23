package subsonic

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func newServer(t *testing.T, body string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		sum := md5.Sum([]byte("pw" + q.Get("s")))
		if q.Get("u") != "bob" || q.Get("t") != hex.EncodeToString(sum[:]) || q.Get("v") != apiVersion || q.Get("f") != "json" {
			t.Errorf("bad auth params: %v", q)
		}
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "bob", "pw")
}

func TestPingSendsTokenAuth(t *testing.T) {
	c := newServer(t, `{"subsonic-response":{"status":"ok"}}`)
	if err := c.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestErrorEnvelopeSurfaces(t *testing.T) {
	c := newServer(t, `{"subsonic-response":{"status":"failed","error":{"code":40,"message":"Wrong username or password"}}}`)
	err := c.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "40") || !strings.Contains(err.Error(), "Wrong username") {
		t.Fatalf("got %v", err)
	}
}

func TestAlbumSongsDecodes(t *testing.T) {
	c := newServer(t, `{"subsonic-response":{"status":"ok","album":{"song":[{"id":"s1","title":"T","artist":"A","duration":61}]}}}`)
	songs, err := c.AlbumSongs(context.Background(), "al1")
	if err != nil || len(songs) != 1 || songs[0].ID != "s1" || songs[0].Duration != 61 {
		t.Fatalf("got %+v, %v", songs, err)
	}
}

func TestStreamURLIsSignedAndTargetsStream(t *testing.T) {
	c := New("https://x.example", "bob", "pw")
	u, err := url.Parse(c.StreamURL("s9"))
	if err != nil {
		t.Fatal(err)
	}
	if u.Path != "/rest/stream.view" || u.Query().Get("id") != "s9" || u.Query().Get("t") == "" || u.Query().Get("s") == "" {
		t.Fatalf("unexpected url: %s", u)
	}
}
