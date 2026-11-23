//go:build windows

package player

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime"
	"strconv"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/ebitengine/purego"

	"jbnplayer/internal/settings"
)

// Resolver maps a track to what mpv should open (stream URL today, local file
// for offline copies later).
type Resolver func(Track) string

// State is a light snapshot of the player for the UI. It deliberately does not
// carry the queue (it can hold thousands of tracks and the UI polls every
// second): the UI re-reads the queue with Queue when Version changes.
type State struct {
	QueueLen int     `json:"queueLen"`
	Version  uint64  `json:"version"` // changes when the list of queued tracks changes
	Index    int     `json:"index"`
	Current  *Track  `json:"current,omitempty"`
	Playing  bool    `json:"playing"`
	Shuffle  bool    `json:"shuffle"`
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	Volume   int     `json:"volume"` // 0-100, the player's own gain
}

// Device is an audio output device.
type Device struct {
	Name        string `json:"name"` // what SetDevice takes ("auto" = system default)
	Description string `json:"description"`
}

// AudioInfo is everything the audio settings screen needs.
type AudioInfo struct {
	Volume      int                    `json:"volume"`
	Device      string                 `json:"device"`
	Devices     []Device               `json:"devices"`
	EQ          settings.EQ            `json:"eq"`
	Bands       [10]float64            `json:"bands"`
	Presets     map[string][10]float64 `json:"presets"`
	PresetOrder []string               `json:"presetOrder"`
}

// QueuePage is a slice of the queue.
type QueuePage struct {
	Tracks []Track `json:"tracks"`
	From   int     `json:"from"`
	Total  int     `json:"total"`
}

// Player drives libmpv-2.dll (loaded at runtime, no cgo) and owns the queue.
type Player struct {
	mu      sync.Mutex
	h       uintptr
	q       queue
	resolve Resolver
	onTrack func(Track)

	cfg       settings.Settings // volume, device, equalizer
	cfgPath   string
	saveTimer *time.Timer

	setOpt     func(h uintptr, name, val *byte) int32
	initialize func(h uintptr) int32
	command    func(h uintptr, args **byte) int32
	destroy    func(h uintptr)
	waitEvent  func(h uintptr, timeout float64) unsafe.Pointer
	getProp    func(h uintptr, name *byte) unsafe.Pointer
	free       func(p unsafe.Pointer)
}

// mpv_event ids and end-file reason we care about (see mpv/client.h).
const (
	evShutdown = 1
	evEndFile  = 7
	endEOF     = 0
)

func cstr(s string) *byte { b, _ := syscall.BytePtrFromString(s); return b }

// New loads libmpv from dll and starts the event loop. cfg is applied right
// away; later changes are saved to cfgPath (empty = do not save).
func New(dll string, resolve Resolver, cfg settings.Settings, cfgPath string) (*Player, error) {
	lib, err := syscall.LoadLibrary(dll)
	if err != nil {
		return nil, fmt.Errorf("load %s: %w", dll, err)
	}
	cfg.Normalize()
	p := &Player{resolve: resolve, cfg: cfg, cfgPath: cfgPath}
	var create func() uintptr
	purego.RegisterLibFunc(&create, uintptr(lib), "mpv_create")
	purego.RegisterLibFunc(&p.setOpt, uintptr(lib), "mpv_set_option_string")
	purego.RegisterLibFunc(&p.initialize, uintptr(lib), "mpv_initialize")
	purego.RegisterLibFunc(&p.command, uintptr(lib), "mpv_command")
	purego.RegisterLibFunc(&p.destroy, uintptr(lib), "mpv_terminate_destroy")
	purego.RegisterLibFunc(&p.waitEvent, uintptr(lib), "mpv_wait_event")
	purego.RegisterLibFunc(&p.getProp, uintptr(lib), "mpv_get_property_string")
	purego.RegisterLibFunc(&p.free, uintptr(lib), "mpv_free")

	if p.h = create(); p.h == 0 {
		return nil, errors.New("mpv_create failed")
	}
	// Audio only, small buffers: keeps the resident process light.
	for _, kv := range [][2]string{{"vid", "no"}, {"terminal", "no"}, {"audio-display", "no"}, {"idle", "yes"},
		{"demuxer-max-bytes", "4MiB"}, {"demuxer-max-back-bytes", "1MiB"}, {"cache", "no"},
		{"volume", strconv.Itoa(cfg.Volume)}, {"volume-max", "100"}} {
		if rc := p.setOpt(p.h, cstr(kv[0]), cstr(kv[1])); rc < 0 {
			return nil, fmt.Errorf("mpv option %s: error %d", kv[0], rc)
		}
	}
	if f := cfg.EQ.Filter(); f != "" {
		_ = p.setOpt(p.h, cstr("af"), cstr(f)) // before initialize: it is an option, not a command
	}
	if rc := p.initialize(p.h); rc < 0 {
		return nil, fmt.Errorf("mpv_initialize: error %d", rc)
	}
	// The saved device may be gone (unplugged speaker): fall back to the system default.
	if cfg.AudioDevice != settings.AutoDevice {
		if err := p.setDevice(cfg.AudioDevice); err != nil {
			log.Println("saved audio device unavailable, using the default:", err)
			p.cfg.AudioDevice = settings.AutoDevice
		}
	}
	go p.events()
	return p, nil
}

// OnTrack registers a callback invoked whenever a track starts loading.
func (p *Player) OnTrack(f func(Track)) {
	p.mu.Lock()
	p.onTrack = f
	p.mu.Unlock()
}

func (p *Player) cmd(args ...string) error {
	c := make([]*byte, 0, len(args)+1)
	for _, a := range args {
		c = append(c, cstr(a))
	}
	c = append(c, nil)
	rc := p.command(p.h, (**byte)(unsafe.Pointer(&c[0])))
	runtime.KeepAlive(c)
	if rc < 0 {
		return fmt.Errorf("mpv %s: error %d", args[0], rc)
	}
	return nil
}

// prop reads an mpv property as a string ("" when unavailable).
func (p *Player) prop(name string) string {
	ptr := p.getProp(p.h, cstr(name))
	if ptr == nil {
		return ""
	}
	defer p.free(ptr)
	n := 0
	for *(*byte)(unsafe.Add(ptr, n)) != 0 {
		n++
	}
	return string(unsafe.Slice((*byte)(ptr), n))
}

func (p *Player) idle() bool { return p.prop("idle-active") == "yes" }

// events blocks in mpv_wait_event (no polling) and advances on natural EOF.
func (p *Player) events() {
	for {
		ev := p.waitEvent(p.h, -1)
		if ev == nil {
			continue
		}
		switch *(*int32)(ev) {
		case evShutdown:
			return
		case evEndFile:
			data := *(*unsafe.Pointer)(unsafe.Add(ev, 16)) // mpv_event.data
			if data != nil && *(*int32)(data) == endEOF {
				p.Next()
			}
		}
	}
}

// playCurrent loads the queue's current track. Caller holds p.mu.
func (p *Player) playCurrent() error {
	t, ok := p.q.current()
	if !ok {
		return errors.New("nothing to play")
	}
	if p.onTrack != nil {
		p.onTrack(t)
	}
	url := p.resolve(t)
	if url == "" {
		return errors.New("no server configured")
	}
	// Force playing even if the player was paused by the user.
	_ = p.cmd("set", "pause", "no")
	return p.cmd("loadfile", url)
}

// Play replaces the queue and starts at index start.
func (p *Player) Play(ts []Track, start int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(ts) == 0 {
		return errors.New("empty queue")
	}
	p.q.set(ts, start)
	return p.playCurrent()
}

// add inserts tracks with ins (insertNext or appendTracks). If nothing is
// playing (empty queue or finished), the first added track starts right away.
func (p *Player) add(ts []Track, ins func([]Track) int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(ts) == 0 {
		return nil
	}
	_, had := p.q.current()
	wasIdle := !had || p.idle()
	at := ins(ts)
	if wasIdle {
		p.q.jump(at)
		return p.playCurrent()
	}
	return nil
}

// Enqueue appends tracks to the end of the queue ("add to queue").
func (p *Player) Enqueue(ts []Track) error { return p.add(ts, p.q.appendTracks) }

// PlayNext inserts tracks right after the current one ("play next").
func (p *Player) PlayNext(ts []Track) error { return p.add(ts, p.q.insertNext) }

// Jump plays the queued track at index i.
func (p *Player) Jump(i int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.q.jump(i) {
		return errors.New("no such queue position")
	}
	return p.playCurrent()
}

// Remove drops a queued track (not the current one).
func (p *Player) Remove(i int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.q.remove(i) {
		return errors.New("cannot remove that track")
	}
	return nil
}

// ClearUpcoming empties the queue after the current track.
func (p *Player) ClearUpcoming() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.q.clearUpcoming()
}

// SetShuffle turns shuffle on or off (see queue.setShuffle).
func (p *Player) SetShuffle(on bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.q.setShuffle(on)
}

// HasQueue reports whether anything is queued.
func (p *Player) HasQueue() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, ok := p.q.current()
	return ok
}

// TogglePause pauses or resumes; if the queue already finished it replays the last track.
func (p *Player) TogglePause() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.q.current(); ok && p.idle() {
		return p.playCurrent()
	}
	return p.cmd("cycle", "pause")
}

// Next plays the next track; at the end of the queue it just stops.
func (p *Player) Next() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.q.next(); ok {
		_ = p.playCurrent()
	}
}

// Prev plays the previous track (restarts the first one).
func (p *Player) Prev() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if _, ok := p.q.prev(); ok {
		_ = p.playCurrent()
	}
}

// State returns a light snapshot for the UI.
func (p *Player) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	s := State{QueueLen: len(p.q.tracks), Version: p.q.version, Index: p.q.idx, Shuffle: p.q.shuffle, Volume: p.cfg.Volume}
	if t, ok := p.q.current(); ok {
		cur := t
		s.Current = &cur
		if !p.idle() {
			s.Playing = p.prop("pause") == "no"
			s.Position, _ = strconv.ParseFloat(p.prop("time-pos"), 64)
			s.Duration, _ = strconv.ParseFloat(p.prop("duration"), 64)
		}
	}
	return s
}

// Queue returns up to n queued tracks starting at index from.
func (p *Player) Queue(from, n int) QueuePage {
	p.mu.Lock()
	defer p.mu.Unlock()
	total := len(p.q.tracks)
	if from < 0 {
		from = 0
	}
	if from > total {
		from = total
	}
	end := total
	if n > 0 && from+n < end {
		end = from + n
	}
	return QueuePage{Tracks: append([]Track{}, p.q.tracks[from:end]...), From: from, Total: total}
}

// Move puts the queued track at index from at index to (the current track keeps playing).
func (p *Player) Move(from, to int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.q.move(from, to) {
		return errors.New("cannot move that track there")
	}
	return nil
}

// save writes the settings a moment after the last change (sliders send many).
// Caller holds p.mu.
func (p *Player) save() {
	if p.cfgPath == "" {
		return
	}
	if p.saveTimer != nil {
		p.saveTimer.Stop()
	}
	p.saveTimer = time.AfterFunc(600*time.Millisecond, func() {
		p.mu.Lock()
		cfg := p.cfg
		p.mu.Unlock()
		if err := settings.Save(p.cfgPath, cfg); err != nil {
			log.Println("save settings:", err)
		}
	})
}

// SetVolume sets the player's own volume (0-100). The Windows volume is untouched.
func (p *Player) SetVolume(level int) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if level < 0 {
		level = 0
	}
	if level > 100 {
		level = 100
	}
	if err := p.cmd("set", "volume", strconv.Itoa(level)); err != nil {
		return err
	}
	p.cfg.Volume = level
	p.save()
	return nil
}

// Devices lists the audio outputs mpv can use, "auto" first.
func (p *Player) Devices() []Device {
	var list []Device
	if err := json.Unmarshal([]byte(p.prop("audio-device-list")), &list); err != nil || len(list) == 0 {
		return []Device{{Name: settings.AutoDevice, Description: "System default"}}
	}
	return list
}

func (p *Player) setDevice(name string) error {
	known := false
	for _, d := range p.Devices() {
		if d.Name == name {
			known = true
			break
		}
	}
	if !known {
		return errors.New("unknown audio device")
	}
	return p.cmd("set", "audio-device", name)
}

// SetDevice sends the audio to another output device.
func (p *Player) SetDevice(name string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.setDevice(name); err != nil {
		return err
	}
	p.cfg.AudioDevice = name
	p.save()
	return nil
}

// SetEQ applies the equalizer. Off or flat removes the filter entirely.
func (p *Player) SetEQ(e settings.EQ) error {
	e.Normalize()
	if g, ok := settings.Presets[e.Preset]; ok && g != e.Gains {
		e.Preset = "custom" // the sliders were moved away from the preset
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if err := p.cmd("set", "af", e.Filter()); err != nil {
		return err
	}
	p.cfg.EQ = e
	p.save()
	return nil
}

// Audio returns the current audio settings and what can be chosen.
func (p *Player) Audio() AudioInfo {
	p.mu.Lock()
	defer p.mu.Unlock()
	return AudioInfo{
		Volume: p.cfg.Volume, Device: p.cfg.AudioDevice, Devices: p.Devices(), EQ: p.cfg.EQ,
		Bands: settings.Bands, Presets: settings.Presets, PresetOrder: settings.PresetOrder,
	}
}

// Close shuts libmpv down.
func (p *Player) Close() { p.destroy(p.h) }
