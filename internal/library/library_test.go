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

func TestRefreshPlaylistAddsNewSongsWithoutTouchingTheOldCopy(t *testing.T) {
	src := &fakeSource{
		songs:     []subsonic.Song{song("a", "Alpha", "X")},
		playlists: []subsonic.Playlist{{ID: "p1", Name: "DETODO"}, {ID: "p2", Name: "Other"}},
		entries:   map[string][]subsonic.Song{"p1": {song("a", "Alpha", "X")}, "p2": {song("a", "Alpha", "X")}},
	}
	c, err := Sync(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	src.entries["p1"] = []subsonic.Song{song("a", "Alpha", "X"), song("n", "New one", "Y")}
	n, err := c.RefreshPlaylist(context.Background(), src, "p1", "DETODO")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := n.PlaylistSongs("p1")
	if !ok || len(got) != 2 || got[1].ID != "n" {
		t.Fatalf("refreshed playlist: %+v", got)
	}
	if found, total := n.Find("new one", 0, 10); total != 1 || found[0].Artist != "Y" {
		t.Fatal("the new song must be searchable")
	}
	if other, _ := n.PlaylistSongs("p2"); len(other) != 1 || len(n.Playlists) != 2 {
		t.Fatal("other playlists stay as they were")
	}
	if old, _ := c.PlaylistSongs("p1"); len(old) != 1 || len(c.Songs) != 1 {
		t.Fatal("the original cache must not change (readers may be using it)")
	}
	// A playlist the copy never saw is added with the given name.
	src.entries["p3"] = []subsonic.Song{song("a", "Alpha", "X")}
	n2, err := n.RefreshPlaylist(context.Background(), src, "p3", "Fresh")
	if err != nil || len(n2.Playlists) != 3 || n2.Playlists[2].Name != "Fresh" {
		t.Fatalf("new playlist: %+v %v", n2.Playlists, err)
	}
}

func offlineCache() *Cache {
	c := &Cache{Version: cacheVersion, Songs: []Song{
		{ID: "a2", Title: "Second", Artist: "Ann", Album: "Zebra", AlbumID: "z", CoverArt: "cz", Track: 2},
		{ID: "a1", Title: "First", Artist: "Ann", Album: "Zebra", AlbumID: "z", CoverArt: "cz", Track: 1},
		{ID: "b1", Title: "Only", Artist: "Bob", Album: "Apple", AlbumID: "ap", CoverArt: "ca", Track: 1},
		{ID: "c1", Title: "Not stored", Artist: "Cid", Album: "Cherry", AlbumID: "ch", Track: 1},
	}}
	c.index()
	return c
}

func storedOnly(ids ...string) func(Song) bool {
	set := map[string]bool{}
	for _, id := range ids {
		set[id] = true
	}
	return func(s Song) bool { return set[s.ID] }
}

func TestAlbumsGroupsKeptSongsByAlbumSortedByName(t *testing.T) {
	c := offlineCache()
	got := c.Albums(storedOnly("a1", "a2", "b1"))
	if len(got) != 2 {
		t.Fatalf("got %+v, want Apple and Zebra only", got)
	}
	if got[0].ID != "ap" || got[0].Name != "Apple" || got[0].Artist != "Bob" || got[0].SongCount != 1 || got[0].CoverArt != "ca" {
		t.Fatalf("first: %+v", got[0])
	}
	if got[1].ID != "z" || got[1].SongCount != 2 {
		t.Fatalf("second: %+v", got[1])
	}
	if all := c.Albums(nil); len(all) != 3 {
		t.Fatalf("nil keeps everything: %+v", all)
	}
}

func TestAlbumSongsAreInTrackOrderAndFiltered(t *testing.T) {
	c := offlineCache()
	got := c.AlbumSongs("z", storedOnly("a1", "a2"))
	if len(got) != 2 || got[0].ID != "a1" || got[1].ID != "a2" {
		t.Fatalf("got %+v", got)
	}
	if got := c.AlbumSongs("z", storedOnly("a2")); len(got) != 1 || got[0].ID != "a2" {
		t.Fatalf("filtered: %+v", got)
	}
	if got := c.AlbumSongs("nope", nil); len(got) != 0 {
		t.Fatalf("unknown album: %+v", got)
	}
}

func TestFindWhereFiltersBeforePaging(t *testing.T) {
	c := offlineCache()
	got, total := c.FindWhere("", 0, 1, storedOnly("b1", "c1"))
	if total != 2 || len(got) != 1 {
		t.Fatalf("got %+v total %d", got, total)
	}
	if got, total := c.FindWhere("ann", 0, 0, storedOnly("a1")); total != 1 || got[0].ID != "a1" {
		t.Fatalf("query+filter: %+v %d", got, total)
	}
	// Find keeps working as before.
	if _, total := c.Find("", 0, 0); total != 4 {
		t.Fatalf("Find total %d", total)
	}
}
