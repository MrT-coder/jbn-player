//go:build windows

// Package store persists the configured servers in %APPDATA%\jbn-player\servers.json.
// Passwords are DPAPI-encrypted on disk and plaintext only in memory.
package store

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// TypeSubsonic is the only server type supported so far (Navidrome, Airsonic...).
const TypeSubsonic = "subsonic"

// Server is one configured music server.
type Server struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Name     string `json:"name"`
	URL      string `json:"url"`
	User     string `json:"user"`
	Password string `json:"-"` // plaintext, memory only
}

// Data is the whole persisted state.
type Data struct {
	Servers []Server `json:"servers"`
	Active  string   `json:"active"` // ID of the server in use
	Songkey Songkey  `json:"-"`
}

// Songkey is the optional songkey service (adds YouTube links to the library).
type Songkey struct {
	URL   string
	Token string // plaintext, memory only
}

// Configured reports whether songkey can be used.
func (s Songkey) Configured() bool { return s.URL != "" }

// ActiveServer returns the selected server, or the first one as a fallback.
func (d Data) ActiveServer() (Server, bool) {
	for _, s := range d.Servers {
		if s.ID == d.Active {
			return s, true
		}
	}
	if len(d.Servers) > 0 {
		return d.Servers[0], true
	}
	return Server{}, false
}

// DefaultPath is where the servers file lives.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jbn-player", "servers.json"), nil
}

// NewID returns a random server ID.
func NewID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// on-disk shape: same as Server plus the encrypted password.
type diskServer struct {
	Server
	Password string `json:"password,omitempty"` // base64(DPAPI(password))
}

type diskSongkey struct {
	URL   string `json:"url"`
	Token string `json:"token,omitempty"` // base64(DPAPI(token))
}

type diskData struct {
	Servers []diskServer `json:"servers"`
	Active  string       `json:"active"`
	Songkey *diskSongkey `json:"songkey,omitempty"`
}

// seal and unseal turn a secret into base64(DPAPI(secret)) and back.
func seal(secret string) (string, error) {
	enc, err := protect([]byte(secret))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(enc), nil
}

func unseal(stored string) (string, error) {
	enc, err := base64.StdEncoding.DecodeString(stored)
	if err != nil {
		return "", fmt.Errorf("bad encoding: %w", err)
	}
	plain, err := unprotect(enc)
	if err != nil {
		return "", fmt.Errorf("cannot decrypt (saved by another Windows user or machine?): %w", err)
	}
	return string(plain), nil
}

// Load reads and decrypts the file. A missing file is an empty Data, not an error.
func Load(path string) (Data, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Data{}, nil
	}
	if err != nil {
		return Data{}, err
	}
	var dd diskData
	if err := json.Unmarshal(raw, &dd); err != nil {
		return Data{}, fmt.Errorf("parse %s: %w", path, err)
	}
	d := Data{Active: dd.Active}
	for _, ds := range dd.Servers {
		s := ds.Server
		if ds.Password != "" {
			plain, err := unseal(ds.Password)
			if err != nil {
				return Data{}, fmt.Errorf("server %q password: %w", s.Name, err)
			}
			s.Password = plain
		}
		d.Servers = append(d.Servers, s)
	}
	if sk := dd.Songkey; sk != nil {
		d.Songkey.URL = sk.URL
		if sk.Token != "" {
			plain, err := unseal(sk.Token)
			if err != nil {
				return Data{}, fmt.Errorf("songkey token: %w", err)
			}
			d.Songkey.Token = plain
		}
	}
	return d, nil
}

// Save encrypts the passwords and writes the file atomically (temp + rename).
func Save(path string, d Data) error {
	dd := diskData{Active: d.Active}
	for _, s := range d.Servers {
		ds := diskServer{Server: s}
		if s.Password != "" {
			enc, err := seal(s.Password)
			if err != nil {
				return fmt.Errorf("encrypt password: %w", err)
			}
			ds.Password = enc
		}
		dd.Servers = append(dd.Servers, ds)
	}
	if d.Songkey.Configured() {
		dd.Songkey = &diskSongkey{URL: d.Songkey.URL}
		if d.Songkey.Token != "" {
			enc, err := seal(d.Songkey.Token)
			if err != nil {
				return fmt.Errorf("encrypt songkey token: %w", err)
			}
			dd.Songkey.Token = enc
		}
	}
	raw, err := json.MarshalIndent(dd, "", "  ")
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
