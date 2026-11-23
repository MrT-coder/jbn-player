//go:build windows

package island

import (
	"jbnplayer/internal/ipc"
	"jbnplayer/internal/offline"
)

// batchSize keeps each IPC request under the player's per-request limit.
const batchSize = 5000

type offlineResult struct {
	offline.Status
	Error string `json:"error"`
}

type offlineIDs struct {
	IDs   []string `json:"ids"`
	Error string   `json:"error"`
}

type offlineAllResult struct {
	Queued  int    `json:"queued"`  // songs sent to download
	Skipped int    `json:"skipped"` // songs that were already offline
	Error   string `json:"error"`
}

// OfflineStatus reports how many songs are stored and what is still downloading.
// The downloads run in the player process, so this asks it.
func (s *service) OfflineStatus() offlineResult {
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return offlineResult{Error: err.Error()}
	}
	st, err := cl.Offline()
	if err != nil {
		return offlineResult{Error: err.Error()}
	}
	return offlineResult{Status: st}
}

// OfflineDo asks the player to download songs (add), delete their local copy
// (remove), delete every local copy (clear), stop downloading (cancel) or use
// another folder (setdir, folder in req.Name). The server is never modified.
func (s *service) OfflineDo(op string, req ipc.Request) offlineResult {
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return offlineResult{Error: err.Error()}
	}
	st, err := cl.OfflineDo(op, req)
	if err != nil {
		return offlineResult{Error: err.Error()}
	}
	return offlineResult{Status: st}
}

// storedIDs reads the stored songs of the active server straight from the
// index file (read-only), so it works even while the player is busy.
func (s *service) storedIDs() ([]string, error) {
	s.mu.Lock()
	sv, err := s.activeServer()
	s.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return offline.IDs(offline.Dir(offline.LoadRoot(), sv.ID))
}

// OfflineIDs lists the stored songs of the active server.
func (s *service) OfflineIDs() offlineIDs {
	ids, err := s.storedIDs()
	if err != nil {
		return offlineIDs{IDs: []string{}, Error: err.Error()}
	}
	return offlineIDs{IDs: ids}
}

// OfflineAll downloads every library song matching q (all of them when q is
// empty) that is not offline yet. Songs already stored are skipped, so running
// it again only fetches what is new.
func (s *service) OfflineAll(q string) offlineAllResult {
	c, err := s.loadLib()
	if err != nil {
		return offlineAllResult{Error: err.Error()}
	}
	if c == nil {
		return offlineAllResult{Error: "library not synced yet"}
	}
	stored, err := s.storedIDs()
	if err != nil {
		return offlineAllResult{Error: err.Error()}
	}
	have := make(map[string]bool, len(stored))
	for _, id := range stored {
		have[id] = true
	}
	songs, total := c.Find(q, 0, 0)
	items := make([]offline.Item, 0, total)
	for _, x := range songs {
		if !have[x.ID] {
			items = append(items, offline.Item{ID: x.ID, Title: x.Title})
		}
	}
	res := offlineAllResult{Queued: len(items), Skipped: total - len(items)}
	if len(items) == 0 {
		return res // nothing to do: no need for the player
	}
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return offlineAllResult{Error: err.Error()}
	}
	for start := 0; start < len(items); start += batchSize {
		end := min(start+batchSize, len(items))
		if _, err := cl.OfflineDo("add", ipc.Request{Items: items[start:end]}); err != nil {
			return offlineAllResult{Error: err.Error()}
		}
	}
	return res
}
