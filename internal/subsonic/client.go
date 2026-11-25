// Package subsonic is a minimal Subsonic/Navidrome API client (token auth, JSON).
package subsonic

import (
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const apiVersion = "1.16.1"

// Client talks to a Subsonic-compatible server.
type Client struct {
	base, user, pass string
	http             *http.Client

	// Fixed salt/token used only for cover art URLs. The same URL must be
	// produced on every call so the browser can cache the images.
	coverSalt, coverToken string
}

// New returns a client. base must not end with a slash.
func New(base, user, pass string) *Client {
	c := &Client{base: base, user: user, pass: pass, http: &http.Client{Timeout: 20 * time.Second}}
	c.coverSalt = randomSalt()
	c.coverToken = token(pass, c.coverSalt)
	return c
}

// Album is a library album.
type Album struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Artist    string `json:"artist"`
	CoverArt  string `json:"coverArt"`
	SongCount int    `json:"songCount"`
	Year      int    `json:"year"`
}

// Song is a playable track.
type Song struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Artist   string `json:"artist"`
	Album    string `json:"album"`
	AlbumID  string `json:"albumId"`
	CoverArt string `json:"coverArt"`
	Duration int    `json:"duration"`
	Track    int    `json:"track"`

	// Used by the weighted shuffle and the genre/decade mixes.
	Genre      string  `json:"genre"`  // Subsonic: one genre
	Genres     []Named `json:"genres"` // OpenSubsonic: every genre
	Year       int     `json:"year"`
	PlayCount  int     `json:"playCount"`  // plays by this user
	Starred    string  `json:"starred"`    // when it was starred, empty if not
	UserRating int     `json:"userRating"` // 1-5, 0 = not rated
	Played     string  `json:"played"`     // OpenSubsonic: last play time
}

// Named is an OpenSubsonic {"name": ...} item, such as a genre.
type Named struct {
	Name string `json:"name"`
}

// Playlist is a server-side playlist.
type Playlist struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	SongCount int    `json:"songCount"`
}

// SearchResult holds search3 matches.
type SearchResult struct {
	Albums []Album
	Songs  []Song
}

type envelope struct {
	Response struct {
		Status string `json:"status"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		AlbumList2 struct {
			Album []Album `json:"album"`
		} `json:"albumList2"`
		Album struct {
			Song []Song `json:"song"`
		} `json:"album"`
		SearchResult3 struct {
			Album []Album `json:"album"`
			Song  []Song  `json:"song"`
		} `json:"searchResult3"`
		Playlists struct {
			Playlist []Playlist `json:"playlist"`
		} `json:"playlists"`
		Playlist struct {
			Entry []Song `json:"entry"`
		} `json:"playlist"`
		ScanStatus struct {
			Count int `json:"count"`
		} `json:"scanStatus"`
	} `json:"subsonic-response"`
}

func randomSalt() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func token(pass, salt string) string {
	sum := md5.Sum([]byte(pass + salt))
	return hex.EncodeToString(sum[:])
}

func (c *Client) signed(name string, extra url.Values, salt, tok string) string {
	q := url.Values{
		"u": {c.user}, "t": {tok}, "s": {salt},
		"v": {apiVersion}, "c": {"jbn-player"}, "f": {"json"},
	}
	for k, v := range extra {
		q[k] = v
	}
	return c.base + "/rest/" + name + ".view?" + q.Encode()
}

func (c *Client) endpoint(name string, extra url.Values) string {
	salt := randomSalt()
	return c.signed(name, extra, salt, token(c.pass, salt))
}

func (c *Client) call(ctx context.Context, name string, extra url.Values) (*envelope, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint(name, extra), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subsonic %s: HTTP %d", name, resp.StatusCode)
	}
	var env envelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("subsonic %s: decode: %w", name, err)
	}
	if env.Response.Status != "ok" {
		if e := env.Response.Error; e != nil {
			return nil, fmt.Errorf("subsonic %s: error %d: %s", name, e.Code, e.Message)
		}
		return nil, fmt.Errorf("subsonic %s: status %q", name, env.Response.Status)
	}
	return &env, nil
}

// Ping checks connectivity and credentials.
func (c *Client) Ping(ctx context.Context) error {
	_, err := c.call(ctx, "ping", nil)
	return err
}

// AlbumList returns albums by list type (random, newest, alphabeticalByName,
// recent, frequent), paginated.
func (c *Client) AlbumList(ctx context.Context, kind string, size, offset int) ([]Album, error) {
	env, err := c.call(ctx, "getAlbumList2", url.Values{
		"type": {kind}, "size": {strconv.Itoa(size)}, "offset": {strconv.Itoa(offset)},
	})
	if err != nil {
		return nil, err
	}
	return env.Response.AlbumList2.Album, nil
}

// RandomAlbums returns up to n random albums.
func (c *Client) RandomAlbums(ctx context.Context, n int) ([]Album, error) {
	return c.AlbumList(ctx, "random", n, 0)
}

// AlbumSongs returns the tracks of an album.
func (c *Client) AlbumSongs(ctx context.Context, albumID string) ([]Song, error) {
	env, err := c.call(ctx, "getAlbum", url.Values{"id": {albumID}})
	if err != nil {
		return nil, err
	}
	return env.Response.Album.Song, nil
}

// Search looks for albums and songs matching q.
func (c *Client) Search(ctx context.Context, q string) (SearchResult, error) {
	env, err := c.call(ctx, "search3", url.Values{
		"query": {q}, "artistCount": {"0"}, "albumCount": {"12"}, "songCount": {"25"},
	})
	if err != nil {
		return SearchResult{}, err
	}
	return SearchResult{Albums: env.Response.SearchResult3.Album, Songs: env.Response.SearchResult3.Song}, nil
}

// AllSongs pages through every song of the library. Navidrome returns all
// songs for an empty search3 query, which is the way sync clients list a library.
func (c *Client) AllSongs(ctx context.Context, offset, count int) ([]Song, error) {
	env, err := c.call(ctx, "search3", url.Values{
		"query": {""}, "artistCount": {"0"}, "albumCount": {"0"},
		"songCount": {strconv.Itoa(count)}, "songOffset": {strconv.Itoa(offset)},
	})
	if err != nil {
		return nil, err
	}
	return env.Response.SearchResult3.Song, nil
}

// Playlists lists the server's playlists.
func (c *Client) Playlists(ctx context.Context) ([]Playlist, error) {
	env, err := c.call(ctx, "getPlaylists", nil)
	if err != nil {
		return nil, err
	}
	return env.Response.Playlists.Playlist, nil
}

// PlaylistSongs returns the tracks of a playlist, in order.
func (c *Client) PlaylistSongs(ctx context.Context, id string) ([]Song, error) {
	env, err := c.call(ctx, "getPlaylist", url.Values{"id": {id}})
	if err != nil {
		return nil, err
	}
	return env.Response.Playlist.Entry, nil
}

// StreamURL returns a signed URL that streams the original file.
func (c *Client) StreamURL(songID string) string {
	return c.endpoint("stream", url.Values{"id": {songID}})
}

// CoverURL returns a stable URL for cover art (cache-friendly). Empty id gives "".
func (c *Client) CoverURL(coverID string, size int) string {
	if coverID == "" {
		return ""
	}
	return c.signed("getCoverArt", url.Values{"id": {coverID}, "size": {strconv.Itoa(size)}}, c.coverSalt, c.coverToken)
}

// downloader has no overall timeout (a whole album can take minutes); the
// caller's context cancels it, and a stalled server is cut by the header timeout.
var downloader = &http.Client{Transport: &http.Transport{
	Proxy:                 http.ProxyFromEnvironment,
	ResponseHeaderTimeout: 30 * time.Second,
	IdleConnTimeout:       30 * time.Second,
}}

// Download opens the original file of a song (the `download` endpoint, so no
// transcoding). The caller closes the body. ext is the file extension the
// server reported, lower case without the dot; it may be empty.
func (c *Client) Download(ctx context.Context, songID string) (body io.ReadCloser, ext string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.endpoint("download", url.Values{"id": {songID}}), nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := downloader.Do(req)
	if err != nil {
		return nil, "", err
	}
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		return nil, "", fmt.Errorf("subsonic download: HTTP %d", resp.StatusCode)
	}
	ctype, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if ctype == "application/json" || ctype == "text/xml" || ctype == "application/xml" || ctype == "text/plain" {
		// Subsonic answers errors with a normal 200 and an envelope instead of audio.
		defer resp.Body.Close()
		var env envelope
		if json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&env) == nil && env.Response.Error != nil {
			return nil, "", fmt.Errorf("subsonic download: error %d: %s", env.Response.Error.Code, env.Response.Error.Message)
		}
		return nil, "", fmt.Errorf("subsonic download: unexpected %s answer", ctype)
	}
	return resp.Body, fileExt(resp.Header.Get("Content-Disposition"), ctype), nil
}

// fileExt picks the extension from the attachment name, else from the MIME type.
func fileExt(disposition, ctype string) string {
	if _, p, err := mime.ParseMediaType(disposition); err == nil {
		if e := strings.ToLower(strings.TrimPrefix(path.Ext(p["filename"]), ".")); e != "" {
			return e
		}
	}
	switch ctype {
	case "audio/mpeg":
		return "mp3"
	case "audio/flac", "audio/x-flac":
		return "flac"
	case "audio/ogg", "application/ogg":
		return "ogg"
	case "audio/opus":
		return "opus"
	case "audio/mp4", "audio/x-m4a", "audio/aac":
		return "m4a"
	case "audio/wav", "audio/x-wav":
		return "wav"
	}
	return ""
}
