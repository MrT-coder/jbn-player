//go:build windows

package island

import (
	"context"
	"time"

	"jbnplayer/internal/library"
	"jbnplayer/internal/player"
	"jbnplayer/internal/subsonic"
)

// Offline mode: when the server does not answer (no network, server off, wrong
// URL or password), the popup shows only the songs stored on this PC, built
// from the local library copy, and asks the server for nothing; covers come
// from the offline folder (covers.go).
// The popup checks on open and every so often while it is open; a failed live
// request also checks at once. Nothing runs while the popup is closed.

const pingTimeout = 3 * time.Second

type serverStatus struct {
	Offline bool   `json:"offline"`
	Error   string `json:"error"`
}

// ServerStatus pings the active server and remembers the answer. Having no
// server configured is a setup problem shown elsewhere, not offline mode.
func (s *service) ServerStatus() serverStatus {
	c, err := s.client()
	if err != nil {
		s.offline.Store(false)
		return serverStatus{}
	}
	s.offline.Store(!reachable(c))
	if !s.offline.Load() {
		s.startBackfill()
	}
	return serverStatus{Offline: s.offline.Load()}
}

func reachable(c *subsonic.Client) bool {
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout)
	defer cancel()
	return c.Ping(ctx) == nil
}

// wentOffline is called after a live request failed: if a ping fails too, the
// popup switches to offline mode and the caller answers from the local copy.
func (s *service) wentOffline(c *subsonic.Client) bool {
	if reachable(c) {
		return false
	}
	s.offline.Store(true)
	return true
}

// keep returns the offline filter (only stored songs), or nil while online.
func (s *service) keep() (func(library.Song) bool, error) {
	if !s.offline.Load() {
		return nil, nil
	}
	ids, err := s.storedIDs()
	if err != nil {
		return nil, err
	}
	set := make(map[string]bool, len(ids))
	for _, id := range ids {
		set[id] = true
	}
	return func(x library.Song) bool { return set[x.ID] }, nil
}

// localLib returns the local copy and the offline filter together.
func (s *service) localLib() (*library.Cache, func(library.Song) bool, error) {
	c, err := s.loadLib()
	if err != nil {
		return nil, nil, err
	}
	keep, err := s.keep()
	if err != nil {
		return nil, nil, err
	}
	return c, keep, nil
}

func (s *service) localAlbumViews(as []library.Album) []albumView {
	out := make([]albumView, 0, len(as))
	for _, a := range as {
		out = append(out, albumView{ID: a.ID, Name: a.Name, Artist: a.Artist, SongCount: a.SongCount, Cover: s.coverURL(a.CoverArt, coverMid)})
	}
	return out
}

// localAlbums lists the stored albums by name, paginated like the live list.
func (s *service) localAlbums(offset int) albumsResult {
	c, keep, err := s.localLib()
	if err != nil {
		return albumsResult{Items: []albumView{}, Offline: true, Error: err.Error()}
	}
	if c == nil {
		return albumsResult{Items: []albumView{}, Offline: true}
	}
	as := c.Albums(keep)
	offset = min(max(offset, 0), len(as))
	return albumsResult{Items: s.localAlbumViews(as[offset:min(offset+pageSize, len(as))]), Offline: true}
}

func (s *service) localAlbumTracks(id string) tracksResult {
	c, keep, err := s.localLib()
	if err != nil {
		return tracksResult{Items: []trackView{}, Error: err.Error()}
	}
	if c == nil {
		return tracksResult{Items: []trackView{}, Error: "library not synced yet"}
	}
	return tracksResult{Items: s.libTrackViews(c.AlbumSongs(id, keep))}
}

// localSearch finds stored songs, and the albums they belong to.
func (s *service) localSearch(q string) searchResult {
	res := searchResult{Albums: []albumView{}, Songs: []trackView{}, Offline: true}
	c, keep, err := s.localLib()
	if err != nil {
		res.Error = err.Error()
		return res
	}
	if c == nil {
		return res
	}
	songs, _ := c.FindWhere(q, 0, searchLimit, keep)
	res.Songs = s.libTrackViews(songs)
	res.Albums = s.localAlbumViews(c.Albums(func(x library.Song) bool {
		return (keep == nil || keep(x)) && inSongs(songs, x.AlbumID)
	}))
	return res
}

const searchLimit = 50

func inSongs(ss []library.Song, albumID string) bool {
	for _, x := range ss {
		if x.AlbumID == albumID {
			return true
		}
	}
	return false
}

func libTrack(x library.Song) player.Track {
	return player.Track{ID: x.ID, Title: x.Title, Artist: x.Artist, Album: x.Album, CoverArt: x.CoverArt, Duration: x.Duration}
}
