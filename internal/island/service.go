//go:build windows

package island

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"time"

	"jbnplayer/internal/library"
	"jbnplayer/internal/store"
	"jbnplayer/internal/subsonic"
)

// service holds the logic behind the buttons of the window. Every method
// returns "" on success or a human-readable error, which is what the page shows.
type service struct {
	mu      sync.Mutex
	path    string // servers.json
	ipcPath string // player endpoint file
	libDir  string // where library-<server>.json caches live

	appearancePath string // appearance.json (accent color, surface tone)

	syncMu    sync.Mutex // one library sync at a time
	lib       *library.Cache
	libServer string // server ID the in-memory cache belongs to

	cached    *subsonic.Client // active server's client, see client()
	cachedKey string
}

type serverView struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	User        string `json:"user"`
	HasPassword bool   `json:"hasPassword"`
	Active      bool   `json:"active"`
}

type serverInput struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	User     string `json:"user"`
	Password string `json:"password"`
}

// GetServers lists the servers; passwords never leave the Go side.
func (s *service) GetServers() []serverView {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := store.Load(s.path)
	if err != nil {
		return []serverView{}
	}
	active, _ := d.ActiveServer()
	out := make([]serverView, 0, len(d.Servers))
	for _, sv := range d.Servers {
		out = append(out, serverView{sv.ID, sv.Type, sv.Name, sv.URL, sv.User, sv.Password != "", sv.ID == active.ID})
	}
	return out
}

func normalize(in *serverInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.User = strings.TrimSpace(in.User)
	in.URL = strings.TrimRight(strings.TrimSpace(in.URL), "/")
	if in.Type == "" {
		in.Type = store.TypeSubsonic
	}
	if in.Type != store.TypeSubsonic {
		return errors.New("unsupported server type")
	}
	u, err := url.Parse(in.URL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errors.New("URL must start with http:// or https://")
	}
	if in.User == "" {
		return errors.New("username is required")
	}
	if in.Name == "" {
		in.Name = u.Host
	}
	return nil
}

// storedPassword finds the saved password of an existing server (empty if none).
func (s *service) storedPassword(id string) string {
	if id == "" {
		return ""
	}
	d, err := store.Load(s.path)
	if err != nil {
		return ""
	}
	for _, sv := range d.Servers {
		if sv.ID == id {
			return sv.Password
		}
	}
	return ""
}

// TestServer checks the connection and credentials without saving anything.
func (s *service) TestServer(in serverInput) string {
	if err := normalize(&in); err != nil {
		return err.Error()
	}
	s.mu.Lock()
	if in.Password == "" {
		in.Password = s.storedPassword(in.ID) // editing: blank means "unchanged"
	}
	s.mu.Unlock()
	if in.Password == "" {
		return "password is required"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := subsonic.New(in.URL, in.User, in.Password).Ping(ctx); err != nil {
		return err.Error()
	}
	return ""
}

// SaveServer adds a server, or updates it when in.ID is set.
func (s *service) SaveServer(in serverInput) string {
	if err := normalize(&in); err != nil {
		return err.Error()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := store.Load(s.path)
	if err != nil {
		return err.Error()
	}
	if in.ID == "" {
		if in.Password == "" {
			return "password is required"
		}
		in.ID = store.NewID()
		d.Servers = append(d.Servers, store.Server{ID: in.ID, Type: in.Type, Name: in.Name, URL: in.URL, User: in.User, Password: in.Password})
		if d.Active == "" || len(d.Servers) == 1 {
			d.Active = in.ID
		}
	} else {
		found := false
		for i := range d.Servers {
			if d.Servers[i].ID != in.ID {
				continue
			}
			found = true
			pw := in.Password
			if pw == "" {
				pw = d.Servers[i].Password // blank means unchanged
			}
			d.Servers[i] = store.Server{ID: in.ID, Type: in.Type, Name: in.Name, URL: in.URL, User: in.User, Password: pw}
		}
		if !found {
			return "server not found"
		}
	}
	if err := store.Save(s.path, d); err != nil {
		return err.Error()
	}
	return ""
}

// DeleteServer removes a server; if it was the active one, the first remaining becomes active.
func (s *service) DeleteServer(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := store.Load(s.path)
	if err != nil {
		return err.Error()
	}
	kept := d.Servers[:0]
	for _, sv := range d.Servers {
		if sv.ID != id {
			kept = append(kept, sv)
		}
	}
	d.Servers = kept
	if d.Active == id {
		d.Active = ""
		if len(kept) > 0 {
			d.Active = kept[0].ID
		}
	}
	if err := store.Save(s.path, d); err != nil {
		return err.Error()
	}
	return ""
}

// SetActive picks the server the player uses.
func (s *service) SetActive(id string) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := store.Load(s.path)
	if err != nil {
		return err.Error()
	}
	for _, sv := range d.Servers {
		if sv.ID == id {
			d.Active = id
			if err := store.Save(s.path, d); err != nil {
				return err.Error()
			}
			return ""
		}
	}
	return "server not found"
}
