//go:build windows

package island

import (
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"jbnplayer/internal/offline"
)

// Offline covers. While the window is open and the server answers, the covers
// of the stored songs that are missing are fetched in the background, one per
// cover ID (usually one per album), and kept in the offline folder. While the
// server is off, the window shows them through a loopback HTTP server that
// lives only as long as this popup process. Nothing runs in the tray process.

const coverStoreSize = 300 // px; big enough for lists, albums and Now playing

var safeCover = regexp.MustCompile(`^[A-Za-z0-9_.-]{1,128}$`)

var coverHTTP = &http.Client{Timeout: 15 * time.Second}

func coverFile(dir, coverID string) (string, bool) {
	if !safeCover.MatchString(coverID) || strings.Trim(coverID, ".") == "" {
		return "", false
	}
	return filepath.Join(offline.CoversDir(dir), coverID+".jpg"), true
}

// offlineDir is the active server's offline folder.
func (s *service) offlineDir() (string, error) {
	s.mu.Lock()
	sv, err := s.activeServer()
	s.mu.Unlock()
	if err != nil {
		return "", err
	}
	return offline.Dir(offline.LoadRoot(), sv.ID), nil
}

// startBackfill runs backfillCovers in the background, one run at a time.
func (s *service) startBackfill() {
	if !s.backfilling.CompareAndSwap(false, true) {
		return
	}
	go func() {
		defer s.backfilling.Store(false)
		s.backfillCovers()
	}()
}

// backfillCovers fetches the missing covers of the stored songs. It stops at
// the first failure: the next status check (30 s) tries again.
func (s *service) backfillCovers() {
	c, err := s.client()
	if err != nil {
		return
	}
	lib, err := s.loadLib()
	if err != nil || lib == nil {
		return
	}
	dir, err := s.offlineDir()
	if err != nil {
		return
	}
	ids, err := offline.IDs(dir)
	if err != nil || len(ids) == 0 {
		return
	}
	stored := make(map[string]bool, len(ids))
	for _, id := range ids {
		stored[id] = true
	}
	seen := map[string]bool{}
	for _, x := range lib.Songs {
		if !stored[x.ID] || x.CoverArt == "" || seen[x.CoverArt] {
			continue
		}
		seen[x.CoverArt] = true
		path, ok := coverFile(dir, x.CoverArt)
		if !ok {
			continue
		}
		if _, err := os.Stat(path); err == nil {
			continue
		}
		if s.offline.Load() || saveCover(c.CoverURL(x.CoverArt, coverStoreSize), path) != nil {
			return
		}
	}
}

func saveCover(url, path string) error {
	ctx, cancel := context.WithTimeout(context.Background(), coverHTTP.Timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	res, err := coverHTTP.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &os.PathError{Op: "get cover", Path: path, Err: os.ErrInvalid}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, io.LimitReader(res.Body, 5<<20))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, path)
}

// localCoverURL returns a loopback URL for a stored cover, or "" if there is none.
func (s *service) localCoverURL(coverID string) string {
	dir, err := s.offlineDir()
	if err != nil {
		return ""
	}
	path, ok := coverFile(dir, coverID)
	if !ok {
		return ""
	}
	if _, err := os.Stat(path); err != nil {
		return ""
	}
	base := s.coverServer()
	if base == "" {
		return ""
	}
	return base + coverID
}

// coverServer starts the loopback cover server on first use and returns its
// base URL ("" if it cannot listen). It only serves files of the covers folder.
func (s *service) coverServer() string {
	s.coverMu.Lock()
	defer s.coverMu.Unlock()
	if s.coverBase != "" {
		return s.coverBase
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return ""
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/cover/", func(w http.ResponseWriter, r *http.Request) {
		dir, err := s.offlineDir()
		if err != nil {
			http.NotFound(w, r)
			return
		}
		path, ok := coverFile(dir, strings.TrimPrefix(r.URL.Path, "/cover/"))
		if !ok {
			http.NotFound(w, r)
			return
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", http.DetectContentType(raw))
		w.Header().Set("Cache-Control", "max-age=3600")
		_, _ = w.Write(raw)
	})
	s.coverSrv = &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() { _ = s.coverSrv.Serve(ln) }()
	s.coverBase = "http://" + ln.Addr().String() + "/cover/"
	return s.coverBase
}

// closeCovers stops the loopback server (tests; the popup process just exits).
func (s *service) closeCovers() {
	s.coverMu.Lock()
	defer s.coverMu.Unlock()
	if s.coverSrv != nil {
		_ = s.coverSrv.Close()
		s.coverSrv, s.coverBase = nil, ""
	}
}

// coverURL is the cover to show: from the server while online, from the
// offline folder while offline ("" shows the placeholder).
func (s *service) coverURL(coverID string, size int) string {
	if coverID == "" {
		return ""
	}
	if s.offline.Load() {
		return s.localCoverURL(coverID)
	}
	c, err := s.client()
	if err != nil {
		return ""
	}
	return c.CoverURL(coverID, size)
}
