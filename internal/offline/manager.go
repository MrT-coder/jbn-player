package offline

import (
	"context"
	"io"
	"sync"
)

// Fetcher opens the original file of a song on the server. ext is the file
// extension the server reported (may be empty).
type Fetcher func(ctx context.Context, id string) (rc io.ReadCloser, ext string, err error)

// Item is a song to download.
type Item struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Status is a snapshot for the window.
type Status struct {
	Count     int    `json:"count"`     // songs stored
	Bytes     int64  `json:"bytes"`     // their size
	Pending   int    `json:"pending"`   // queued plus the one in progress
	Current   string `json:"current"`   // title being downloaded
	Failed    int    `json:"failed"`    // downloads that failed since the last Bind or Retry
	LastError string `json:"lastError"` // message of the last failure
	Dir       string `json:"dir"`       // offline root in use (filled in by the player)
	CustomDir bool   `json:"customDir"` // false while the default folder is in use
}

// maxConsecutiveFailures stops a batch whose downloads keep failing (server off,
// wrong password): fetching thousands of songs one by one to fail each is pointless.
const maxConsecutiveFailures = 5

// Manager downloads songs one at a time in the background. The worker goroutine
// exists only while there is something to download, so an idle player costs nothing.
type Manager struct {
	mu      sync.Mutex
	store   *Store
	fetch   Fetcher
	gen     int // bumped by Bind: work of an older generation is dropped
	cancel  context.CancelFunc
	queue   []Item
	queued  map[string]bool
	running bool
	current string
	failed  int
	lastErr string
}

// NewManager returns an idle manager; call Bind before Add.
func NewManager() *Manager { return &Manager{queued: map[string]bool{}} }

// Bind points the manager at a server's folder and downloader, dropping any
// pending work of the previous one.
func (m *Manager) Bind(st *Store, f Fetcher) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	m.gen++
	m.store, m.fetch = st, f
	m.queue, m.queued = nil, map[string]bool{}
	m.failed, m.lastErr, m.current = 0, "", ""
	m.running = false // the old worker sees the new generation and exits
}

// Add queues the songs that are not stored or queued yet.
func (m *Manager) Add(items []Item) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil || m.fetch == nil {
		return
	}
	for _, it := range items {
		if it.ID == "" || m.queued[it.ID] || m.store.Has(it.ID) {
			continue
		}
		m.queue = append(m.queue, it)
		m.queued[it.ID] = true
	}
	if len(m.queue) > 0 && !m.running {
		m.running = true
		m.failed, m.lastErr = 0, "" // a new batch starts with a clean slate
		go m.work(m.gen)
	}
}

func (m *Manager) work(gen int) {
	consecutive := 0
	for {
		m.mu.Lock()
		if gen != m.gen || len(m.queue) == 0 {
			if gen == m.gen {
				m.running, m.current = false, ""
			}
			m.mu.Unlock()
			return
		}
		it := m.queue[0]
		m.queue = m.queue[1:]
		m.current = it.Title
		st, fetch := m.store, m.fetch
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		m.mu.Unlock()

		err := download(ctx, st, fetch, it.ID)
		cancelled := ctx.Err() != nil // Cancel or Bind, not a real failure
		cancel()

		m.mu.Lock()
		if gen == m.gen {
			delete(m.queued, it.ID)
			switch {
			case err == nil:
				consecutive = 0
			case !cancelled:
				m.failed++
				m.lastErr = err.Error()
				if consecutive++; consecutive >= maxConsecutiveFailures {
					m.queue, m.queued = nil, map[string]bool{} // give up on the rest of the batch
				}
			}
		}
		m.mu.Unlock()
	}
}

func download(ctx context.Context, st *Store, fetch Fetcher, id string) error {
	rc, ext, err := fetch(ctx, id)
	if err != nil {
		return err
	}
	defer rc.Close()
	_, err = st.Save(id, ext, rc)
	return err
}

// Remove deletes the local copies of the given songs and cancels them if queued.
func (m *Manager) Remove(ids []string) error {
	m.mu.Lock()
	st := m.store
	drop := map[string]bool{}
	for _, id := range ids {
		drop[id] = true
	}
	kept := m.queue[:0]
	for _, it := range m.queue {
		if drop[it.ID] {
			delete(m.queued, it.ID)
			continue
		}
		kept = append(kept, it)
	}
	m.queue = kept
	m.mu.Unlock()
	if st == nil {
		return nil
	}
	var first error
	for _, id := range ids {
		if err := st.Remove(id); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// Clear stops everything and deletes every local copy of the bound server.
func (m *Manager) Clear() error {
	m.mu.Lock()
	st := m.store
	if m.cancel != nil {
		m.cancel()
	}
	m.queue, m.queued = nil, map[string]bool{}
	m.mu.Unlock()
	if st == nil {
		return nil
	}
	return st.Clear()
}

// Status reports what is stored and what is still being downloaded.
func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	s := Status{Pending: len(m.queue), Current: m.current, Failed: m.failed, LastError: m.lastErr}
	if m.running {
		s.Pending++
	}
	if m.store != nil {
		s.Count, s.Bytes = m.store.Totals()
	}
	return s
}

// Cancel drops everything still queued and aborts the download in progress.
// Songs already stored stay.
func (m *Manager) Cancel() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.cancel != nil {
		m.cancel()
	}
	m.queue, m.queued = nil, map[string]bool{}
}
