//go:build windows

package island

import (
	"context"
	"errors"
	"time"

	"jbnplayer/internal/ipc"
	"jbnplayer/internal/player"
	"jbnplayer/internal/store"
	"jbnplayer/internal/subsonic"
)

const (
	pageSize   = 30
	coverSmall = 80
	coverMid   = 200
	coverBig   = 400
)

type albumView struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Artist    string `json:"artist"`
	Cover     string `json:"cover"`
	Year      int    `json:"year"`
	SongCount int    `json:"songCount"`
}

// trackView is a player.Track plus a ready-to-use cover URL. The page sends
// whole trackViews back when it asks to play them; unknown fields are ignored.
type trackView struct {
	player.Track
	Cover string `json:"cover"`
}

type albumsResult struct {
	Items   []albumView `json:"items"`
	Offline bool        `json:"offline"` // answered from the local copy
	Error   string      `json:"error"`
}

type tracksResult struct {
	Items []trackView `json:"items"`
	Error string      `json:"error"`
}

type searchResult struct {
	Albums  []albumView `json:"albums"`
	Songs   []trackView `json:"songs"`
	Offline bool        `json:"offline"` // answered from the local copy
	Error   string      `json:"error"`
}

// stateResult is the light player state polled every second.
type stateResult struct {
	QueueLen int        `json:"queueLen"`
	Version  uint64     `json:"version"` // changes when the queued tracks change
	Index    int        `json:"index"`
	Current  *trackView `json:"current"`
	Playing  bool       `json:"playing"`
	Shuffle  bool       `json:"shuffle"`
	Position float64    `json:"position"`
	Duration float64    `json:"duration"`
	Volume   int        `json:"volume"`
	Cover    string     `json:"cover"` // big cover of the current track
	Error    string     `json:"error"`
}

type queueResult struct {
	Items []trackView `json:"items"`
	From  int         `json:"from"`
	Total int         `json:"total"`
	Error string      `json:"error"`
}

// activeServer returns the server the player uses.
func (s *service) activeServer() (store.Server, error) {
	d, err := store.Load(s.path)
	if err != nil {
		return store.Server{}, err
	}
	sv, ok := d.ActiveServer()
	if !ok {
		return store.Server{}, errors.New("no server configured")
	}
	return sv, nil
}

// client returns a Subsonic client for the active server. It is cached while
// the server settings do not change, which keeps cover URLs stable.
func (s *service) client() (*subsonic.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	sv, err := s.activeServer()
	if err != nil {
		return nil, err
	}
	key := sv.URL + "|" + sv.User + "|" + sv.Password
	if s.cached == nil || s.cachedKey != key {
		s.cached, s.cachedKey = subsonic.New(sv.URL, sv.User, sv.Password), key
	}
	return s.cached, nil
}

func albumViews(c *subsonic.Client, as []subsonic.Album) []albumView {
	out := make([]albumView, 0, len(as))
	for _, a := range as {
		out = append(out, albumView{a.ID, a.Name, a.Artist, c.CoverURL(a.CoverArt, coverMid), a.Year, a.SongCount})
	}
	return out
}

func songTracks(c *subsonic.Client, ss []subsonic.Song) []trackView {
	out := make([]trackView, 0, len(ss))
	for _, s := range ss {
		out = append(out, trackView{
			Track: player.Track{ID: s.ID, Title: s.Title, Artist: s.Artist, Album: s.Album, CoverArt: s.CoverArt, Duration: s.Duration},
			Cover: c.CoverURL(s.CoverArt, coverSmall),
		})
	}
	return out
}

// Albums lists albums by kind (newest, random, alphabeticalByName, recent), paginated.
// Offline, it lists the stored albums by name whatever the kind.
func (s *service) Albums(kind string, offset int) albumsResult {
	if s.offline.Load() {
		return s.localAlbums(offset)
	}
	c, err := s.client()
	if err != nil {
		return albumsResult{Items: []albumView{}, Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	as, err := c.AlbumList(ctx, kind, pageSize, offset)
	if err != nil {
		if s.wentOffline(c) {
			return s.localAlbums(offset)
		}
		return albumsResult{Items: []albumView{}, Error: err.Error()}
	}
	return albumsResult{Items: albumViews(c, as)}
}

// AlbumTracks lists the songs of an album.
func (s *service) AlbumTracks(id string) tracksResult {
	if s.offline.Load() {
		return s.localAlbumTracks(id)
	}
	c, err := s.client()
	if err != nil {
		return tracksResult{Items: []trackView{}, Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	ss, err := c.AlbumSongs(ctx, id)
	if err != nil {
		if s.wentOffline(c) {
			return s.localAlbumTracks(id)
		}
		return tracksResult{Items: []trackView{}, Error: err.Error()}
	}
	return tracksResult{Items: songTracks(c, ss)}
}

// SearchLibrary looks for albums and songs.
func (s *service) SearchLibrary(q string) searchResult {
	if s.offline.Load() {
		return s.localSearch(q)
	}
	empty := searchResult{Albums: []albumView{}, Songs: []trackView{}}
	c, err := s.client()
	if err != nil {
		empty.Error = err.Error()
		return empty
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	res, err := c.Search(ctx, q)
	if err != nil {
		if s.wentOffline(c) {
			return s.localSearch(q)
		}
		empty.Error = err.Error()
		return empty
	}
	return searchResult{Albums: albumViews(c, res.Albums), Songs: songTracks(c, res.Songs)}
}

func (s *service) stateView(st player.State) stateResult {
	out := stateResult{
		QueueLen: st.QueueLen, Version: st.Version, Index: st.Index, Playing: st.Playing,
		Shuffle: st.Shuffle, Position: st.Position, Duration: st.Duration, Volume: st.Volume,
	}
	if st.Current != nil {
		tv := trackView{Track: *st.Current}
		tv.Cover = s.coverURL(st.Current.CoverArt, coverSmall)
		out.Cover = s.coverURL(st.Current.CoverArt, coverBig)
		out.Current = &tv
	}
	return out
}

// PlayerState fetches the light player snapshot from the tray process.
func (s *service) PlayerState() stateResult {
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	st, err := cl.State()
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	return s.stateView(st)
}

// PlayerQueue fetches a slice of the queue (the UI shows a window of it).
func (s *service) PlayerQueue(from, n int) queueResult {
	empty := queueResult{Items: []trackView{}}
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		empty.Error = err.Error()
		return empty
	}
	page, err := cl.Queue(from, n)
	if err != nil {
		empty.Error = err.Error()
		return empty
	}
	out := queueResult{Items: make([]trackView, 0, len(page.Tracks)), From: page.From, Total: page.Total}
	for _, t := range page.Tracks {
		tv := trackView{Track: t, Cover: s.coverURL(t.CoverArt, coverSmall)}
		out.Items = append(out.Items, tv)
	}
	return out
}

// PlayerDo sends a command (toggle, next, prev, clear, shuffle, jump, remove,
// play, enqueue, playnext, playnow, seek) to the player and returns the new state.
func (s *service) PlayerDo(op string, req ipc.Request) stateResult {
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	st, err := cl.Do(op, req)
	if err != nil {
		return stateResult{Error: err.Error()}
	}
	return s.stateView(st)
}
