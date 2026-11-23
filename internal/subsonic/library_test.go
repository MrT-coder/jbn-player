package subsonic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

func TestAlbumListSendsTypeSizeOffset(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","albumList2":{"album":[{"id":"a1","name":"N","artist":"A","coverArt":"al-a1","songCount":3,"year":2020}]}}}`))
	}))
	defer srv.Close()
	albums, err := New(srv.URL, "bob", "pw").AlbumList(context.Background(), "newest", 30, 60)
	if err != nil || len(albums) != 1 || albums[0].CoverArt != "al-a1" || albums[0].Year != 2020 {
		t.Fatalf("got %+v, %v", albums, err)
	}
	if got.Get("type") != "newest" || got.Get("size") != "30" || got.Get("offset") != "60" {
		t.Fatalf("bad params: %v", got)
	}
}

func TestSearchDecodesAlbumsAndSongs(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("query") != "rock" {
			t.Errorf("query not sent: %v", r.URL.Query())
		}
		_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok","searchResult3":{"album":[{"id":"a1","name":"N"}],"song":[{"id":"s1","title":"T","coverArt":"c1"}]}}}`))
	}))
	defer srv.Close()
	res, err := New(srv.URL, "bob", "pw").Search(context.Background(), "rock")
	if err != nil || len(res.Albums) != 1 || len(res.Songs) != 1 || res.Songs[0].CoverArt != "c1" {
		t.Fatalf("got %+v, %v", res, err)
	}
}

func TestCoverURLIsStableAndEmptyForNoCover(t *testing.T) {
	c := New("https://x.example", "bob", "pw")
	a, b := c.CoverURL("al-1", 96), c.CoverURL("al-1", 96)
	if a == "" || a != b {
		t.Fatalf("cover URL must be stable for caching: %q vs %q", a, b)
	}
	u, _ := url.Parse(a)
	if u.Path != "/rest/getCoverArt.view" || u.Query().Get("size") != "96" || u.Query().Get("id") != "al-1" {
		t.Fatalf("unexpected: %s", a)
	}
	if c.CoverURL("", 96) != "" {
		t.Fatal("no cover id must give an empty URL")
	}
}
