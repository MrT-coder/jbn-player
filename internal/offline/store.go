// Package offline keeps local copies of songs so they play without the server.
//
// Layout of one server's folder:
//
//	index.json     which songs are stored and how big they are
//	<id>.<ext>     the original audio file, as the server sent it
//	covers/        album covers of the stored songs, for offline mode
//
// Nothing here ever writes to or deletes anything on the server: removing an
// offline copy only deletes the local file.
package offline

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
)

const indexName = "index.json"

var (
	safeID  = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)
	safeExt = regexp.MustCompile(`^[a-z0-9]{1,5}$`)
)

// Dir is the folder of one server's offline copies under the root.
func Dir(root, serverID string) string { return filepath.Join(root, serverID) }

// CoversDir holds the album covers of the stored songs (<cover id>.jpg), so
// the window can show them while the server is off. The window fills it.
func CoversDir(dir string) string { return filepath.Join(dir, "covers") }

type entry struct {
	File string `json:"file"`
	Size int64  `json:"size"`
}

type indexFile struct {
	Songs map[string]entry `json:"songs"`
}

// Store is the set of songs stored in one folder. Safe for concurrent use.
type Store struct {
	dir string
	mu  sync.Mutex
	idx map[string]entry
}

// Open loads (or creates) the folder. Entries whose file is gone are dropped
// and half-written downloads from an earlier run are removed.
func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	s := &Store{dir: dir, idx: map[string]entry{}}
	raw, err := os.ReadFile(filepath.Join(dir, indexName))
	if err == nil {
		var f indexFile
		if json.Unmarshal(raw, &f) == nil { // a corrupt index starts empty
			for id, e := range f.Songs {
				if _, err := os.Stat(filepath.Join(dir, e.File)); err == nil && safeID.MatchString(id) {
					s.idx[id] = e
				}
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if parts, _ := filepath.Glob(filepath.Join(dir, "*.part")); len(parts) > 0 {
		for _, p := range parts {
			_ = os.Remove(p)
		}
	}
	return s, nil
}

// IDs reads the IDs of the stored songs straight from the index file, without
// opening the store. The island window uses it (read-only) to mark songs.
func IDs(dir string) ([]string, error) {
	raw, err := os.ReadFile(filepath.Join(dir, indexName))
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var f indexFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return []string{}, nil
	}
	out := make([]string, 0, len(f.Songs))
	for id := range f.Songs {
		out = append(out, id)
	}
	sort.Strings(out)
	return out, nil
}

// Path returns the local file of a song, if it is stored.
func (s *Store) Path(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.idx[id]
	if !ok {
		return "", false
	}
	return filepath.Join(s.dir, e.File), true
}

// Has reports whether the song is stored.
func (s *Store) Has(id string) bool { _, ok := s.Path(id); return ok }

// IDs lists the stored songs, sorted.
func (s *Store) IDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.idx))
	for id := range s.idx {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

// Totals returns how many songs are stored and their size in bytes.
func (s *Store) Totals() (count int, bytes int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range s.idx {
		bytes += e.Size
	}
	return len(s.idx), bytes
}

// Save writes the song to disk. The file only appears (and is only indexed)
// once it has been fully received, so a cut connection leaves nothing behind.
func (s *Store) Save(id, ext string, r io.Reader) (int64, error) {
	if !safeID.MatchString(id) {
		return 0, fmt.Errorf("unsafe song id %q", id)
	}
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))
	if !safeExt.MatchString(ext) {
		ext = "audio"
	}
	name := id + "." + ext
	tmp := filepath.Join(s.dir, name+".part")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n == 0 {
		err = errors.New("empty download")
	}
	if err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	final := filepath.Join(s.dir, name)
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return 0, err
	}
	if old, ok := s.idx[id]; ok && old.File != name {
		_ = os.Remove(filepath.Join(s.dir, old.File))
	}
	s.idx[id] = entry{File: name, Size: n}
	if err := s.saveLocked(); err != nil {
		delete(s.idx, id)
		_ = os.Remove(final)
		return 0, err
	}
	return n, nil
}

// Remove deletes the local copy of a song (never anything on the server).
func (s *Store) Remove(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.idx[id]
	if !ok {
		return nil
	}
	if err := os.Remove(filepath.Join(s.dir, e.File)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	delete(s.idx, id)
	return s.saveLocked()
}

// Clear deletes every local copy, and the covers kept for them.
func (s *Store) Clear() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	first := os.RemoveAll(CoversDir(s.dir))
	for id, e := range s.idx {
		if err := os.Remove(filepath.Join(s.dir, e.File)); err != nil && !errors.Is(err, os.ErrNotExist) && first == nil {
			first = err
			continue
		}
		delete(s.idx, id)
	}
	if err := s.saveLocked(); err != nil && first == nil {
		first = err
	}
	return first
}

func (s *Store) saveLocked() error {
	raw, err := json.Marshal(indexFile{Songs: s.idx})
	if err != nil {
		return err
	}
	tmp := filepath.Join(s.dir, indexName+".tmp")
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(s.dir, indexName))
}
