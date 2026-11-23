package library

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"testing"

	"jbnplayer/internal/subsonic"
)

type fakeSource struct {
	songs     []subsonic.Song
	playlists []subsonic.Playlist
	entries   map[string][]subsonic.Song
	failList  bool
	pages     int
}

func (f *fakeSource) AllSongs(_ context.Context, offset, count int) ([]subsonic.Song, error) {
	f.pages++
	if offset >= len(f.songs) {
		return nil, nil
	}
	end := offset + count
	if end > len(f.songs) {
		end = len(f.songs)
	}
	return f.songs[offset:end], nil
}
func (f *fakeSource) Playlists(context.Context) ([]subsonic.Playlist, error) {
	if f.failList {
		return nil, errors.New("boom")
	}
	return f.playlists, nil
}
func (f *fakeSource) PlaylistSongs(_ context.Context, id string) ([]subsonic.Song, error) {
	return f.entries[id], nil
}

func song(id, title, artist string) subsonic.Song {
	return subsonic.Song{ID: id, Title: title, Artist: artist, Album: "Alb", CoverArt: "c-" + id, Duration: 100}
}

func TestSyncPagesAllSongsAndSortsByTitle(t *testing.T) {
	var songs []subsonic.Song
	for i := 0; i < pageSize+10; i++ { // forces two pages
		songs = append(songs, song(fmt.Sprintf("id%d", i), "T", "A"))
	}
	songs = append(songs, song("zz", "Águila", "X"), song("yy", "Beta", "X"))
	src := &fakeSource{songs: songs}
	c, err := Sync(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if src.pages < 2 {
		t.Fatalf("expected paging, got %d page calls", src.pages)
	}
	if len(c.Songs) != len(songs) {
		t.Fatalf("songs = %d", len(c.Songs))
	}
	// "Águila" sorts as "aguila", before "Beta" and "T".
	if c.Songs[0].Title != "Águila" || c.Songs[1].Title != "Beta" {
		t.Fatalf("order: %q, %q", c.Songs[0].Title, c.Songs[1].Title)
	}
}

func TestSyncPlaylistsKeepOrderAndAddMissingSongs(t *testing.T) {
	src := &fakeSource{
		songs:     []subsonic.Song{song("1", "One", "A"), song("2", "Two", "A")},
		playlists: []subsonic.Playlist{{ID: "p1", Name: "Mine"}},
		entries:   map[string][]subsonic.Song{"p1": {song("2", "Two", "A"), song("3", "Three", "B"), song("1", "One", "A")}},
	}
	c, err := Sync(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Songs) != 3 {
		t.Fatalf("song only present in a playlist must be kept, got %d songs", len(c.Songs))
	}
	got, ok := c.PlaylistSongs("p1")
	if !ok || len(got) != 3 || got[0].ID != "2" || got[1].ID != "3" || got[2].ID != "1" {
		t.Fatalf("playlist order lost: %+v", got)
	}
	if _, ok := c.PlaylistSongs("nope"); ok {
		t.Fatal("unknown playlist must report false")
	}
}

func TestSyncFailureReturnsNoCache(t *testing.T) {
	src := &fakeSource{songs: []subsonic.Song{song("1", "One", "A")}, failList: true}
	if c, err := Sync(context.Background(), src); err == nil || c != nil {
		t.Fatalf("got %v, %v", c, err)
	}
}

func TestFindIsAccentAndCaseInsensitiveAndMatchesAllWords(t *testing.T) {
	src := &fakeSource{songs: []subsonic.Song{
		song("1", "Águila Blanca", "Los Tigres"), song("2", "Luna", "Aguila Records"), song("3", "Sol", "Otro"),
	}}
	c, _ := Sync(context.Background(), src)
	got, total := c.Find("AGUILA", 0, 10)
	if total != 2 || len(got) != 2 {
		t.Fatalf("accent/case folding: total=%d", total)
	}
	if _, total := c.Find("aguila tigres", 0, 10); total != 1 {
		t.Fatalf("all words must match, total=%d", total)
	}
	if _, total := c.Find("", 0, 10); total != 3 {
		t.Fatalf("empty query matches everything, total=%d", total)
	}
	page, total := c.Find("", 1, 1)
	if total != 3 || len(page) != 1 {
		t.Fatalf("paging: total=%d len=%d", total, len(page))
	}
	if page, _ := c.Find("", 99, 5); len(page) != 0 {
		t.Fatal("offset past the end must give an empty page")
	}
}

func TestSaveLoadRoundTripAndMissingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "lib.json")
	if c, err := Load(path); c != nil || err != nil {
		t.Fatalf("missing file: %v, %v", c, err)
	}
	src := &fakeSource{songs: []subsonic.Song{song("1", "One", "A")},
		playlists: []subsonic.Playlist{{ID: "p", Name: "P"}}, entries: map[string][]subsonic.Song{"p": {song("1", "One", "A")}}}
	c, _ := Sync(context.Background(), src)
	if err := c.Save(path); err != nil {
		t.Fatal(err)
	}
	back, err := Load(path)
	if err != nil || back == nil || len(back.Songs) != 1 || len(back.Playlists) != 1 {
		t.Fatalf("round trip: %+v, %v", back, err)
	}
	if got, ok := back.PlaylistSongs("p"); !ok || len(got) != 1 || got[0].Title != "One" {
		t.Fatal("indexes must be rebuilt on load")
	}
	if !back.SyncedAt.Equal(c.SyncedAt) {
		t.Fatal("SyncedAt lost")
	}
}
