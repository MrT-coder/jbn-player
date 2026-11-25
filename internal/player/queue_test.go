package player

import (
	"sort"
	"testing"
)

func ids(ts []Track) string {
	s := ""
	for _, t := range ts {
		s += t.ID
	}
	return s
}

func mk(s string) []Track {
	var ts []Track
	for _, r := range s {
		ts = append(ts, Track{ID: string(r)})
	}
	return ts
}

func TestQueueNavigation(t *testing.T) {
	var q queue
	if _, ok := q.current(); ok {
		t.Fatal("empty queue must have no current track")
	}
	q.set(mk("ab"), 0)
	if tr, _ := q.current(); tr.ID != "a" {
		t.Fatalf("start at a, got %s", tr.ID)
	}
	if tr, ok := q.next(); !ok || tr.ID != "b" {
		t.Fatalf("next -> b, got %v %v", tr, ok)
	}
	if _, ok := q.next(); ok {
		t.Fatal("next at end must report false")
	}
	if tr, _ := q.current(); tr.ID != "b" {
		t.Fatal("index must stay on last track after failed next")
	}
	if tr, _ := q.prev(); tr.ID != "a" {
		t.Fatalf("prev -> a, got %s", tr.ID)
	}
	if tr, ok := q.prev(); !ok || tr.ID != "a" {
		t.Fatal("prev on first track restarts it")
	}
}

func TestSetStartsAtChosenTrack(t *testing.T) {
	var q queue
	q.set(mk("abc"), 2)
	if tr, _ := q.current(); tr.ID != "c" {
		t.Fatalf("got %s", tr.ID)
	}
	q.set(mk("abc"), 99) // out of range falls back to the first track
	if tr, _ := q.current(); tr.ID != "a" {
		t.Fatalf("got %s", tr.ID)
	}
}

func TestPlayNextInsertsAfterCurrent(t *testing.T) {
	var q queue
	q.set(mk("abc"), 0)
	at := q.insertNext(mk("xy"))
	if at != 1 || ids(q.tracks) != "axybc" {
		t.Fatalf("at=%d queue=%s", at, ids(q.tracks))
	}
	var e queue
	if at := e.insertNext(mk("xy")); at != 0 || ids(e.tracks) != "xy" {
		t.Fatalf("empty queue: at=%d queue=%s", at, ids(e.tracks))
	}
}

func TestAppendGoesToTheEnd(t *testing.T) {
	var q queue
	q.set(mk("ab"), 0)
	if at := q.appendTracks(mk("xy")); at != 2 || ids(q.tracks) != "abxy" {
		t.Fatalf("at=%d queue=%s", at, ids(q.tracks))
	}
}

func TestRemoveKeepsCurrentAndAdjustsIndex(t *testing.T) {
	var q queue
	q.set(mk("abcd"), 2) // current = c
	if q.remove(2) {
		t.Fatal("must not remove the current track")
	}
	if !q.remove(0) || ids(q.tracks) != "bcd" {
		t.Fatalf("queue=%s", ids(q.tracks))
	}
	if tr, _ := q.current(); tr.ID != "c" {
		t.Fatalf("current must still be c, got %s", tr.ID)
	}
	if !q.remove(2) || ids(q.tracks) != "bc" {
		t.Fatalf("queue=%s", ids(q.tracks))
	}
	if q.remove(9) {
		t.Fatal("out of range must fail")
	}
}

func TestClearUpcomingKeepsHistoryAndCurrent(t *testing.T) {
	var q queue
	q.set(mk("abcd"), 1)
	q.clearUpcoming()
	if ids(q.tracks) != "ab" {
		t.Fatalf("queue=%s", ids(q.tracks))
	}
}

func TestShuffleOnlyReordersUpcoming(t *testing.T) {
	var q queue
	q.set(mk("abcdefghij"), 3) // current = d
	q.setShuffle(true)
	if ids(q.tracks[:4]) != "abcd" {
		t.Fatalf("history and current must not move: %s", ids(q.tracks))
	}
	rest := []string{}
	for _, t := range q.tracks[4:] {
		rest = append(rest, t.ID)
	}
	sort.Strings(rest)
	if got := ""; true {
		for _, r := range rest {
			got += r
		}
		if got != "efghij" {
			t.Fatalf("shuffle lost or duplicated tracks: %s", got)
		}
	}
}

func TestSetWithShuffleStartsWithChosenTrack(t *testing.T) {
	var q queue
	q.shuffle = true
	q.set(mk("abcdef"), 4) // chosen = e
	if tr, _ := q.current(); tr.ID != "e" || q.idx != 0 {
		t.Fatalf("chosen track must play first: idx=%d current=%s", q.idx, tr.ID)
	}
	if len(q.tracks) != 6 {
		t.Fatal("no tracks may be lost")
	}
}

func TestJump(t *testing.T) {
	var q queue
	q.set(mk("abc"), 0)
	if !q.jump(2) {
		t.Fatal("jump failed")
	}
	if tr, _ := q.current(); tr.ID != "c" {
		t.Fatal("jump did not move")
	}
	if q.jump(5) {
		t.Fatal("out of range must fail")
	}
}

func TestMoveReordersAndTheCurrentTrackKeepsPlaying(t *testing.T) {
	cases := []struct {
		name          string
		cur, from, to int
		wantQueue     string
		wantCurrent   string
	}{
		{"move a later track before the current", 1, 3, 0, "dabc", "b"},
		{"move an earlier track after the current", 2, 0, 3, "bcda", "c"},
		{"move the current track itself", 1, 1, 3, "acdb", "b"},
		{"move across the current from before to after", 1, 0, 2, "bcad", "b"},
		{"move within upcoming tracks", 0, 3, 1, "adbc", "a"},
	}
	for _, c := range cases {
		var q queue
		q.set(mk("abcd"), c.cur)
		v := q.version
		if !q.move(c.from, c.to) {
			t.Fatalf("%s: move failed", c.name)
		}
		if ids(q.tracks) != c.wantQueue {
			t.Errorf("%s: queue=%s want %s", c.name, ids(q.tracks), c.wantQueue)
		}
		if cur, _ := q.current(); cur.ID != c.wantCurrent {
			t.Errorf("%s: current=%s want %s", c.name, cur.ID, c.wantCurrent)
		}
		if q.version == v {
			t.Errorf("%s: version must change so the UI refetches the queue", c.name)
		}
	}
}

func TestMoveRejectsBadIndexesAndNoopKeepsVersion(t *testing.T) {
	var q queue
	q.set(mk("abc"), 0)
	for _, bad := range [][2]int{{-1, 0}, {0, 3}, {5, 1}, {1, -2}} {
		if q.move(bad[0], bad[1]) {
			t.Fatalf("move %v must fail", bad)
		}
	}
	v := q.version
	if !q.move(1, 1) || q.version != v {
		t.Fatal("moving a track onto itself is a no-op")
	}
	if ids(q.tracks) != "abc" {
		t.Fatalf("queue changed: %s", ids(q.tracks))
	}
}

func TestAnyStartIsRandomWhenShuffleIsOnAndFirstOtherwise(t *testing.T) {
	var q queue
	q.shuffle = true
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		q.set(mk("abcdefghij"), AnyStart)
		cur, _ := q.current()
		seen[cur.ID] = true
		if len(q.tracks) != 10 || q.idx != 0 {
			t.Fatalf("queue must keep all tracks with the chosen one first: idx=%d len=%d", q.idx, len(q.tracks))
		}
	}
	if len(seen) < 5 {
		t.Fatalf("a shuffled play must not always begin with the same track, saw only %v", seen)
	}

	q.shuffle = false
	for i := 0; i < 50; i++ {
		q.set(mk("abcdefghij"), AnyStart)
		if cur, _ := q.current(); cur.ID != "a" {
			t.Fatalf("without shuffle the first track plays, got %s", cur.ID)
		}
	}
}

func TestExplicitStartIsAlwaysRespectedEvenWithShuffle(t *testing.T) {
	var q queue
	q.shuffle = true
	for i := 0; i < 50; i++ {
		q.set(mk("abcdefghij"), 6)
		if cur, _ := q.current(); cur.ID != "g" {
			t.Fatalf("playing from a chosen row must start there, got %s", cur.ID)
		}
	}
}

func TestPlayNowKeepsTheQueueAndContinuesWithIt(t *testing.T) {
	var q queue
	q.set(mk("abcd"), 1) // playing b
	if !q.playNow(Track{ID: "x"}) {
		t.Fatal("a new track must be loaded")
	}
	if ids(q.tracks) != "abxcd" || q.idx != 2 {
		t.Fatalf("queue=%s idx=%d", ids(q.tracks), q.idx)
	}
	if tr, _ := q.next(); tr.ID != "c" {
		t.Fatalf("after x the queue goes on with c, got %s", tr.ID)
	}
}

func TestPlayNowMovesAQueuedTrackInsteadOfDuplicatingIt(t *testing.T) {
	var q queue
	q.set(mk("abcde"), 1) // playing b
	q.playNow(Track{ID: "d"})
	if ids(q.tracks) != "abdce" || q.idx != 2 {
		t.Fatalf("upcoming: queue=%s idx=%d", ids(q.tracks), q.idx)
	}
	q.playNow(Track{ID: "a"}) // already played: it comes back, history shrinks
	if ids(q.tracks) != "bdace" || q.idx != 2 {
		t.Fatalf("history: queue=%s idx=%d", ids(q.tracks), q.idx)
	}
	v := q.version
	if q.playNow(Track{ID: "a"}) || q.version != v {
		t.Fatal("the current track must keep playing untouched")
	}
}

func TestPlayNowIsNotReshuffled(t *testing.T) {
	var q queue
	q.set(mk("abc"), 0)
	q.setShuffle(true)
	up := ids(q.tracks[1:])
	q.playNow(Track{ID: "x"})
	if ids(q.tracks) != "ax"+up {
		t.Fatalf("queue=%s, upcoming order must survive", ids(q.tracks))
	}
}

// artists builds n tracks per artist, grouped (aaa...bbb...), IDs unique.
func artists(n int, names ...string) []Track {
	var ts []Track
	for _, a := range names {
		for i := 0; i < n; i++ {
			ts = append(ts, Track{ID: a + string(rune('0'+i)), Artist: a})
		}
	}
	return ts
}

func sameArtistNeighbours(ts []Track) int {
	n := 0
	for i := 1; i < len(ts); i++ {
		if ts[i].Artist == ts[i-1].Artist {
			n++
		}
	}
	return n
}

func TestShuffleSpreadsEachArtistAcrossTheQueue(t *testing.T) {
	for run := 0; run < 50; run++ {
		ts := artists(10, "A", "B", "C")
		shuffleTracks(ts)
		if len(ts) != 30 {
			t.Fatal("tracks lost")
		}
		seen := map[string]bool{}
		for _, tr := range ts {
			seen[tr.ID] = true
		}
		if len(seen) != 30 {
			t.Fatalf("duplicated tracks: %v", ids(ts))
		}
		// A plain shuffle leaves about 9 same-artist neighbours here; spread keeps it at 0 or 1.
		if got := sameArtistNeighbours(ts); got > 1 {
			t.Fatalf("run %d: %d same-artist neighbours", run, got)
		}
	}
}

func TestShuffleKeepsTracksWithoutArtistApart(t *testing.T) {
	ts := mk("abcdefgh") // no artist: each is its own group, nothing is lost
	shuffleTracks(ts)
	if len(ts) != 8 {
		t.Fatal("tracks lost")
	}
}

func TestOrderedStartKeepsTheGivenOrderEvenWithShuffleOn(t *testing.T) {
	var q queue
	q.shuffle = true
	q.set(mk("abcdef"), Ordered)
	if ids(q.tracks) != "abcdef" || q.idx != 0 {
		t.Fatalf("already-shuffled list must play as given: %s idx=%d", ids(q.tracks), q.idx)
	}
}

func TestSeparateArtistsBreaksRunsAndKeepsEveryTrack(t *testing.T) {
	ts := []Track{{ID: "1", Artist: "A"}, {ID: "2", Artist: "A"}, {ID: "3", Artist: "B"}, {ID: "4", Artist: "C"}, {ID: "5", Artist: "B"}}
	SeparateArtists(ts)
	if ids(ts) != "13245" || sameArtistNeighbours(ts) != 0 {
		t.Fatalf("got %s", ids(ts))
	}
	one := []Track{{ID: "1", Artist: "A"}, {ID: "2", Artist: "A"}}
	SeparateArtists(one) // nothing to swap with: left as is
	if ids(one) != "12" {
		t.Fatalf("got %s", ids(one))
	}
}
