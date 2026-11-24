//go:build windows

package ipc

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"jbnplayer/internal/player"
	"jbnplayer/internal/settings"
)

type fake struct {
	st    player.State
	queue []player.Track
	calls []string
	vol   int
	dev   string
	eq    settings.EQ
}

func (f *fake) State() player.State {
	s := f.st
	s.QueueLen = len(f.queue)
	return s
}
func (f *fake) Queue(from, n int) player.QueuePage {
	end := from + n
	if end > len(f.queue) {
		end = len(f.queue)
	}
	return player.QueuePage{Tracks: f.queue[from:end], From: from, Total: len(f.queue)}
}
func (f *fake) TogglePause() error { f.calls = append(f.calls, "toggle"); return nil }
func (f *fake) Next()              { f.calls = append(f.calls, "next") }
func (f *fake) Prev()              { f.calls = append(f.calls, "prev") }
func (f *fake) Play(ts []player.Track, start int) error {
	f.calls = append(f.calls, "play")
	f.queue, f.st.Index = ts, start
	return nil
}
func (f *fake) Enqueue(ts []player.Track) error {
	f.calls = append(f.calls, "enqueue")
	f.queue = append(f.queue, ts...)
	return nil
}
func (f *fake) PlayNext(ts []player.Track) error { f.calls = append(f.calls, "playnext"); return nil }
func (f *fake) Jump(i int) error {
	if i > 5 {
		return errors.New("no such queue position")
	}
	f.calls = append(f.calls, "jump")
	return nil
}
func (f *fake) Move(from, to int) error {
	if from > 5 {
		return errors.New("cannot move that track there")
	}
	f.calls = append(f.calls, "move")
	return nil
}
func (f *fake) Audio() player.AudioInfo {
	return player.AudioInfo{Volume: f.vol, Device: f.dev, EQ: f.eq, Devices: []player.Device{{Name: "auto"}}}
}
func (f *fake) SetVolume(l int) error { f.vol = l; f.calls = append(f.calls, "volume"); return nil }
func (f *fake) SetDevice(n string) error {
	if n == "nope" {
		return errors.New("unknown audio device")
	}
	f.dev = n
	f.calls = append(f.calls, "device")
	return nil
}
func (f *fake) SetEQ(e settings.EQ) error { f.eq = e; f.calls = append(f.calls, "eq"); return nil }
func (f *fake) Remove(i int) error        { f.calls = append(f.calls, "remove"); return nil }
func (f *fake) ClearUpcoming()            { f.calls = append(f.calls, "clear") }
func (f *fake) SetShuffle(on bool)        { f.st.Shuffle = on; f.calls = append(f.calls, "shuffle") }

func start(t *testing.T) (*Client, *fake, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ipc.json")
	f := &fake{}
	stop, err := Serve(path, f)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
	c, err := Dial(path)
	if err != nil {
		t.Fatal(err)
	}
	return c, f, path
}

func TestRoundTripPlayEnqueueShuffle(t *testing.T) {
	c, f, _ := start(t)
	tracks := []player.Track{{ID: "a", Title: "A"}, {ID: "b", Title: "B"}}
	st, err := c.Do("play", Request{Tracks: tracks, Start: 1})
	if err != nil || st.QueueLen != 2 || st.Index != 1 {
		t.Fatalf("play: %+v %v", st, err)
	}
	if st, err = c.Do("enqueue", Request{Tracks: []player.Track{{ID: "c"}}}); err != nil || st.QueueLen != 3 {
		t.Fatalf("enqueue: %+v %v", st, err)
	}
	page, err := c.Queue(1, 10)
	if err != nil || page.Total != 3 || page.From != 1 || len(page.Tracks) != 2 || page.Tracks[0].ID != "b" {
		t.Fatalf("queue page: %+v %v", page, err)
	}
	if st, err = c.Do("shuffle", Request{On: true}); err != nil || !st.Shuffle {
		t.Fatalf("shuffle: %+v %v", st, err)
	}
	if got := strings.Join(f.calls, ","); got != "play,enqueue,shuffle" {
		t.Fatalf("calls = %s", got)
	}
}

func TestControllerErrorsReachTheClient(t *testing.T) {
	c, _, _ := start(t)
	if _, err := c.Do("jump", Request{Index: 9}); err == nil || !strings.Contains(err.Error(), "no such queue position") {
		t.Fatalf("got %v", err)
	}
}

func TestRejectsMissingOrWrongToken(t *testing.T) {
	c, _, _ := start(t)
	for _, tok := range []string{"", "Bearer nope"} {
		req, _ := http.NewRequest("GET", c.base+"/state", nil)
		if tok != "" {
			req.Header.Set("Authorization", tok)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Fatalf("token %q: status %d, want 403", tok, resp.StatusCode)
		}
	}
}

func TestRejectsForeignHostHeader(t *testing.T) {
	c, _, _ := start(t)
	req, _ := http.NewRequest("GET", c.base+"/state", nil)
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Host = "evil.example" // DNS-rebinding style request
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status %d, want 403", resp.StatusCode)
	}
}

func TestTooManyTracksRejected(t *testing.T) {
	c, _, _ := start(t)
	many := make([]player.Track, maxTracks+1)
	if _, err := c.Do("enqueue", Request{Tracks: many}); err == nil || !strings.Contains(err.Error(), "too many") {
		t.Fatalf("got %v", err)
	}
}

func TestStopRemovesEndpointFileAndDialFailsWithoutIt(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ipc.json")
	stop, err := Serve(path, &fake{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	var ep endpointFile
	if json.Unmarshal(raw, &ep) != nil || ep.Port == 0 || ep.Token == "" {
		t.Fatalf("bad endpoint file: %s", raw)
	}
	stop()
	if _, err := os.Stat(path); err == nil {
		t.Fatal("endpoint file must be removed on stop")
	}
	if _, err := Dial(path); err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("got %v", err)
	}
}

func TestMoveGoesThroughAndErrorsComeBack(t *testing.T) {
	c, f, _ := start(t)
	if _, err := c.Do("move", Request{From: 1, To: 3}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Do("move", Request{From: 9, To: 0}); err == nil || !strings.Contains(err.Error(), "cannot move") {
		t.Fatalf("got %v", err)
	}
	if got := strings.Join(f.calls, ","); got != "move" {
		t.Fatalf("calls = %s", got)
	}
}

func TestAudioSettingsRoundTrip(t *testing.T) {
	c, _, _ := start(t)
	a, err := c.DoAudio("volume", Request{Level: 40})
	if err != nil || a.Volume != 40 {
		t.Fatalf("volume: %+v %v", a, err)
	}
	a, err = c.DoAudio("device", Request{Name: "wasapi/x"})
	if err != nil || a.Device != "wasapi/x" {
		t.Fatalf("device: %+v %v", a, err)
	}
	eq := settings.EQ{Enabled: true, Preset: "bass", Gains: settings.Presets["bass"]}
	a, err = c.DoAudio("eq", Request{EQ: &eq})
	if err != nil || !a.EQ.Enabled || a.EQ.Gains[0] != 6 {
		t.Fatalf("eq: %+v %v", a, err)
	}
	if _, err := c.DoAudio("device", Request{Name: "nope"}); err == nil || !strings.Contains(err.Error(), "unknown audio device") {
		t.Fatalf("bad device: %v", err)
	}
	if _, err := c.DoAudio("eq", Request{}); err == nil {
		t.Fatal("eq without a body must fail")
	}
	got, err := c.Audio()
	if err != nil || got.Volume != 40 || got.Device != "wasapi/x" {
		t.Fatalf("read back: %+v %v", got, err)
	}
}

func (f *fake) PlayNow(ts []player.Track, start int) error {
	f.calls = append(f.calls, "playnow:"+ts[start].ID)
	return nil
}
func (f *fake) Seek(pos float64) error {
	if pos > 1000 {
		return errors.New("nothing is playing")
	}
	f.st.Position = pos
	f.calls = append(f.calls, "seek")
	return nil
}

func TestPlayNowAndSeekGoThrough(t *testing.T) {
	c, f, _ := start(t)
	if _, err := c.Do("playnow", Request{Tracks: []player.Track{{ID: "a"}, {ID: "b"}}, Start: 1}); err != nil {
		t.Fatal(err)
	}
	st, err := c.Do("seek", Request{Position: 42.5})
	if err != nil || st.Position != 42.5 {
		t.Fatalf("seek: %+v %v", st, err)
	}
	if _, err := c.Do("seek", Request{Position: 5000}); err == nil || !strings.Contains(err.Error(), "nothing is playing") {
		t.Fatalf("seek error: %v", err)
	}
	if got := strings.Join(f.calls, ","); got != "playnow:b,seek" {
		t.Fatalf("calls = %s", got)
	}
}
