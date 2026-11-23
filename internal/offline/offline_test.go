package offline

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestStoreSavePathRemove(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	n, err := st.Save("abc123", "mp3", strings.NewReader("audio-bytes"))
	if err != nil || n != int64(len("audio-bytes")) {
		t.Fatalf("Save = %d, %v", n, err)
	}
	p, ok := st.Path("abc123")
	if !ok {
		t.Fatal("Path: not found after Save")
	}
	if b, _ := os.ReadFile(p); string(b) != "audio-bytes" {
		t.Fatalf("file content = %q", b)
	}
	if got := st.IDs(); len(got) != 1 || got[0] != "abc123" {
		t.Fatalf("IDs = %v", got)
	}

	// The index survives a reopen.
	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := st2.Path("abc123"); !ok {
		t.Fatal("entry lost after reopen")
	}
	if got, err := IDs(dir); err != nil || len(got) != 1 {
		t.Fatalf("IDs(dir) = %v, %v", got, err)
	}

	if err := st2.Remove("abc123"); err != nil {
		t.Fatal(err)
	}
	if _, ok := st2.Path("abc123"); ok {
		t.Fatal("Path still found after Remove")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("file should be gone, stat err = %v", err)
	}
}

func TestStoreOpenDropsMissingFilesAndPartials(t *testing.T) {
	dir := t.TempDir()
	st, _ := Open(dir)
	if _, err := st.Save("gone", "mp3", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	p, _ := st.Path("gone")
	_ = os.Remove(p) // the user deleted the file by hand
	part := filepath.Join(dir, "half.mp3.part")
	_ = os.WriteFile(part, []byte("zz"), 0o600)

	st2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if st2.Has("gone") {
		t.Fatal("entry whose file vanished must be dropped")
	}
	if _, err := os.Stat(part); !os.IsNotExist(err) {
		t.Fatal("leftover .part file must be removed")
	}
}

func TestStoreRejectsUnsafeID(t *testing.T) {
	st, _ := Open(t.TempDir())
	for _, id := range []string{"", "../evil", `a\b`, "a/b", ".."} {
		if _, err := st.Save(id, "mp3", strings.NewReader("x")); err == nil {
			t.Fatalf("Save(%q) should fail", id)
		}
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errors.New("boom") }

func TestStoreFailedSaveLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	st, _ := Open(dir)
	if _, err := st.Save("abc", "mp3", io.MultiReader(strings.NewReader("part"), errReader{})); err == nil {
		t.Fatal("expected error")
	}
	if st.Has("abc") {
		t.Fatal("failed download must not be indexed")
	}
	left, _ := filepath.Glob(filepath.Join(dir, "abc*"))
	if len(left) != 0 {
		t.Fatalf("leftover files: %v", left)
	}
}

func waitIdle(t *testing.T, m *Manager) Status {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if s := m.Status(); s.Pending == 0 {
			return s
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("manager never went idle")
	return Status{}
}

func okFetch(calls *atomic.Int32) Fetcher {
	return func(ctx context.Context, id string) (io.ReadCloser, string, error) {
		calls.Add(1)
		return io.NopCloser(strings.NewReader("data-" + id)), "mp3", nil
	}
}

func TestManagerDownloadsOnceAndRemoves(t *testing.T) {
	st, _ := Open(t.TempDir())
	var calls atomic.Int32
	m := NewManager()
	m.Bind(st, okFetch(&calls))

	m.Add([]Item{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}, {ID: "a", Title: "A"}})
	s := waitIdle(t, m)
	if s.Count != 2 || calls.Load() != 2 {
		t.Fatalf("count=%d calls=%d, want 2/2 (duplicate ignored)", s.Count, calls.Load())
	}
	if s.Bytes != int64(len("data-a")+len("data-b")) {
		t.Fatalf("bytes = %d", s.Bytes)
	}

	m.Add([]Item{{ID: "a"}}) // already downloaded: no new fetch
	waitIdle(t, m)
	if calls.Load() != 2 {
		t.Fatalf("already-downloaded song was fetched again (calls=%d)", calls.Load())
	}

	if err := m.Remove([]string{"a"}); err != nil {
		t.Fatal(err)
	}
	if st.Has("a") || !st.Has("b") {
		t.Fatal("Remove must delete only the requested song")
	}
	if err := m.Clear(); err != nil {
		t.Fatal(err)
	}
	if got := m.Status(); got.Count != 0 {
		t.Fatalf("Clear left %d songs", got.Count)
	}
}

func TestManagerCountsFailuresAndKeepsGoing(t *testing.T) {
	st, _ := Open(t.TempDir())
	m := NewManager()
	m.Bind(st, func(ctx context.Context, id string) (io.ReadCloser, string, error) {
		if id == "bad" {
			return nil, "", errors.New("server is off")
		}
		return io.NopCloser(strings.NewReader("ok")), "mp3", nil
	})
	m.Add([]Item{{ID: "bad"}, {ID: "good"}})
	s := waitIdle(t, m)
	if s.Failed != 1 || !strings.Contains(s.LastError, "server is off") || s.Count != 1 {
		t.Fatalf("status = %+v", s)
	}
}

func TestManagerBindDropsPendingWork(t *testing.T) {
	st1, _ := Open(t.TempDir())
	st2, _ := Open(t.TempDir())
	release := make(chan struct{})
	m := NewManager()
	m.Bind(st1, func(ctx context.Context, id string) (io.ReadCloser, string, error) {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
		return io.NopCloser(strings.NewReader("x")), "mp3", nil
	})
	m.Add([]Item{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	m.Bind(st2, func(ctx context.Context, id string) (io.ReadCloser, string, error) {
		return io.NopCloser(strings.NewReader("y")), "mp3", nil
	})
	close(release)
	s := waitIdle(t, m)
	if s.Count != 0 || s.Failed != 0 {
		t.Fatalf("a server switch must drop old work silently: %+v", s)
	}
}

func TestManagerStopsABatchWhenTheServerKeepsFailing(t *testing.T) {
	st, _ := Open(t.TempDir())
	var calls atomic.Int32
	m := NewManager()
	m.Bind(st, func(ctx context.Context, id string) (io.ReadCloser, string, error) {
		calls.Add(1)
		return nil, "", errors.New("connection refused")
	})
	items := make([]Item, 200)
	for i := range items {
		items[i] = Item{ID: "s" + string(rune('a'+i%26)) + string(rune('a'+i/26))}
	}
	m.Add(items)
	s := waitIdle(t, m)
	if calls.Load() != int32(maxConsecutiveFailures) {
		t.Fatalf("fetched %d times, want it to stop after %d", calls.Load(), maxConsecutiveFailures)
	}
	if s.Failed != maxConsecutiveFailures || s.Pending != 0 || !strings.Contains(s.LastError, "connection refused") {
		t.Fatalf("status = %+v", s)
	}
}

func TestManagerCancelDropsQueueAndKeepsWhatIsStored(t *testing.T) {
	st, _ := Open(t.TempDir())
	if _, err := st.Save("done", "mp3", strings.NewReader("x")); err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	m := NewManager()
	m.Bind(st, func(ctx context.Context, id string) (io.ReadCloser, string, error) {
		select {
		case <-release:
		case <-ctx.Done():
			return nil, "", ctx.Err()
		}
		return io.NopCloser(strings.NewReader("x")), "mp3", nil
	})
	m.Add([]Item{{ID: "a"}, {ID: "b"}, {ID: "c"}})
	m.Cancel()
	s := waitIdle(t, m)
	if s.Pending != 0 || s.Count != 1 || s.Failed != 0 {
		t.Fatalf("after Cancel: %+v (a cancel is not a failure)", s)
	}
	close(release)
	// The manager is usable again after a cancel.
	m.Bind(st, okFetch(new(atomic.Int32)))
	m.Add([]Item{{ID: "z"}})
	if waitIdle(t, m); !st.Has("z") {
		t.Fatal("downloads must work again after a cancel")
	}
}
