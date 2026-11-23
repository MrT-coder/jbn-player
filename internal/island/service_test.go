//go:build windows

package island

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func newService(t *testing.T) *service {
	t.Helper()
	return &service{path: filepath.Join(t.TempDir(), "servers.json")}
}

func okServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"subsonic-response":{"status":"ok"}}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestSaveFirstServerBecomesActiveAndHidesPassword(t *testing.T) {
	s := newService(t)
	if msg := s.SaveServer(serverInput{URL: "https://music.example.com/", User: "bob", Password: "pw"}); msg != "" {
		t.Fatal(msg)
	}
	list := s.GetServers()
	if len(list) != 1 || !list[0].Active || !list[0].HasPassword {
		t.Fatalf("unexpected list: %+v", list)
	}
	if list[0].URL != "https://music.example.com" || list[0].Name != "music.example.com" || list[0].Type != "subsonic" {
		t.Fatalf("not normalized: %+v", list[0])
	}
}

func TestSaveValidation(t *testing.T) {
	s := newService(t)
	cases := []struct {
		in   serverInput
		want string
	}{
		{serverInput{URL: "music.example.com", User: "u", Password: "p"}, "http"},
		{serverInput{URL: "https://x.example", User: " ", Password: "p"}, "username"},
		{serverInput{URL: "https://x.example", User: "u"}, "password"},
		{serverInput{URL: "https://x.example", User: "u", Password: "p", Type: "plex"}, "unsupported"},
	}
	for _, c := range cases {
		if msg := s.SaveServer(c.in); !strings.Contains(msg, c.want) {
			t.Errorf("%+v: got %q, want it to mention %q", c.in, msg, c.want)
		}
	}
	if len(s.GetServers()) != 0 {
		t.Fatal("invalid servers must not be saved")
	}
}

func TestEditKeepsPasswordWhenBlank(t *testing.T) {
	srv := okServer(t)
	s := newService(t)
	_ = s.SaveServer(serverInput{URL: srv.URL, User: "bob", Password: "pw"})
	id := s.GetServers()[0].ID
	if msg := s.SaveServer(serverInput{ID: id, Name: "Renamed", URL: srv.URL, User: "bob"}); msg != "" {
		t.Fatal(msg)
	}
	got := s.GetServers()[0]
	if got.Name != "Renamed" || !got.HasPassword {
		t.Fatalf("edit lost data: %+v", got)
	}
	// Testing with a blank password must reuse the stored one.
	if msg := s.TestServer(serverInput{ID: id, URL: srv.URL, User: "bob"}); msg != "" {
		t.Fatalf("test with stored password failed: %s", msg)
	}
}

func TestTestServerReportsFailure(t *testing.T) {
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"subsonic-response":{"status":"failed","error":{"code":40,"message":"Wrong username or password"}}}`))
	}))
	defer bad.Close()
	s := newService(t)
	if msg := s.TestServer(serverInput{URL: bad.URL, User: "u", Password: "p"}); !strings.Contains(msg, "Wrong username") {
		t.Fatalf("got %q", msg)
	}
}

func TestDeleteActiveMovesActiveToNext(t *testing.T) {
	s := newService(t)
	_ = s.SaveServer(serverInput{Name: "A", URL: "https://a.example", User: "u", Password: "p"})
	_ = s.SaveServer(serverInput{Name: "B", URL: "https://b.example", User: "u", Password: "p"})
	list := s.GetServers()
	if msg := s.DeleteServer(list[0].ID); msg != "" {
		t.Fatal(msg)
	}
	left := s.GetServers()
	if len(left) != 1 || left[0].Name != "B" || !left[0].Active {
		t.Fatalf("got %+v", left)
	}
}

func TestSetActive(t *testing.T) {
	s := newService(t)
	_ = s.SaveServer(serverInput{Name: "A", URL: "https://a.example", User: "u", Password: "p"})
	_ = s.SaveServer(serverInput{Name: "B", URL: "https://b.example", User: "u", Password: "p"})
	b := s.GetServers()[1]
	if msg := s.SetActive(b.ID); msg != "" {
		t.Fatal(msg)
	}
	if !s.GetServers()[1].Active || s.GetServers()[0].Active {
		t.Fatal("B must be the only active server")
	}
	if msg := s.SetActive("nope"); msg == "" {
		t.Fatal("unknown id must fail")
	}
}
