package library

import (
	"math"
	"math/rand"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Weighted shuffle and genre/decade mixes, built only from data Navidrome
// already sends (stars, ratings, play counts, last play, genres, year).

// Weight is how likely a song is to come early in a weighted shuffle. A plain
// song weighs 1: favorites, high ratings and often played songs weigh more,
// low ratings and songs played in the last week weigh less.
func Weight(s Song, now time.Time) float64 {
	w := 1.0
	if s.Starred {
		w *= 3
	}
	switch s.Rating {
	case 5:
		w *= 3
	case 4:
		w *= 2
	case 2:
		w *= 0.5
	case 1:
		w *= 0.2
	}
	w *= 1 + math.Log1p(float64(s.PlayCount))/3 // 10 plays: x1.8, 100 plays: x2.5
	if s.Played > 0 {
		switch ago := now.Sub(time.Unix(s.Played, 0)); {
		case ago < 2*24*time.Hour:
			w *= 0.15
		case ago < 7*24*time.Hour:
			w *= 0.5
		}
	}
	return w
}

// WeightedOrder returns the songs in a random order where heavier songs tend
// to come first (Efraimidis-Spirakis: sort by u^(1/w), u uniform in (0,1)).
// Every song appears once; ss itself is not modified.
func WeightedOrder(ss []Song, now time.Time, r *rand.Rand) []Song {
	type keyed struct {
		key float64
		s   Song
	}
	ks := make([]keyed, len(ss))
	for i, s := range ss {
		u := r.Float64()
		for u == 0 {
			u = r.Float64()
		}
		ks[i] = keyed{math.Pow(u, 1/Weight(s, now)), s}
	}
	sort.SliceStable(ks, func(i, j int) bool { return ks[i].key > ks[j].key })
	out := make([]Song, len(ks))
	for i, k := range ks {
		out[i] = k.s
	}
	return out
}

// MixFilter picks songs by genre and decade. Inside a group any value
// matches; when both groups are set, a song must match both. Empty = all.
type MixFilter struct {
	Genres  []string `json:"genres"`
	Decades []int    `json:"decades"` // 1980 means 1980-1989
}

func (f MixFilter) match(s Song) bool {
	if len(f.Decades) > 0 {
		ok := false
		for _, d := range f.Decades {
			if s.Year > 0 && s.Year/10*10 == d {
				ok = true
				break
			}
		}
		if !ok {
			return false
		}
	}
	if len(f.Genres) > 0 {
		for _, want := range f.Genres {
			for _, g := range s.Genres {
				if strings.EqualFold(g, want) {
					return true
				}
			}
		}
		return false
	}
	return true
}

// Mix returns the songs matching f that keep accepts (nil keeps all), in
// library order.
func (c *Cache) Mix(f MixFilter, keep func(Song) bool) []Song {
	var out []Song
	for _, s := range c.Songs {
		if (keep == nil || keep(s)) && f.match(s) {
			out = append(out, s)
		}
	}
	return out
}

// Count is one choice of the mix panel and how many songs it has.
type Count struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
}

// MixOptions lists the genres (most songs first) and decades (oldest first)
// of the songs keep accepts. Genres differing only in case are one genre,
// named as first seen.
func (c *Cache) MixOptions(keep func(Song) bool) (genres, decades []Count) {
	gi, di := map[string]int{}, map[int]int{}
	for _, s := range c.Songs {
		if keep != nil && !keep(s) {
			continue
		}
		seen := map[string]bool{}
		for _, g := range s.Genres {
			k := strings.ToLower(strings.TrimSpace(g))
			if k == "" || seen[k] {
				continue
			}
			seen[k] = true
			if i, ok := gi[k]; ok {
				genres[i].Count++
			} else {
				gi[k] = len(genres)
				genres = append(genres, Count{strings.TrimSpace(g), 1})
			}
		}
		if s.Year > 0 {
			di[s.Year/10*10]++
		}
	}
	sort.SliceStable(genres, func(i, j int) bool {
		if genres[i].Count != genres[j].Count {
			return genres[i].Count > genres[j].Count
		}
		return fold(genres[i].Name) < fold(genres[j].Name)
	})
	ds := make([]int, 0, len(di))
	for d := range di {
		ds = append(ds, d)
	}
	sort.Ints(ds)
	for _, d := range ds {
		decades = append(decades, Count{strconv.Itoa(d), di[d]})
	}
	return genres, decades
}
