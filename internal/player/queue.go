package player

import (
	"math/rand"
	"sort"
	"strconv"
	"strings"
)

// Track is a queued song. The JSON tags are the wire format used by the island.
type Track struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	CoverArt string `json:"coverArt"`
	Duration int    `json:"duration"`
}

// queue is the play queue: tracks plus the index of the current one.
// Shuffle reorders only what is still to come, so history stays intact.
type queue struct {
	tracks  []Track
	idx     int
	shuffle bool
	version uint64 // bumped whenever the list of tracks changes (not on index moves)
}

// shuffleTracks is a "balanced" shuffle (the method Spotify described in
// 2014): each artist's songs are shuffled among themselves and then spread
// evenly over the whole list, starting at a random offset and with a little
// jitter, so the same artist rarely plays twice in a row.
func shuffleTracks(ts []Track) {
	groups := map[string][]Track{}
	for i, t := range ts {
		key := strings.ToLower(strings.TrimSpace(t.Artist))
		if key == "" {
			key = "\x00" + strconv.Itoa(i) // no artist: a group of its own
		}
		groups[key] = append(groups[key], t)
	}
	type placed struct {
		pos float64
		t   Track
	}
	all := make([]placed, 0, len(ts))
	for _, g := range groups {
		rand.Shuffle(len(g), func(i, j int) { g[i], g[j] = g[j], g[i] })
		step := 1 / float64(len(g))
		offset := rand.Float64() * step
		for k, t := range g {
			jitter := (rand.Float64() - 0.5) * 0.2 * step
			all = append(all, placed{offset + float64(k)*step + jitter, t})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].pos < all[j].pos })
	for i, p := range all {
		ts[i] = p.t
	}
	SeparateArtists(ts)
}

func artistKey(t Track) string { return strings.ToLower(strings.TrimSpace(t.Artist)) }

// SeparateArtists breaks up back-to-back songs of the same artist by swapping
// the second one with the nearest later song of another artist. The order is
// otherwise kept, so a list ranked by preference stays roughly ranked.
func SeparateArtists(ts []Track) {
	for i := 1; i < len(ts); i++ {
		x := artistKey(ts[i])
		if x == "" || x != artistKey(ts[i-1]) {
			continue
		}
		// Move ts[i] to a slot j whose neighbours are not artist x either.
		for j := i + 1; j < len(ts); j++ {
			if artistKey(ts[j]) == x {
				continue
			}
			leftOK := j == i+1 || artistKey(ts[j-1]) != x
			rightOK := j+1 >= len(ts) || artistKey(ts[j+1]) != x
			if leftOK && rightOK {
				ts[i], ts[j] = ts[j], ts[i]
				break
			}
		}
	}
}

// AnyStart asks for "no preference" as the start index of a new queue: the
// first track normally, but a random one when shuffle is on.
const AnyStart = -1

// Ordered is a start index for a list the sender already shuffled (a weighted
// mix): it plays from the first track in the given order, even with shuffle on.
const Ordered = -2

// set replaces the queue and starts at start. With shuffle on, the chosen
// track plays first and the rest is shuffled.
func (q *queue) set(ts []Track, start int) {
	q.version++
	q.tracks = append([]Track(nil), ts...)
	if start == Ordered {
		q.idx = 0
		return
	}
	switch {
	case start == AnyStart && q.shuffle && len(q.tracks) > 1:
		start = rand.Intn(len(q.tracks))
	case start < 0 || start >= len(q.tracks):
		start = 0
	}
	q.idx = start
	if q.shuffle && len(q.tracks) > 1 {
		q.tracks[0], q.tracks[start] = q.tracks[start], q.tracks[0]
		q.idx = 0
		shuffleTracks(q.tracks[1:])
	}
}

func (q *queue) current() (Track, bool) {
	if q.idx < 0 || q.idx >= len(q.tracks) {
		return Track{}, false
	}
	return q.tracks[q.idx], true
}

// next advances; it reports false (and stays put) at the end of the queue.
func (q *queue) next() (Track, bool) {
	if q.idx+1 >= len(q.tracks) {
		return Track{}, false
	}
	q.idx++
	return q.current()
}

// prev goes back one track; on the first track it restarts it.
func (q *queue) prev() (Track, bool) {
	if q.idx > 0 {
		q.idx--
	}
	return q.current()
}

// insertNext puts ts right after the current track ("play next") and returns
// the index of the first inserted track.
func (q *queue) insertNext(ts []Track) int {
	q.version++
	at := q.idx + 1
	if len(q.tracks) == 0 {
		at = 0
	}
	out := make([]Track, 0, len(q.tracks)+len(ts))
	out = append(out, q.tracks[:at]...)
	out = append(out, ts...)
	out = append(out, q.tracks[at:]...)
	q.tracks = out
	return at
}

// playNow makes t the current track without dropping the rest of the queue:
// it goes right after the current one, so the queue carries on as it was
// arranged once t ends. A t already queued is moved, never duplicated. It
// reports whether t must be loaded (false when t is already playing).
func (q *queue) playNow(t Track) bool {
	if cur, ok := q.current(); ok && cur.ID == t.ID {
		return false
	}
	for i, x := range q.tracks {
		if x.ID == t.ID {
			q.remove(i) // never the current one: checked above
			break
		}
	}
	at := q.insertNext([]Track{t})
	q.idx = at
	return true
}

// appendTracks adds ts at the end ("add to queue") and returns the index of the first one.
func (q *queue) appendTracks(ts []Track) int {
	q.version++
	at := len(q.tracks)
	q.tracks = append(q.tracks, ts...)
	return at
}

// jump makes i the current track.
func (q *queue) jump(i int) bool {
	if i < 0 || i >= len(q.tracks) {
		return false
	}
	q.idx = i
	return true
}

// move takes the track at from and puts it at index to (its final position).
// The current track keeps playing: the index follows it.
func (q *queue) move(from, to int) bool {
	n := len(q.tracks)
	if from < 0 || from >= n || to < 0 || to >= n {
		return false
	}
	if from == to {
		return true
	}
	t := q.tracks[from]
	q.tracks = append(q.tracks[:from], q.tracks[from+1:]...)                  // take it out...
	q.tracks = append(q.tracks[:to], append([]Track{t}, q.tracks[to:]...)...) // ...and put it back at to
	switch {
	case q.idx == from:
		q.idx = to
	case from < q.idx && to >= q.idx:
		q.idx--
	case from > q.idx && to <= q.idx:
		q.idx++
	}
	q.version++
	return true
}

// remove deletes a queued track. The current track cannot be removed.
func (q *queue) remove(i int) bool {
	if i < 0 || i >= len(q.tracks) || i == q.idx {
		return false
	}
	q.version++
	q.tracks = append(q.tracks[:i], q.tracks[i+1:]...)
	if i < q.idx {
		q.idx--
	}
	return true
}

// clearUpcoming drops everything after the current track.
func (q *queue) clearUpcoming() {
	if q.idx+1 < len(q.tracks) {
		q.version++
		q.tracks = q.tracks[:q.idx+1]
	}
}

// setShuffle turns shuffle on or off. Turning it on shuffles the upcoming
// tracks once; turning it off keeps the current order (it cannot be restored).
func (q *queue) setShuffle(on bool) {
	q.shuffle = on
	if on && q.idx+1 < len(q.tracks) {
		q.version++
		shuffleTracks(q.tracks[q.idx+1:])
	}
}
