// Tray spike: a resident tray icon with a menu, nothing else. Measures idle cost.
package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"os"

	"github.com/energye/systray"
)

// makeICO builds a 16x16 32-bit solid-color ICO in memory.
func makeICO() []byte {
	const w, h = 16, 16
	var b bytes.Buffer
	le := binary.LittleEndian
	binary.Write(&b, le, [3]uint16{0, 1, 1}) // reserved, type=icon, count=1
	pixels := w * h * 4
	mask := 4 * h // 1bpp rows padded to 4 bytes
	b.Write([]byte{w, h, 0, 0})
	binary.Write(&b, le, [2]uint16{1, 32})
	binary.Write(&b, le, uint32(40+pixels+mask))
	binary.Write(&b, le, uint32(22))
	binary.Write(&b, le, uint32(40))
	binary.Write(&b, le, [2]int32{w, h * 2})
	binary.Write(&b, le, [2]uint16{1, 32})
	binary.Write(&b, le, [6]uint32{0, uint32(pixels + mask), 0, 0, 0, 0})
	for i := 0; i < w*h; i++ {
		b.Write([]byte{0xE0, 0x7B, 0x2D, 0xFF}) // BGRA
	}
	b.Write(make([]byte, mask))
	return b.Bytes()
}

func main() {
	fmt.Printf("PID=%d\n", os.Getpid())
	systray.Run(func() {
		systray.SetIcon(makeICO())
		systray.SetTitle("jbn-player")
		systray.SetTooltip("jbn-player spike")
		play := systray.AddMenuItem("Play / Pause", "")
		next := systray.AddMenuItem("Next", "")
		prev := systray.AddMenuItem("Previous", "")
		open := systray.AddMenuItem("Open", "")
		systray.AddSeparator()
		quit := systray.AddMenuItem("Quit", "")
		play.Click(func() { fmt.Println("play/pause") })
		next.Click(func() { fmt.Println("next") })
		prev.Click(func() { fmt.Println("prev") })
		open.Click(func() { fmt.Println("open") })
		quit.Click(func() { systray.Quit() })
	}, func() {})
}
