//go:build windows

package ipc

import (
	"errors"
	"path/filepath"
	"testing"

	"jbnplayer/internal/offline"
)

type offFake struct {
	*fake
	added     []offline.Item
	removed   []string
	cleared   bool
	cancelled bool
	dir       string
	err       error
}

func (o *offFake) OfflineStatus() offline.Status {
	return offline.Status{Count: len(o.added), Pending: 2}
}
func (o *offFake) OfflineAdd(it []offline.Item) error { o.added = append(o.added, it...); return o.err }
func (o *offFake) OfflineRemove(ids []string) error {
	o.removed = append(o.removed, ids...)
	return o.err
}
func (o *offFake) OfflineClear() error { o.cleared = true; return o.err }

func serveOffline(t *testing.T, c Controller) *Client {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ipc.json")
	stop, err := Serve(path, c)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	cl, err := Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	return cl
}

func TestOfflineRoundTrip(t *testing.T) {
	o := &offFake{fake: &fake{}}
	c := serveOffline(t, o)

	st, err := c.OfflineDo("add", Request{Items: []offline.Item{{ID: "a", Title: "A"}, {ID: "b"}}})
	if err != nil || st.Count != 2 || st.Pending != 2 {
		t.Fatalf("add: %+v, %v", st, err)
	}
	if len(o.added) != 2 || o.added[0].ID != "a" {
		t.Fatalf("added = %+v", o.added)
	}
	if _, err := c.OfflineDo("remove", Request{IDs: []string{"a"}}); err != nil || len(o.removed) != 1 || o.removed[0] != "a" {
		t.Fatalf("remove: %v %v", o.removed, err)
	}
	if _, err := c.OfflineDo("clear", Request{}); err != nil || !o.cleared {
		t.Fatalf("clear: %v", err)
	}
	if st, err := c.Offline(); err != nil || st.Count != 2 {
		t.Fatalf("status: %+v, %v", st, err)
	}
}

func TestOfflineErrorsComeBackAndAreRejectedWithoutSupport(t *testing.T) {
	o := &offFake{fake: &fake{}, err: errors.New("disk full")}
	if _, err := serveOffline(t, o).OfflineDo("add", Request{Items: []offline.Item{{ID: "a"}}}); err == nil || err.Error() != "disk full" {
		t.Fatalf("err = %v", err)
	}
	// A controller without offline support answers a clear error, not a crash.
	if _, err := serveOffline(t, &fake{}).Offline(); err == nil {
		t.Fatal("expected an error when offline is not available")
	}
}

func (o *offFake) OfflineCancel()                 { o.cancelled = true }
func (o *offFake) OfflineSetDir(dir string) error { o.dir = dir; return o.err }

func TestOfflineCancelAndSetDir(t *testing.T) {
	o := &offFake{fake: &fake{}}
	c := serveOffline(t, o)
	if _, err := c.OfflineDo("cancel", Request{}); err != nil || !o.cancelled {
		t.Fatalf("cancel: %v", err)
	}
	if _, err := c.OfflineDo("setdir", Request{Name: `D:\Music\offline`}); err != nil || o.dir != `D:\Music\offline` {
		t.Fatalf("setdir: %q %v", o.dir, err)
	}
	o.err = errors.New("cannot write")
	if _, err := c.OfflineDo("setdir", Request{Name: `E:\nope`}); err == nil || err.Error() != "cannot write" {
		t.Fatalf("setdir error must reach the client, got %v", err)
	}
}
