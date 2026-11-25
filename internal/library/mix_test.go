package library

import (
	"context"
	"math/rand"
	"testing"
	"time"

	"jbnplayer/internal/subsonic"
)

var now = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func TestSyncKeepsGenreYearAndListeningStats(t *testing.T) {
	src := &fakeSource{songs: []subsonic.Song{{
		ID: "s1", Title: "T", Genre: "Rock", Genres: []subsonic.Named{{Name: "Rock"}, {Name: "Pop"}},
		Year: 1987, PlayCount: 12, Starred: "2025-01-01T00:00:00Z", UserRating: 4, Played: "2026-11-24T10:00:00Z",
	}, {ID: "s2", Title: "U", Genre: "Jazz"}}}
	c, err := Sync(context.Background(), src)
	if err != nil {
		t.Fatal(err)
	}
	s1, s2 := c.Songs[0], c.Songs[1]
	if len(s1.Genres) != 2 || s1.Genres[1] != "Pop" || s1.Year != 1987 || s1.PlayCount != 12 || !s1.Starred || s1.Rating != 4 {
		t.Fatalf("s1: %+v", s1)
	}
	if s1.Played != time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC).Unix() {
		t.Fatalf("played: %d", s1.Played)
	}
	if len(s2.Genres) != 1 || s2.Genres[0] != "Jazz" || s2.Starred || s2.Played != 0 {
		t.Fatalf("s2 (only the old genre field): %+v", s2)
	}
}

func TestWeightFavorsLovedAndPlayedAndRestsRecentSongs(t *testing.T) {
	plain := Weight(Song{}, now)
	if plain != 1 {
		t.Fatalf("plain song weight %v, want 1", plain)
	}
	for name, s := range map[string]Song{
		"starred":      {Starred: true},
		"rated 5":      {Rating: 5},
		"rated 4":      {Rating: 4},
		"often played": {PlayCount: 50},
	} {
		if Weight(s, now) <= plain {
			t.Errorf("%s must weigh more than a plain song: %v", name, Weight(s, now))
		}
	}
	for name, s := range map[string]Song{
		"rated 1":           {Rating: 1},
		"played today":      {Played: now.Add(-3 * time.Hour).Unix()},
		"played 5 days ago": {Played: now.Add(-5 * 24 * time.Hour).Unix()},
	} {
		if Weight(s, now) >= plain {
			t.Errorf("%s must weigh less than a plain song: %v", name, Weight(s, now))
		}
	}
	if Weight(Song{Played: now.Add(-3 * time.Hour).Unix()}, now) >= Weight(Song{Played: now.Add(-5 * 24 * time.Hour).Unix()}, now) {
		t.Error("a song played today rests longer than one played 5 days ago")
	}
	if Weight(Song{Played: now.Add(-30 * 24 * time.Hour).Unix()}, now) != plain {
		t.Error("a month ago no longer counts")
	}
}

func TestWeightedOrderKeepsEverySongAndPutsFavoritesFirstMoreOften(t *testing.T) {
	ss := []Song{{ID: "fav", Starred: true, Rating: 5}, {ID: "a"}, {ID: "b"}, {ID: "c"}, {ID: "d"}}
	r := rand.New(rand.NewSource(1))
	firsts := map[string]int{}
	for i := 0; i < 2000; i++ {
		out := WeightedOrder(ss, now, r)
		if len(out) != len(ss) {
			t.Fatalf("lost songs: %+v", out)
		}
		firsts[out[0].ID]++
	}
	// Weight 9 against 1 for each of the others: the favorite opens ~69% of the time.
	if firsts["fav"] < 1200 || firsts["a"] > 400 {
		t.Fatalf("first-song counts: %v", firsts)
	}
	if ss[0].ID != "fav" || ss[4].ID != "d" {
		t.Fatal("the input slice must not be reordered")
	}
}

func mixCache() *Cache {
	c := &Cache{Version: cacheVersion, Songs: []Song{
		{ID: "1", Title: "a", Genres: []string{"Rock"}, Year: 1985},
		{ID: "2", Title: "b", Genres: []string{"rock", "Pop"}, Year: 1994},
		{ID: "3", Title: "c", Genres: []string{"Cumbia"}, Year: 1989},
		{ID: "4", Title: "d", Genres: []string{"Cumbia"}, Year: 2003},
		{ID: "5", Title: "e"},
	}}
	c.index()
	return c
}

func idsOf(ss []Song) string {
	out := ""
	for _, s := range ss {
		out += s.ID
	}
	return out
}

func TestMixFiltersByGenreAndDecade(t *testing.T) {
	c := mixCache()
	cases := []struct {
		f    MixFilter
		want string
	}{
		{MixFilter{}, "12345"},
		{MixFilter{Genres: []string{"ROCK"}}, "12"},                              // case-insensitive
		{MixFilter{Genres: []string{"Rock", "Cumbia"}}, "1234"},                  // any of the genres
		{MixFilter{Decades: []int{1980}}, "13"},                                  // 1980-1989
		{MixFilter{Genres: []string{"Cumbia"}, Decades: []int{1980, 1990}}, "3"}, // both groups must match
	}
	for _, tc := range cases {
		if got := idsOf(c.Mix(tc.f, nil)); got != tc.want {
			t.Errorf("%+v: got %s, want %s", tc.f, got, tc.want)
		}
	}
	if got := idsOf(c.Mix(MixFilter{Genres: []string{"Cumbia"}}, func(s Song) bool { return s.ID == "4" })); got != "4" {
		t.Errorf("keep filter: %s", got)
	}
}

func TestMixOptionsCountGenresAndDecades(t *testing.T) {
	genres, decades := mixCache().MixOptions(nil)
	// Rock 2 (case folded, first spelling kept), Cumbia 2, Pop 1; by count then name.
	if len(genres) != 3 || genres[0] != (Count{"Cumbia", 2}) || genres[1] != (Count{"Rock", 2}) || genres[2] != (Count{"Pop", 1}) {
		t.Fatalf("genres: %+v", genres)
	}
	if len(decades) != 3 || decades[0] != (Count{"1980", 2}) || decades[2] != (Count{"2000", 1}) {
		t.Fatalf("decades: %+v", decades)
	}
}
