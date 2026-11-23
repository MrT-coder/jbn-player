//go:build windows

// Package tray is the always-resident system tray icon and menu.
package tray

import (
	"bytes"
	"encoding/binary"
	"strconv"
	"sync"

	"github.com/energye/systray"
)

// Handlers are the menu callbacks.
type Handlers struct {
	PlayPause, Next, Prev, Open func()
	Click                       func() // left click on the icon
	Accent                      string // "#rrggbb": color of the tray icon
}

var (
	mu      sync.Mutex
	ready   bool
	current = "#2d7be0"
)

// SetAccent recolors the tray icon (safe to call before the tray is running).
func SetAccent(hex string) {
	mu.Lock()
	defer mu.Unlock()
	current = hex
	if ready {
		systray.SetIcon(icon(hex))
	}
}

// Run blocks until Quit is chosen (or Stop is called). Call it from main.
func Run(h Handlers, onExit func()) {
	systray.Run(func() {
		mu.Lock()
		if h.Accent != "" {
			current = h.Accent
		}
		systray.SetIcon(icon(current))
		ready = true
		mu.Unlock()
		systray.SetTitle("jbn-player")
		systray.SetTooltip("jbn-player")
		if h.Click != nil {
			systray.SetOnClick(func(systray.IMenu) { h.Click() }) // right click keeps showing the menu
		}
		systray.AddMenuItem("Play / Pause", "").Click(h.PlayPause)
		systray.AddMenuItem("Next", "").Click(h.Next)
		systray.AddMenuItem("Previous", "").Click(h.Prev)
		systray.AddMenuItem("Open", "").Click(h.Open)
		systray.AddSeparator()
		systray.AddMenuItem("Quit", "").Click(systray.Quit)
	}, onExit)
}

// SetTooltip updates the hover text (used for "now playing").
func SetTooltip(s string) {
	if r := []rune(s); len(r) > 120 { // Windows tooltips are limited to 127 chars
		s = string(r[:120])
	}
	systray.SetTooltip(s)
}

// rgbOf parses "#rrggbb" (anything else gives the default blue).
func rgbOf(hex string) (r, g, b byte) {
	v, err := strconv.ParseUint(hex[min(1, len(hex)):], 16, 32)
	if err != nil || len(hex) != 7 {
		return 0x2d, 0x7b, 0xe0
	}
	return byte(v >> 16), byte(v >> 8), byte(v)
}

// icon builds a 16x16 32-bit solid-color ICO in memory (no asset files yet).
func icon(hex string) []byte {
	red, green, blue := rgbOf(hex)
	const w, h = 16, 16
	var b bytes.Buffer
	le := binary.LittleEndian
	pixels, mask := w*h*4, 4*h
	_ = binary.Write(&b, le, [3]uint16{0, 1, 1}) // reserved, type=icon, count=1
	b.Write([]byte{w, h, 0, 0})
	_ = binary.Write(&b, le, [2]uint16{1, 32})
	_ = binary.Write(&b, le, uint32(40+pixels+mask))
	_ = binary.Write(&b, le, uint32(22))
	_ = binary.Write(&b, le, uint32(40))
	_ = binary.Write(&b, le, [2]int32{w, h * 2})
	_ = binary.Write(&b, le, [2]uint16{1, 32})
	_ = binary.Write(&b, le, [6]uint32{0, uint32(pixels + mask), 0, 0, 0, 0})
	for i := 0; i < w*h; i++ {
		b.Write([]byte{blue, green, red, 0xFF}) // BGRA
	}
	b.Write(make([]byte, mask))
	return b.Bytes()
}
