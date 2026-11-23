// Audio spike: stream one song from Navidrome as MP3 and play it with beep/oto.
package main

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
)

func envInt(name string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(name)); err == nil {
		return v
	}
	return def
}

// trimWorkingSet asks Windows to drop pages not recently used.
func trimWorkingSet() {
	k := syscall.NewLazyDLL("kernel32.dll")
	h, _ := syscall.GetCurrentProcess()
	k.NewProc("SetProcessWorkingSetSize").Call(uintptr(h), ^uintptr(0), ^uintptr(0))
}

func loadEnv(path string) map[string]string {
	m := map[string]string{}
	f, err := os.Open(path)
	if err != nil {
		panic(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			m[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return m
}

type client struct{ base, user, pass string }

func (c client) u(endpoint string, extra url.Values) string {
	salt := fmt.Sprintf("%08x", rand.Uint32())
	sum := md5.Sum([]byte(c.pass + salt))
	q := url.Values{}
	q.Set("u", c.user)
	q.Set("t", hex.EncodeToString(sum[:]))
	q.Set("s", salt)
	q.Set("v", "1.16.1")
	q.Set("c", "jbn-player-spike")
	q.Set("f", "json")
	for k, v := range extra {
		q[k] = v
	}
	return c.base + "/rest/" + endpoint + ".view?" + q.Encode()
}

func (c client) getJSON(endpoint string, extra url.Values, out any) error {
	resp, err := http.Get(c.u(endpoint, extra))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

func main() {
	env := loadEnv("../../.env")
	c := client{strings.TrimRight(env["NAVIDROME_URL"], "/"), env["NAVIDROME_USER"], env["NAVIDROME_PASSWORD"]}

	var al struct {
		R struct {
			AL struct {
				Album []struct{ ID string } `json:"album"`
			} `json:"albumList2"`
		} `json:"subsonic-response"`
	}
	if err := c.getJSON("getAlbumList2", url.Values{"type": {"random"}, "size": {"1"}}, &al); err != nil || len(al.R.AL.Album) == 0 {
		panic(fmt.Sprint("album list failed: ", err))
	}
	var ab struct {
		R struct {
			Album struct {
				Name string
				Song []struct {
					ID, Title, Artist, Suffix string
				} `json:"song"`
			} `json:"album"`
		} `json:"subsonic-response"`
	}
	if err := c.getJSON("getAlbum", url.Values{"id": {al.R.AL.Album[0].ID}}, &ab); err != nil || len(ab.R.Album.Song) == 0 {
		panic(fmt.Sprint("album failed: ", err))
	}
	s := ab.R.Album.Song[0]
	fmt.Printf("PID=%d playing: %s - %s (original: %s)\n", os.Getpid(), s.Artist, s.Title, s.Suffix)

	if gc := envInt("SPIKE_GC", 0); gc > 0 {
		debug.SetGCPercent(gc)
	}
	resp, err := http.Get(c.u("stream", url.Values{"id": {s.ID}, "format": {"mp3"}, "maxBitRate": {"192"}}))
	if err != nil {
		panic(err)
	}
	streamer, format, err := mp3.Decode(resp.Body)
	if err != nil {
		panic(err)
	}
	defer streamer.Close()
	if err := speaker.Init(format.SampleRate, format.SampleRate.N(time.Duration(envInt("SPIKE_BUF_MS", 200))*time.Millisecond)); err != nil {
		panic(err)
	}
	done := make(chan struct{})
	speaker.Play(beep.Seq(streamer, beep.Callback(func() { close(done) })))
	if envInt("SPIKE_TRIM", 0) == 1 {
		go func() {
			for {
				time.Sleep(2 * time.Second)
				debug.FreeOSMemory()
				trimWorkingSet()
			}
		}()
	}
	select {
	case <-done:
	case <-time.After(45 * time.Second):
	}
	fmt.Println("finished")
}
