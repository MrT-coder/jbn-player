//go:build windows

// Package ipc lets the island window (a separate process) control the player
// that lives in the tray process. It is plain HTTP on 127.0.0.1 with a random
// port; the port and a random bearer token go into a file only the current
// Windows user can read. Browsers cannot reach it: they cannot send the token
// header cross-origin and the Host header is checked.
package ipc

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"jbnplayer/internal/offline"
	"jbnplayer/internal/player"
	"jbnplayer/internal/settings"
)

const (
	maxTracks    = 20000    // tracks per request
	maxBody      = 16 << 20 // bytes
	maxQueuePage = 500
)

// Controller is what the tray process exposes (implemented by *player.Player).
type Controller interface {
	State() player.State
	Queue(from, n int) player.QueuePage
	TogglePause() error
	Next()
	Prev()
	Play(ts []player.Track, start int) error
	Enqueue(ts []player.Track) error
	PlayNext(ts []player.Track) error
	PlayNow(ts []player.Track, start int) error
	Seek(pos float64) error
	Jump(i int) error
	Remove(i int) error
	Move(from, to int) error
	Audio() player.AudioInfo
	SetVolume(level int) error
	SetDevice(name string) error
	SetEQ(e settings.EQ) error
	ClearUpcoming()
	SetShuffle(on bool)
}

// Offline is what the tray process exposes for offline copies. A Controller
// that also implements it gets the /offline endpoints; others answer an error.
type Offline interface {
	OfflineStatus() offline.Status
	OfflineAdd(items []offline.Item) error
	OfflineRemove(ids []string) error
	OfflineClear() error
	OfflineCancel()                 // stop the downloads, keep what is stored
	OfflineSetDir(dir string) error // "" = default folder
}

// Request is the JSON body of the mutating calls.
type Request struct {
	Tracks   []player.Track `json:"tracks,omitempty"`
	Start    int            `json:"start,omitempty"`
	Index    int            `json:"index,omitempty"`
	On       bool           `json:"on,omitempty"`
	From     int            `json:"from,omitempty"`  // move: current position
	To       int            `json:"to,omitempty"`    // move: new position
	Level    int            `json:"level,omitempty"` // volume 0-100
	Name     string         `json:"name,omitempty"`  // audio device
	EQ       *settings.EQ   `json:"eq,omitempty"`
	Items    []offline.Item `json:"items,omitempty"`    // offline: songs to download
	IDs      []string       `json:"ids,omitempty"`      // offline: songs to delete locally
	Position float64        `json:"position,omitempty"` // seek: seconds into the current track
}

type endpointFile struct {
	Port  int    `json:"port"`
	Token string `json:"token"`
}

// DefaultPath is where the endpoint file lives.
func DefaultPath() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jbn-player", "ipc.json"), nil
}

// Serve starts the control server and writes the endpoint file. Call the
// returned function to stop it and remove the file.
func Serve(path string, c Controller) (stop func(), err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	tokBytes := make([]byte, 16)
	_, _ = rand.Read(tokBytes)
	token := hex.EncodeToString(tokBytes)
	host := ln.Addr().String()

	mux := http.NewServeMux()
	reply := func(w http.ResponseWriter, err error) {
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(c.State())
	}
	mux.HandleFunc("GET /state", func(w http.ResponseWriter, r *http.Request) { reply(w, nil) })
	mux.HandleFunc("GET /queue", func(w http.ResponseWriter, r *http.Request) {
		from, _ := strconv.Atoi(r.URL.Query().Get("from"))
		n, _ := strconv.Atoi(r.URL.Query().Get("n"))
		if n <= 0 || n > maxQueuePage {
			n = maxQueuePage
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(c.Queue(from, n))
	})

	post := func(op string, do func(Request) error) {
		mux.HandleFunc("POST /"+op, func(w http.ResponseWriter, r *http.Request) {
			var req Request
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
			if r.ContentLength != 0 {
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					reply(w, fmt.Errorf("bad request: %w", err))
					return
				}
			}
			if len(req.Tracks) > maxTracks {
				reply(w, errors.New("too many tracks"))
				return
			}
			reply(w, do(req))
		})
	}
	replyAudio := func(w http.ResponseWriter, err error) {
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(c.Audio())
	}
	mux.HandleFunc("GET /audio", func(w http.ResponseWriter, r *http.Request) { replyAudio(w, nil) })
	postAudio := func(op string, do func(Request) error) {
		mux.HandleFunc("POST /"+op, func(w http.ResponseWriter, r *http.Request) {
			var req Request
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			if r.ContentLength != 0 {
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					replyAudio(w, fmt.Errorf("bad request: %w", err))
					return
				}
			}
			replyAudio(w, do(req))
		})
	}
	postAudio("volume", func(r Request) error { return c.SetVolume(r.Level) })
	postAudio("device", func(r Request) error { return c.SetDevice(r.Name) })
	postAudio("eq", func(r Request) error {
		if r.EQ == nil {
			return errors.New("missing eq")
		}
		return c.SetEQ(*r.EQ)
	})

	post("move", func(r Request) error { return c.Move(r.From, r.To) })
	post("toggle", func(Request) error { return c.TogglePause() })
	post("next", func(Request) error { c.Next(); return nil })
	post("prev", func(Request) error { c.Prev(); return nil })
	post("clear", func(Request) error { c.ClearUpcoming(); return nil })
	post("shuffle", func(r Request) error { c.SetShuffle(r.On); return nil })
	post("jump", func(r Request) error { return c.Jump(r.Index) })
	post("remove", func(r Request) error { return c.Remove(r.Index) })
	post("play", func(r Request) error { return c.Play(r.Tracks, r.Start) })
	post("enqueue", func(r Request) error { return c.Enqueue(r.Tracks) })
	post("playnext", func(r Request) error { return c.PlayNext(r.Tracks) })
	post("playnow", func(r Request) error { return c.PlayNow(r.Tracks, r.Start) })
	post("seek", func(r Request) error { return c.Seek(r.Position) })

	// Offline copies: the downloads run in this (long-lived) process, because
	// the window that asks for them is destroyed when it closes.
	replyOffline := func(w http.ResponseWriter, err error) {
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
			return
		}
		_ = json.NewEncoder(w).Encode(c.(Offline).OfflineStatus())
	}
	if _, ok := c.(Offline); !ok {
		unsupported := func(w http.ResponseWriter, r *http.Request) {
			replyAudio(w, errors.New("offline copies are not available"))
		}
		mux.HandleFunc("GET /offline", unsupported)
		mux.HandleFunc("POST /offline/{op}", unsupported)
	} else {
		o := c.(Offline)
		mux.HandleFunc("GET /offline", func(w http.ResponseWriter, r *http.Request) { replyOffline(w, nil) })
		postOffline := func(op string, do func(Request) error) {
			mux.HandleFunc("POST /offline/"+op, func(w http.ResponseWriter, r *http.Request) {
				var req Request
				r.Body = http.MaxBytesReader(w, r.Body, maxBody)
				if r.ContentLength != 0 {
					if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
						replyOffline(w, fmt.Errorf("bad request: %w", err))
						return
					}
				}
				if len(req.Items) > maxTracks || len(req.IDs) > maxTracks {
					replyOffline(w, errors.New("too many songs"))
					return
				}
				replyOffline(w, do(req))
			})
		}
		postOffline("add", func(r Request) error { return o.OfflineAdd(r.Items) })
		postOffline("remove", func(r Request) error { return o.OfflineRemove(r.IDs) })
		postOffline("clear", func(Request) error { return o.OfflineClear() })
		postOffline("cancel", func(Request) error { o.OfflineCancel(); return nil })
		postOffline("setdir", func(r Request) error { return o.OfflineSetDir(r.Name) })
	}

	auth := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if r.Host != host || subtle.ConstantTimeCompare([]byte(got), []byte(token)) != 1 {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		_ = ln.Close()
		return nil, err
	}
	raw, _ := json.Marshal(endpointFile{Port: ln.Addr().(*net.TCPAddr).Port, Token: token})
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		_ = ln.Close()
		return nil, err
	}
	srv := &http.Server{Handler: auth, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = srv.Serve(ln) }()
	return func() {
		_ = srv.Close()
		_ = os.Remove(path)
	}, nil
}

// Client calls the control server from the island process.
type Client struct {
	base, host, token string
	http              *http.Client
}

// Dial reads the endpoint file. It fails when the tray process is not running.
func Dial(path string) (*Client, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, errors.New("player is not running")
	}
	var ep endpointFile
	if err := json.Unmarshal(raw, &ep); err != nil || ep.Port == 0 {
		return nil, errors.New("player endpoint file is corrupt")
	}
	host := fmt.Sprintf("127.0.0.1:%d", ep.Port)
	return &Client{base: "http://" + host, host: host, token: ep.Token, http: &http.Client{Timeout: 10 * time.Second}}, nil
}

func (c *Client) do(method, op string, body *Request) (player.State, error) {
	var rd *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	req, err := http.NewRequest(method, c.base+"/"+op, rd)
	if err != nil {
		return player.State{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return player.State{}, errors.New("player is not running")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return player.State{}, errors.New(e.Error)
	}
	var s player.State
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return player.State{}, err
	}
	return s, nil
}

// State fetches the current player snapshot.
func (c *Client) State() (player.State, error) { return c.do(http.MethodGet, "state", nil) }

// Queue fetches a slice of the queue (n is capped by the server).
func (c *Client) Queue(from, n int) (player.QueuePage, error) {
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf("%s/queue?from=%d&n=%d", c.base, from, n), nil)
	if err != nil {
		return player.QueuePage{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return player.QueuePage{}, errors.New("player is not running")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return player.QueuePage{}, errors.New(resp.Status)
	}
	var page player.QueuePage
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return player.QueuePage{}, err
	}
	return page, nil
}

// Audio fetches the audio settings.
func (c *Client) Audio() (player.AudioInfo, error) { return c.audio(http.MethodGet, "audio", nil) }

// DoAudio changes an audio setting (volume, device, eq) and returns the new settings.
func (c *Client) DoAudio(op string, r Request) (player.AudioInfo, error) {
	return c.audio(http.MethodPost, op, &r)
}

func (c *Client) audio(method, op string, body *Request) (player.AudioInfo, error) {
	var rd *strings.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	} else {
		rd = strings.NewReader("")
	}
	req, err := http.NewRequest(method, c.base+"/"+op, rd)
	if err != nil {
		return player.AudioInfo{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return player.AudioInfo{}, errors.New("player is not running")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return player.AudioInfo{}, errors.New(e.Error)
	}
	var a player.AudioInfo
	if err := json.NewDecoder(resp.Body).Decode(&a); err != nil {
		return player.AudioInfo{}, err
	}
	return a, nil
}

// Do runs a mutating op (toggle, next, prev, clear, shuffle, jump, remove,
// play, enqueue, playnext, playnow, seek) and returns the new state.
func (c *Client) Do(op string, r Request) (player.State, error) {
	return c.do(http.MethodPost, op, &r)
}

// Offline fetches the offline status (songs stored, downloads pending).
func (c *Client) Offline() (offline.Status, error) { return c.offline(http.MethodGet, "offline", nil) }

// OfflineDo runs an offline op (add, remove, clear, cancel, setdir) and returns the new status.
func (c *Client) OfflineDo(op string, r Request) (offline.Status, error) {
	return c.offline(http.MethodPost, "offline/"+op, &r)
}

func (c *Client) offline(method, op string, body *Request) (offline.Status, error) {
	rd := strings.NewReader("")
	if body != nil {
		b, _ := json.Marshal(body)
		rd = strings.NewReader(string(b))
	}
	req, err := http.NewRequest(method, c.base+"/"+op, rd)
	if err != nil {
		return offline.Status{}, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return offline.Status{}, errors.New("player is not running")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		if e.Error == "" {
			e.Error = resp.Status
		}
		return offline.Status{}, errors.New(e.Error)
	}
	var s offline.Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return offline.Status{}, err
	}
	return s, nil
}
