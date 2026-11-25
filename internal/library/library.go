// Package library keeps a local copy of the server's songs and playlists so
// browsing, searching and "shuffle all" are instant and work without the server.
// The copy is replaced as a whole by Sync; nothing here ever writes to the server.
package library

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"jbnplayer/internal/subsonic"
)

const (
	cacheVersion = 1
	pageSize     = 500
	maxPages     = 400 // 200k songs: a guard against a server that never stops paging
)

// Song is one track of the library.
type Song struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	AlbumID  string `json:"albumId"`
	CoverArt string `json:"coverArt"`
	Duration int    `json:"duration"`
	Track    int    `json:"track"`

	// Listening data for the weighted shuffle and the mixes (mix.go). It is
	// as fresh as the last sync; copies saved before these fields existed
	// simply lack them until the next sync.
	Genres    []string `json:"genres,omitempty"`
	Year      int      `json:"year,omitempty"`
	PlayCount int      `json:"playCount,omitempty"`
	Starred   bool     `json:"starred,omitempty"`
	Rating    int      `json:"rating,omitempty"` // 1-5, 0 = not rated
	Played    int64    `json:"played,omitempty"` // last play, Unix seconds
}

// Playlist references songs by ID, in playlist order.
type Playlist struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	SongIDs []string `json:"songIds"`
}

// Cache is the whole local copy.
type Cache struct {
	Version   int        `json:"version"`
	SyncedAt  time.Time  `json:"syncedAt"`
	Songs     []Song     `json:"songs"` // sorted by title
	Playlists []Playlist `json:"playlists"`

	byID   map[string]int // song ID -> index in Songs
	folded []string       // searchable text per song, same order as Songs
}

// Source is the part of the Subsonic client that Sync needs.
type Source interface {
	AllSongs(ctx context.Context, offset, count int) ([]subsonic.Song, error)
	Playlists(ctx context.Context) ([]subsonic.Playlist, error)
	PlaylistSongs(ctx context.Context, id string) ([]subsonic.Song, error)
}

var foldReplacer = strings.NewReplacer(
	"á", "a", "à", "a", "ä", "a", "â", "a", "ã", "a",
	"é", "e", "è", "e", "ë", "e", "ê", "e",
	"í", "i", "ì", "i", "ï", "i", "î", "i",
	"ó", "o", "ò", "o", "ö", "o", "ô", "o", "õ", "o",
	"ú", "u", "ù", "u", "ü", "u", "û", "u",
	"ñ", "n", "ç", "c",
)

// fold lowercases and removes common accents so "Águila" matches "aguila".
func fold(s string) string { return foldReplacer.Replace(strings.ToLower(s)) }

func (c *Cache) index() {
	sort.SliceStable(c.Songs, func(i, j int) bool { return fold(c.Songs[i].Title) < fold(c.Songs[j].Title) })
	c.byID = make(map[string]int, len(c.Songs))
	c.folded = make([]string, len(c.Songs))
	for i, s := range c.Songs {
		c.byID[s.ID] = i
		c.folded[i] = fold(s.Title + "\n" + s.Artist + "\n" + s.Album)
	}
}

func fromSubsonic(s subsonic.Song) Song {
	out := Song{ID: s.ID, Title: s.Title, Artist: s.Artist, Album: s.Album, AlbumID: s.AlbumID,
		CoverArt: s.CoverArt, Duration: s.Duration, Track: s.Track,
		Year: s.Year, PlayCount: s.PlayCount, Starred: s.Starred != "", Rating: s.UserRating}
	for _, g := range s.Genres {
		if g.Name != "" {
			out.Genres = append(out.Genres, g.Name)
		}
	}
	if len(out.Genres) == 0 && s.Genre != "" {
		out.Genres = []string{s.Genre}
	}
	if t, err := time.Parse(time.RFC3339, s.Played); err == nil {
		out.Played = t.Unix()
	}
	return out
}

// Sync downloads every song and playlist. The result is only meaningful if
// err is nil: on failure the caller keeps whatever copy it already had.
func Sync(ctx context.Context, src Source) (*Cache, error) {
	c := &Cache{Version: cacheVersion, SyncedAt: time.Now().UTC()}
	seen := map[string]bool{}
	for page := 0; page < maxPages; page++ {
		songs, err := src.AllSongs(ctx, page*pageSize, pageSize)
		if err != nil {
			return nil, fmt.Errorf("songs: %w", err)
		}
		for _, s := range songs {
			if s.ID != "" && !seen[s.ID] {
				seen[s.ID] = true
				c.Songs = append(c.Songs, fromSubsonic(s))
			}
		}
		if len(songs) < pageSize {
			break
		}
	}
	pls, err := src.Playlists(ctx)
	if err != nil {
		return nil, fmt.Errorf("playlists: %w", err)
	}
	for _, p := range pls {
		entries, err := src.PlaylistSongs(ctx, p.ID)
		if err != nil {
			return nil, fmt.Errorf("playlist %q: %w", p.Name, err)
		}
		pl := Playlist{ID: p.ID, Name: p.Name, SongIDs: make([]string, 0, len(entries))}
		for _, e := range entries {
			if !seen[e.ID] { // a playlist song the paged list missed (library changed mid-sync)
				seen[e.ID] = true
				c.Songs = append(c.Songs, fromSubsonic(e))
			}
			pl.SongIDs = append(pl.SongIDs, e.ID)
		}
		c.Playlists = append(c.Playlists, pl)
	}
	c.index()
	return c, nil
}

// RefreshPlaylist re-reads one playlist from the server (a few songs were
// added to it) and returns an updated copy; c itself is left untouched, since
// other readers may be using it. Songs new to the copy are added to the
// library list. A playlist the copy does not know yet is added as name.
func (c *Cache) RefreshPlaylist(ctx context.Context, src Source, id, name string) (*Cache, error) {
	entries, err := src.PlaylistSongs(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("playlist %q: %w", name, err)
	}
	n := &Cache{Version: c.Version, SyncedAt: c.SyncedAt,
		Songs:     append([]Song(nil), c.Songs...),
		Playlists: append([]Playlist(nil), c.Playlists...)}
	pl := Playlist{ID: id, Name: name, SongIDs: make([]string, 0, len(entries))}
	added := map[string]bool{}
	for _, e := range entries {
		if _, ok := c.byID[e.ID]; !ok && !added[e.ID] { // a playlist may list a song twice
			added[e.ID] = true
			n.Songs = append(n.Songs, fromSubsonic(e))
		}
		pl.SongIDs = append(pl.SongIDs, e.ID)
	}
	found := false
	for i, p := range n.Playlists {
		if p.ID == id {
			pl.Name = p.Name // keep the name the full sync saw
			n.Playlists[i] = pl
			found = true
			break
		}
	}
	if !found {
		n.Playlists = append(n.Playlists, pl)
	}
	n.index()
	return n, nil
}

// Load reads a cache file. A missing file is (nil, nil): not synced yet.
func Load(path string) (*Cache, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var c Cache
	if err := json.Unmarshal(raw, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Version != cacheVersion {
		return nil, nil // unknown layout: treat as not synced, the next Sync rewrites it
	}
	c.index()
	return &c, nil
}

// Save writes the cache atomically (temp file + rename).
func (c *Cache) Save(path string) error {
	raw, err := json.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Find returns the songs matching q (all words must appear in title, artist
// or album; empty q matches everything), plus the total number of matches.
func (c *Cache) Find(q string, offset, limit int) ([]Song, int) {
	return c.FindWhere(q, offset, limit, nil)
}

// FindWhere is Find limited to the songs keep accepts (nil keeps all); the
// filter runs before paging, so the total counts only kept songs.
func (c *Cache) FindWhere(q string, offset, limit int, keep func(Song) bool) ([]Song, int) {
	words := strings.Fields(fold(q))
	var matches []int
	for i := range c.Songs {
		if keep != nil && !keep(c.Songs[i]) {
			continue
		}
		ok := true
		for _, w := range words {
			if !strings.Contains(c.folded[i], w) {
				ok = false
				break
			}
		}
		if ok {
			matches = append(matches, i)
		}
	}
	total := len(matches)
	if offset < 0 {
		offset = 0
	}
	if offset > total {
		offset = total
	}
	end := total
	if limit > 0 && offset+limit < end {
		end = offset + limit
	}
	out := make([]Song, 0, end-offset)
	for _, i := range matches[offset:end] {
		out = append(out, c.Songs[i])
	}
	return out, total
}

// Album is an album rebuilt from the local copy (used when the server is off).
type Album struct {
	ID        string
	Name      string
	Artist    string
	CoverArt  string
	SongCount int
}

// Albums groups the songs keep accepts (nil keeps all) into albums, sorted by name.
func (c *Cache) Albums(keep func(Song) bool) []Album {
	at := map[string]int{}
	var out []Album
	for _, s := range c.Songs {
		if s.AlbumID == "" || (keep != nil && !keep(s)) {
			continue
		}
		if i, ok := at[s.AlbumID]; ok {
			out[i].SongCount++
			if out[i].CoverArt == "" {
				out[i].CoverArt = s.CoverArt
			}
			continue
		}
		at[s.AlbumID] = len(out)
		out = append(out, Album{ID: s.AlbumID, Name: s.Album, Artist: s.Artist, CoverArt: s.CoverArt, SongCount: 1})
	}
	sort.SliceStable(out, func(i, j int) bool { return fold(out[i].Name) < fold(out[j].Name) })
	return out
}

// AlbumSongs returns the songs of an album that keep accepts, in track order.
func (c *Cache) AlbumSongs(id string, keep func(Song) bool) []Song {
	var out []Song
	for _, s := range c.Songs {
		if s.AlbumID == id && (keep == nil || keep(s)) {
			out = append(out, s)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Track < out[j].Track })
	return out
}

// PlaylistSongs returns a playlist's songs in order (songs no longer in the
// library are skipped), or false for an unknown playlist.
func (c *Cache) PlaylistSongs(id string) ([]Song, bool) {
	for _, p := range c.Playlists {
		if p.ID != id {
			continue
		}
		out := make([]Song, 0, len(p.SongIDs))
		for _, sid := range p.SongIDs {
			if i, ok := c.byID[sid]; ok {
				out = append(out, c.Songs[i])
			}
		}
		return out, true
	}
	return nil, false
}
