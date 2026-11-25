//go:build windows

package island

import (
	"testing"

	"jbnplayer/internal/ipc"
	"jbnplayer/internal/library"
	"jbnplayer/internal/player"
)

// mixService has a synced library with genres and years, and a fake player.
func mixService(t *testing.T) (*service, *ctl) {
	t.Helper()
	useRoot(t)
	s := libService(t)
	sv, _ := s.activeServer()
	c := &library.Cache{Version: 1, Songs: []library.Song{
		{ID: "r1", Title: "Rock one", Artist: "A", Genres: []string{"Rock"}, Year: 1985, Starred: true},
		{ID: "r2", Title: "Rock two", Artist: "B", Genres: []string{"Rock"}, Year: 1994},
		{ID: "c1", Title: "Cumbia", Artist: "C", Genres: []string{"Cumbia"}, Year: 1988},
		{ID: "x1", Title: "No tags", Artist: "D"},
	}}
	if err := c.Save(s.libPath(sv.ID)); err != nil {
		t.Fatal(err)
	}
	fake := &ctl{}
	stop, err := ipc.Serve(s.ipcPath, fake)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	return s, fake
}

func queued(c *ctl) map[string]bool {
	m := map[string]bool{}
	for _, tr := range c.queue {
		m[tr.ID] = true
	}
	return m
}

func TestMixOptionsListGenresAndDecades(t *testing.T) {
	s, _ := mixService(t)
	o := s.MixOptions()
	if o.Error != "" || len(o.Genres) != 2 || o.Genres[0].Name != "Rock" || o.Genres[0].Count != 2 {
		t.Fatalf("genres: %+v", o)
	}
	if len(o.Decades) != 2 || o.Decades[0].Name != "1980" || o.Decades[0].Count != 2 || o.NeedsSync {
		t.Fatalf("decades: %+v", o)
	}
}

func TestMixOptionsAskForASyncWhenTheCopyHasNoTags(t *testing.T) {
	s := syncedService(t) // a copy saved without genres or years
	if o := s.MixOptions(); !o.NeedsSync || len(o.Genres) != 0 {
		t.Fatalf("got %+v", o)
	}
}

func TestPlayMixQueuesOnlyMatchingSongsInWeightedOrder(t *testing.T) {
	s, fake := mixService(t)
	f := library.MixFilter{Genres: []string{"rock"}}
	if n := s.MixCount(f); n.Count != 2 || n.Error != "" {
		t.Fatalf("count: %+v", n)
	}
	st := s.PlayMix(f)
	if st.Error != "" || !st.Shuffle || st.QueueLen != 2 {
		t.Fatalf("play mix: %+v", st)
	}
	if q := queued(fake); !q["r1"] || !q["r2"] {
		t.Fatalf("queue: %+v", fake.queue)
	}
	if fake.start != player.Ordered {
		t.Fatalf("the weighted order must be kept by the player: start %d", fake.start)
	}
	if st := s.PlayMix(library.MixFilter{Genres: []string{"Jazz"}}); st.Error != "no songs match this mix" {
		t.Fatalf("empty mix: %+v", st)
	}
	if n := s.MixCount(library.MixFilter{Decades: []int{1980}}); n.Count != 2 {
		t.Fatalf("decade count: %+v", n)
	}
}

func TestShuffleAllIsWeightedAndKeepsItsOrder(t *testing.T) {
	s, fake := mixService(t)
	if st := s.PlayAll(true); st.Error != "" || !st.Shuffle || st.QueueLen != 4 {
		t.Fatalf("shuffle all: %+v", st)
	}
	if fake.start != player.Ordered {
		t.Fatalf("start %d, want Ordered", fake.start)
	}
	if st := s.PlayAll(false); st.Error != "" || st.Shuffle || fake.start != player.AnyStart {
		t.Fatalf("play all in order is unchanged: %+v start %d", st, fake.start)
	}
}
