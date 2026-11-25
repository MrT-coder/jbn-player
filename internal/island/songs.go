//go:build windows

package island

import (
	"context"
	"path/filepath"
	"time"

	"jbnplayer/internal/ipc"
	"jbnplayer/internal/library"
	"jbnplayer/internal/player"
	"jbnplayer/internal/subsonic"
)

type syncInfo struct {
	Synced    bool   `json:"synced"`
	At        string `json:"at"` // RFC 3339, empty when never synced
	Songs     int    `json:"songs"`
	Playlists int    `json:"playlists"`
	Error     string `json:"error"`
}

type songsResult struct {
	Items  []trackView `json:"items"`
	Total  int         `json:"total"`
	Synced bool        `json:"synced"`
	Error  string      `json:"error"`
}

type playlistView struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Count int    `json:"count"`
	Cover string `json:"cover"`
}

type playlistsResult struct {
	Items  []playlistView `json:"items"`
	Synced bool           `json:"synced"`
	Error  string         `json:"error"`
}

func (s *service) libPath(serverID string) string {
	return filepath.Join(s.libDir, "library-"+serverID+".json")
}

// loadLib returns the local copy for the active server (nil if never synced).
func (s *service) loadLib() (*library.Cache, error) {
	s.mu.Lock()
	sv, err := s.activeServer()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if s.lib != nil && s.libServer == sv.ID {
		return s.lib, nil
	}
	c, err := library.Load(s.libPath(sv.ID))
	if err != nil {
		return nil, err
	}
	s.lib, s.libServer = c, sv.ID
	return c, nil
}

func infoOf(c *library.Cache) syncInfo {
	if c == nil {
		return syncInfo{}
	}
	return syncInfo{Synced: true, At: c.SyncedAt.Format(time.RFC3339), Songs: len(c.Songs), Playlists: len(c.Playlists)}
}

// SyncInfo reports what the local copy holds.
func (s *service) SyncInfo() syncInfo {
	c, err := s.loadLib()
	if err != nil {
		return syncInfo{Error: err.Error()}
	}
	return infoOf(c)
}

// SyncLibrary downloads every song and playlist from the server and replaces
// the local copy. If anything fails the previous copy is kept.
func (s *service) SyncLibrary() syncInfo {
	s.mu.Lock()
	sv, err := s.activeServer()
	s.mu.Unlock()
	if err != nil {
		return syncInfo{Error: err.Error()}
	}
	if !s.syncMu.TryLock() {
		return syncInfo{Error: "a sync is already running"}
	}
	defer s.syncMu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	c, err := library.Sync(ctx, subsonic.New(sv.URL, sv.User, sv.Password))
	if err != nil {
		return syncInfo{Error: err.Error()}
	}
	if err := c.Save(s.libPath(sv.ID)); err != nil {
		return syncInfo{Error: err.Error()}
	}
	s.lib, s.libServer = c, sv.ID
	return infoOf(c)
}

func (s *service) libTrackViews(ss []library.Song) []trackView {
	out := make([]trackView, 0, len(ss))
	for _, x := range ss {
		out = append(out, trackView{Track: libTrack(x), Cover: s.coverURL(x.CoverArt, coverSmall)})
	}
	return out
}

// Songs lists the library's songs (all of them, no album needed), filtered by q.
// Offline, only the stored ones.
func (s *service) Songs(q string, offset, limit int) songsResult {
	c, keep, err := s.localLib()
	if err != nil {
		return songsResult{Items: []trackView{}, Error: err.Error()}
	}
	if c == nil {
		return songsResult{Items: []trackView{}}
	}
	songs, total := c.FindWhere(q, offset, limit, keep)
	return songsResult{Items: s.libTrackViews(songs), Total: total, Synced: true}
}

// Playlists lists the server's playlists from the local copy. Offline, only
// the playlists with stored songs, counting those.
func (s *service) Playlists() playlistsResult {
	c, keep, err := s.localLib()
	if err != nil {
		return playlistsResult{Items: []playlistView{}, Error: err.Error()}
	}
	if c == nil {
		return playlistsResult{Items: []playlistView{}}
	}
	out := playlistsResult{Items: make([]playlistView, 0, len(c.Playlists)), Synced: true}
	for _, p := range c.Playlists {
		songs, _ := c.PlaylistSongs(p.ID)
		songs = only(songs, keep)
		if keep != nil && len(songs) == 0 {
			continue
		}
		v := playlistView{ID: p.ID, Name: p.Name, Count: len(songs)}
		for _, sg := range songs {
			if sg.CoverArt != "" {
				v.Cover = s.coverURL(sg.CoverArt, coverMid)
				break
			}
		}
		out.Items = append(out.Items, v)
	}
	return out
}

// PlaylistTracks lists the songs of a playlist, in playlist order.
func (s *service) PlaylistTracks(id string) tracksResult {
	c, keep, err := s.localLib()
	if err != nil {
		return tracksResult{Items: []trackView{}, Error: err.Error()}
	}
	if c == nil {
		return tracksResult{Items: []trackView{}, Error: "library not synced yet"}
	}
	songs, ok := c.PlaylistSongs(id)
	if !ok {
		return tracksResult{Items: []trackView{}, Error: "playlist not found (sync again?)"}
	}
	return tracksResult{Items: s.libTrackViews(only(songs, keep))}
}

// only returns the songs keep accepts (all of them when keep is nil).
func only(ss []library.Song, keep func(library.Song) bool) []library.Song {
	if keep == nil {
		return ss
	}
	out := make([]library.Song, 0, len(ss))
	for _, x := range ss {
		if keep(x) {
			out = append(out, x)
		}
	}
	return out
}

// PlayAll replaces the one queue with the whole library: in title order, or
// in weighted shuffle order. Shuffle stays on/off as chosen here.
// Offline, only the stored songs.
func (s *service) PlayAll(shuffle bool) stateResult {
	c, keep, err := s.localLib()
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	if c == nil || len(c.Songs) == 0 {
		return stateResult{Error: "library not synced yet"}
	}
	songs := only(c.Songs, keep)
	if len(songs) == 0 {
		return stateResult{Error: "no songs are stored offline yet"}
	}
	if shuffle {
		return s.playWeighted(songs) // favorites first more often, artists kept apart (mix.go)
	}
	tracks := make([]player.Track, len(songs))
	for i, x := range songs {
		tracks[i] = libTrack(x)
	}
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	if _, err := cl.Do("shuffle", ipc.Request{On: false}); err != nil {
		return stateResult{Error: err.Error()}
	}
	st, err := cl.Do("play", ipc.Request{Tracks: tracks, Start: player.AnyStart})
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	return s.stateView(st)
}
