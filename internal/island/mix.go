//go:build windows

package island

import (
	"math/rand"
	"time"

	"jbnplayer/internal/ipc"
	"jbnplayer/internal/library"
	"jbnplayer/internal/player"
)

// Mixes: "Shuffle all" and the genre/decade mixes of the Songs tab play in a
// weighted order (library.Weight) computed here, with the same artist kept
// apart; the player receives it with player.Ordered so it is not reshuffled.
// Offline, only the stored songs take part.

type mixOptions struct {
	Genres    []library.Count `json:"genres"`
	Decades   []library.Count `json:"decades"`
	NeedsSync bool            `json:"needsSync"` // the local copy predates genres and years
	Error     string          `json:"error"`
}

type mixCount struct {
	Count int    `json:"count"`
	Error string `json:"error"`
}

// MixOptions lists the genres and decades to build a mix from.
func (s *service) MixOptions() mixOptions {
	empty := mixOptions{Genres: []library.Count{}, Decades: []library.Count{}}
	c, keep, err := s.localLib()
	if err != nil {
		empty.Error = err.Error()
		return empty
	}
	if c == nil {
		empty.NeedsSync = true
		return empty
	}
	g, d := c.MixOptions(keep)
	out := mixOptions{Genres: g, Decades: d}
	if out.Genres == nil {
		out.Genres = []library.Count{}
	}
	if out.Decades == nil {
		out.Decades = []library.Count{}
	}
	out.NeedsSync = len(g) == 0 && len(d) == 0 && len(c.Songs) > 0
	return out
}

// MixCount says how many songs a mix would play.
func (s *service) MixCount(f library.MixFilter) mixCount {
	c, keep, err := s.localLib()
	if err != nil {
		return mixCount{Error: err.Error()}
	}
	if c == nil {
		return mixCount{}
	}
	return mixCount{Count: len(c.Mix(f, keep))}
}

// PlayMix replaces the queue with the songs of a mix, in weighted order.
func (s *service) PlayMix(f library.MixFilter) stateResult {
	c, keep, err := s.localLib()
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	if c == nil {
		return stateResult{Error: "library not synced yet"}
	}
	songs := c.Mix(f, keep)
	if len(songs) == 0 {
		return stateResult{Error: "no songs match this mix"}
	}
	return s.playWeighted(songs)
}

// playWeighted turns shuffle on and plays songs in weighted order.
func (s *service) playWeighted(songs []library.Song) stateResult {
	ordered := library.WeightedOrder(songs, time.Now(), rand.New(rand.NewSource(time.Now().UnixNano())))
	tracks := make([]player.Track, len(ordered))
	for i, x := range ordered {
		tracks[i] = libTrack(x)
	}
	player.SeparateArtists(tracks)
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	if _, err := cl.Do("shuffle", ipc.Request{On: true}); err != nil {
		return stateResult{Error: err.Error()}
	}
	st, err := cl.Do("play", ipc.Request{Tracks: tracks, Start: player.Ordered})
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	return s.stateView(st)
}
