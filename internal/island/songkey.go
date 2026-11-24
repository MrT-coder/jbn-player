//go:build windows

package island

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"jbnplayer/internal/songkey"
	"jbnplayer/internal/store"
	"jbnplayer/internal/subsonic"
)

// songkey: paste a YouTube link, songkey downloads it into the library. All
// HTTP calls happen here in Go; the token never reaches the page.

type songkeyView struct {
	URL        string `json:"url"`
	HasToken   bool   `json:"hasToken"`
	Configured bool   `json:"configured"`
	Error      string `json:"error"`
}

type songkeyInput struct {
	URL   string `json:"url"`
	Token string `json:"token"` // empty keeps the saved one
	Force bool   `json:"force"` // save even if the test call fails
}

type skJobView struct {
	ID     string `json:"id"`
	State  string `json:"state"`
	Title  string `json:"title"`
	Artist string `json:"artist"`
	Reason string `json:"reason"`
}

type skAddResult struct {
	Result  string     `json:"result"`
	Message string     `json:"message"`
	Job     *skJobView `json:"job,omitempty"`
	Error   string     `json:"error"`
}

type skJobsResult struct {
	Jobs      []skJobView `json:"jobs"`      // followed jobs: in progress, failed or duplicate
	Filed     []string    `json:"filed"`     // songs that reached the playlist in this call
	Refreshed bool        `json:"refreshed"` // the library playlist was re-read
	Error     string      `json:"error"`
}

func skView(d store.Data) songkeyView {
	return songkeyView{URL: d.Songkey.URL, HasToken: d.Songkey.Token != "", Configured: d.Songkey.Configured()}
}

// GetSongkey returns the songkey settings (never the token itself).
func (s *service) GetSongkey() songkeyView {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := store.Load(s.path)
	if err != nil {
		return songkeyView{Error: err.Error()}
	}
	return skView(d)
}

// SaveSongkey stores the songkey URL and token. An empty URL removes songkey.
// Unless forced, the settings are tried first, so a typo shows up right away.
func (s *service) SaveSongkey(in songkeyInput) songkeyView {
	in.URL = strings.TrimRight(strings.TrimSpace(in.URL), "/")
	in.Token = strings.TrimSpace(in.Token)
	if in.URL != "" {
		u, err := url.Parse(in.URL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return songkeyView{Error: "URL must start with http:// or https://"}
		}
	}
	s.mu.Lock()
	d, err := store.Load(s.path)
	s.mu.Unlock()
	if err != nil {
		return songkeyView{Error: err.Error()}
	}
	cfg := store.Songkey{URL: in.URL, Token: in.Token}
	// An empty token field keeps the saved token, but only for the same
	// server: a new host must be given its token explicitly.
	if cfg.Token == "" && in.URL != "" && sameOrigin(in.URL, d.Songkey.URL) {
		cfg.Token = d.Songkey.Token
	}
	if in.URL != "" && !in.Force {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := songkey.New(cfg.URL, cfg.Token).Status(ctx); err != nil {
			v := skView(d)
			v.Error = err.Error()
			return v
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if d, err = store.Load(s.path); err != nil { // re-read: servers may have changed meanwhile
		return songkeyView{Error: err.Error()}
	}
	d.Songkey = cfg
	if err := store.Save(s.path, d); err != nil {
		return songkeyView{Error: err.Error()}
	}
	return skView(d)
}

// sameOrigin reports whether two URLs share scheme and host.
func sameOrigin(a, b string) bool {
	ua, errA := url.Parse(a)
	ub, errB := url.Parse(b)
	return errA == nil && errB == nil && ua.Scheme == ub.Scheme && strings.EqualFold(ua.Host, ub.Host)
}

func (s *service) songkeyClient() (*songkey.Client, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, err := store.Load(s.path)
	if err != nil {
		return nil, err
	}
	if !d.Songkey.Configured() {
		return nil, errors.New("songkey is not set up")
	}
	return songkey.New(d.Songkey.URL, d.Songkey.Token), nil
}

func jobView(j songkey.Job) skJobView {
	return skJobView{ID: j.ID, State: j.State, Title: j.Title, Artist: j.Artist, Reason: j.Reason}
}

// SongkeyAdd sends a link to songkey. A song it starts working on is followed
// (see SongkeyJobs). "already have it" is an answer, not an error.
func (s *service) SongkeyAdd(link string) skAddResult {
	link = strings.TrimSpace(link)
	if link == "" {
		return skAddResult{Error: "paste a YouTube link first"}
	}
	c, err := s.songkeyClient()
	if err != nil {
		return skAddResult{Error: err.Error()}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	r, err := c.Add(ctx, link)
	if err != nil {
		return skAddResult{Error: err.Error()}
	}
	out := skAddResult{Result: r.Result, Message: r.Message}
	if r.Job != nil {
		v := jobView(*r.Job)
		out.Job = &v
		s.skJobsMu.Lock()
		jobs := s.loadSkJobs()
		known := false
		for i := range jobs {
			if jobs[i].ID == v.ID {
				jobs[i], known = v, true
			}
		}
		if !known {
			jobs = append(jobs, v)
		}
		s.saveSkJobs(jobs)
		s.skJobsMu.Unlock()
	}
	return out
}

// SongkeyJobs asks songkey about every followed job still in progress. A job
// that reached the playlist leaves the list (it is reported once in Filed) and
// the library playlist is re-read so the song shows up at once. Failed and
// duplicate jobs stay, with their reason, until dismissed.
func (s *service) SongkeyJobs() skJobsResult {
	s.skJobsMu.Lock()
	defer s.skJobsMu.Unlock()
	jobs := s.loadSkJobs()
	out := skJobsResult{Jobs: []skJobView{}, Filed: []string{}}
	pending := false
	for _, j := range jobs {
		pending = pending || !(songkey.Job{State: j.State}).Terminal()
	}
	if !pending {
		out.Jobs = append(out.Jobs, jobs...)
		return out
	}
	c, err := s.songkeyClient()
	if err != nil {
		out.Jobs, out.Error = append(out.Jobs, jobs...), err.Error()
		return out
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	for _, j := range jobs {
		if (songkey.Job{State: j.State}).Terminal() {
			out.Jobs = append(out.Jobs, j)
			continue
		}
		fresh, err := c.Job(ctx, j.ID)
		switch {
		case songkey.IsCode(err, "job_not_found"):
			j.State, j.Reason = songkey.StateFailed, "songkey no longer knows this job"
		case err != nil:
			out.Error = err.Error() // keep it as it was, try again on the next call
		case fresh.State == songkey.StateFiled:
			out.Filed = append(out.Filed, displayName(fresh))
			continue
		default:
			j = jobView(fresh)
		}
		out.Jobs = append(out.Jobs, j)
	}
	s.saveSkJobs(out.Jobs)
	if len(out.Filed) > 0 {
		if err := s.refreshSongkeyPlaylist(ctx, c); err != nil && out.Error == "" {
			out.Error = "added, but the playlist could not be refreshed: " + err.Error()
		} else if err == nil {
			out.Refreshed = true
		}
	}
	return out
}

// SongkeyDismiss stops showing a finished (failed or duplicate) job.
func (s *service) SongkeyDismiss(id string) skJobsResult {
	s.skJobsMu.Lock()
	defer s.skJobsMu.Unlock()
	out := skJobsResult{Jobs: []skJobView{}, Filed: []string{}}
	for _, j := range s.loadSkJobs() {
		if j.ID != id {
			out.Jobs = append(out.Jobs, j)
		}
	}
	s.saveSkJobs(out.Jobs)
	return out
}

func displayName(j songkey.Job) string {
	if j.Artist == "" {
		return j.Title
	}
	return j.Artist + " - " + j.Title
}

// refreshSongkeyPlaylist re-reads songkey's library playlist into the local
// copy. Without a local copy there is nothing to refresh.
func (s *service) refreshSongkeyPlaylist(ctx context.Context, c *songkey.Client) error {
	st, err := c.Status(ctx)
	if err != nil {
		return err
	}
	if st.Playlist.ID == "" {
		return errors.New("songkey did not name its playlist")
	}
	lib, err := s.loadLib()
	if err != nil || lib == nil {
		return err
	}
	s.mu.Lock()
	sv, err := s.activeServer()
	s.mu.Unlock()
	if err != nil {
		return err
	}
	// The server is asked without holding syncMu (lists and search keep
	// working); RefreshPlaylist never changes lib, it builds a new copy.
	n, err := lib.RefreshPlaylist(ctx, subsonic.New(sv.URL, sv.User, sv.Password), st.Playlist.ID, st.Playlist.Name)
	if err != nil {
		return err
	}
	s.syncMu.Lock()
	defer s.syncMu.Unlock()
	if s.lib != lib || s.libServer != sv.ID { // a sync or a server switch happened meanwhile: theirs wins
		return nil
	}
	if err := n.Save(s.libPath(sv.ID)); err != nil {
		return err
	}
	s.lib = n
	return nil
}

// The followed jobs live in a small file, so they survive the window closing.
// Caller holds skJobsMu.
func (s *service) skJobsPath() string { return filepath.Join(s.libDir, "songkey-jobs.json") }

func (s *service) loadSkJobs() []skJobView {
	var jobs []skJobView
	if raw, err := os.ReadFile(s.skJobsPath()); err == nil {
		_ = json.Unmarshal(raw, &jobs) // a corrupt file just means nothing is followed
	}
	return jobs
}

func (s *service) saveSkJobs(jobs []skJobView) {
	if len(jobs) == 0 {
		_ = os.Remove(s.skJobsPath())
		return
	}
	raw, _ := json.Marshal(jobs)
	_ = os.MkdirAll(s.libDir, 0o700)
	tmp := s.skJobsPath() + ".tmp"
	if os.WriteFile(tmp, raw, 0o600) == nil {
		_ = os.Rename(tmp, s.skJobsPath())
	}
}
