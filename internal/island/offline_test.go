//go:build windows

package island

import (
	"path/filepath"
	"strings"
	"testing"

	"jbnplayer/internal/ipc"
	"jbnplayer/internal/library"
	"jbnplayer/internal/offline"
)

type offCtl struct {
	*ctl
	added   []offline.Item
	removed []string
}

func (o *offCtl) OfflineStatus() offline.Status {
	return offline.Status{Count: 1, Pending: len(o.added)}
}
func (o *offCtl) OfflineAdd(it []offline.Item) error { o.added = append(o.added, it...); return nil }
func (o *offCtl) OfflineRemove(ids []string) error   { o.removed = append(o.removed, ids...); return nil }
func (o *offCtl) OfflineClear() error                { return nil }
func (o *offCtl) OfflineCancel()                     {}
func (o *offCtl) OfflineSetDir(string) error         { return nil }

func TestOfflineBridgeSendsSongsToThePlayer(t *testing.T) {
	s := libService(t)
	o := &offCtl{ctl: &ctl{}}
	stop, err := ipc.Serve(s.ipcPath, o)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	r := s.OfflineDo("add", ipc.Request{Items: []offline.Item{{ID: "s1", Title: "One"}}})
	if r.Error != "" || r.Pending != 1 || len(o.added) != 1 || o.added[0].ID != "s1" {
		t.Fatalf("add: %+v / %+v", r, o.added)
	}
	if r = s.OfflineDo("remove", ipc.Request{IDs: []string{"s1"}}); r.Error != "" || len(o.removed) != 1 {
		t.Fatalf("remove: %+v", r)
	}
	if r = s.OfflineStatus(); r.Error != "" || r.Count != 1 {
		t.Fatalf("status: %+v", r)
	}
}

func TestOfflineWhenPlayerNotRunning(t *testing.T) {
	s := libService(t) // no ipc server
	if r := s.OfflineStatus(); r.Error != "player is not running" {
		t.Fatalf("got %+v", r)
	}
}

func TestOfflineIDsReadsTheActiveServersFolder(t *testing.T) {
	s := libService(t)
	sv, err := s.activeServer()
	if err != nil {
		t.Fatal(err)
	}
	if got := s.OfflineIDs(); got.Error != "" || len(got.IDs) != 0 {
		t.Fatalf("empty: %+v", got)
	}
	st, err := offline.Open(offline.Dir(useRoot(t), sv.ID))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.Save("s1", "mp3", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	if got := s.OfflineIDs(); got.Error != "" || len(got.IDs) != 1 || got.IDs[0] != "s1" {
		t.Fatalf("ids: %+v (dir %s)", got, filepath.Base(s.libDir))
	}
}

// useRoot points the offline root at a temp folder (and its config file too),
// so tests never touch the real one.
func useRoot(t *testing.T) string {
	t.Helper()
	t.Setenv("JBN_PLAYER_OFFLINE", filepath.Join(t.TempDir(), "offline.json"))
	root := t.TempDir()
	if err := offline.SaveRoot(root); err != nil {
		t.Fatal(err)
	}
	return root
}

func syncedService(t *testing.T) *service {
	t.Helper()
	s := libService(t)
	sv, _ := s.activeServer()
	c := &library.Cache{Version: 1, Songs: []library.Song{
		{ID: "s1", Title: "Alpha one"}, {ID: "s2", Title: "Alpha two"}, {ID: "s3", Title: "Beta three"},
	}}
	if err := c.Save(s.libPath(sv.ID)); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestOfflineAllSendsOnlyWhatIsNotStored(t *testing.T) {
	root := useRoot(t)
	s := syncedService(t)
	sv, _ := s.activeServer()
	st, _ := offline.Open(offline.Dir(root, sv.ID))
	if _, err := st.Save("s1", "mp3", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	o := &offCtl{ctl: &ctl{}}
	stop, err := ipc.Serve(s.ipcPath, o)
	if err != nil {
		t.Fatal(err)
	}
	defer stop()

	r := s.OfflineAll("")
	if r.Error != "" || r.Queued != 2 || r.Skipped != 1 {
		t.Fatalf("all: %+v", r)
	}
	if len(o.added) != 2 || o.added[0].ID != "s2" || o.added[1].ID != "s3" {
		t.Fatalf("sent %+v, want s2 and s3 only", o.added)
	}

	// With a search, only the matches are considered.
	o.added = nil
	if r = s.OfflineAll("alpha"); r.Error != "" || r.Queued != 1 || r.Skipped != 1 || len(o.added) != 1 || o.added[0].ID != "s2" {
		t.Fatalf("filtered: %+v / %+v", r, o.added)
	}
}

func TestOfflineAllWithNothingMissingDoesNotNeedThePlayer(t *testing.T) {
	root := useRoot(t)
	s := syncedService(t) // no ipc server on purpose
	sv, _ := s.activeServer()
	st, _ := offline.Open(offline.Dir(root, sv.ID))
	for _, id := range []string{"s1", "s2", "s3"} {
		if _, err := st.Save(id, "mp3", strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
	}
	if r := s.OfflineAll(""); r.Error != "" || r.Queued != 0 || r.Skipped != 3 {
		t.Fatalf("everything is stored already: %+v", r)
	}
}

func TestOfflineAllNeedsASyncedLibrary(t *testing.T) {
	useRoot(t)
	if r := libService(t).OfflineAll(""); r.Error != "library not synced yet" {
		t.Fatalf("got %+v", r)
	}
}
