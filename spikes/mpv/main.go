// libmpv spike: play the ORIGINAL file (no transcoding) from Navidrome via libmpv-2.dll, no cgo.
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
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"
)

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
	q := url.Values{"u": {c.user}, "t": {hex.EncodeToString(sum[:])}, "s": {salt}, "v": {"1.16.1"}, "c": {"jbn-player-spike"}, "f": {"json"}}
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

func cstr(s string) *byte { b, _ := syscall.BytePtrFromString(s); return b }

func main() {
	dll := "bin/libmpv-2.dll"
	if len(os.Args) > 1 {
		dll = os.Args[1]
	}
	lib, err := syscall.LoadLibrary(dll)
	if err != nil {
		panic(err)
	}
	var create func() uintptr
	var setOpt func(h uintptr, name, val *byte) int32
	var initialize func(h uintptr) int32
	var command func(h uintptr, args **byte) int32
	var destroy func(h uintptr)
	purego.RegisterLibFunc(&create, uintptr(lib), "mpv_create")
	purego.RegisterLibFunc(&setOpt, uintptr(lib), "mpv_set_option_string")
	purego.RegisterLibFunc(&initialize, uintptr(lib), "mpv_initialize")
	purego.RegisterLibFunc(&command, uintptr(lib), "mpv_command")
	purego.RegisterLibFunc(&destroy, uintptr(lib), "mpv_terminate_destroy")

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
				Song []struct{ ID, Title, Artist, Suffix string } `json:"song"`
			} `json:"album"`
		} `json:"subsonic-response"`
	}
	if err := c.getJSON("getAlbum", url.Values{"id": {al.R.AL.Album[0].ID}}, &ab); err != nil || len(ab.R.Album.Song) == 0 {
		panic(fmt.Sprint("album failed: ", err))
	}
	s := ab.R.Album.Song[0]
	fmt.Printf("PID=%d playing: %s - %s (original: %s)\n", os.Getpid(), s.Artist, s.Title, s.Suffix)

	h := create()
	for _, kv := range [][2]string{{"vid", "no"}, {"terminal", "no"}, {"audio-display", "no"}, {"idle", "yes"},
		{"demuxer-max-bytes", "4MiB"}, {"demuxer-max-back-bytes", "1MiB"}, {"cache", "no"}} {
		if rc := setOpt(h, cstr(kv[0]), cstr(kv[1])); rc < 0 {
			fmt.Println("option failed:", kv, rc)
		}
	}
	if rc := initialize(h); rc < 0 {
		panic(fmt.Sprint("mpv_initialize: ", rc))
	}
	streamURL := c.u("stream", url.Values{"id": {s.ID}}) // original format, no transcoding
	args := []*byte{cstr("loadfile"), cstr(streamURL), nil}
	if rc := command(h, (**byte)(unsafe.Pointer(&args[0]))); rc < 0 {
		panic(fmt.Sprint("loadfile: ", rc))
	}
	time.Sleep(45 * time.Second)
	destroy(h)
	fmt.Println("finished")
}
