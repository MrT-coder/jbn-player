//go:build windows

package main

import (
	"log"
	"os/exec"
	"sync"
	"time"

	"jbnplayer/internal/island"
)

// islandCtl owns the popup window process: at most one at a time.
type islandCtl struct {
	exe    string
	onExit func() // runs after the window closed (it may have changed the servers)

	mu         sync.Mutex
	cmd        *exec.Cmd
	lastClick  time.Time
	lastClosed time.Time
}

// open starts the popup (it places itself in the bottom-right corner).
// Does nothing if it is already open.
func (c *islandCtl) open() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd != nil {
		return
	}
	cmd := exec.Command(c.exe, "--island")
	if err := cmd.Start(); err != nil {
		log.Println("island:", err)
		return
	}
	island.AllowForeground(cmd.Process.Pid) // so the popup can take focus from the shell
	c.cmd = cmd
	go func() {
		err := cmd.Wait()
		log.Println("popup process ended:", err)
		c.mu.Lock()
		c.cmd = nil
		c.lastClosed = time.Now()
		c.mu.Unlock()
		c.onExit()
	}()
}

// kill closes the popup if it is open: first politely (WM_CLOSE, so WebView2
// shuts down cleanly), and by force if it has not gone after a few seconds.
func (c *islandCtl) kill() {
	c.mu.Lock()
	cmd := c.cmd
	c.mu.Unlock()
	if cmd == nil {
		return
	}
	if !island.Close(cmd.Process.Pid) {
		_ = cmd.Process.Kill()
		return
	}
	time.AfterFunc(3*time.Second, func() {
		c.mu.Lock()
		stillOpen := c.cmd == cmd
		c.mu.Unlock()
		if stillOpen {
			_ = cmd.Process.Kill()
		}
	})
}

// toggle is the tray icon's left click: open the popup, or close it if open.
// Pressing the icon steals the popup's focus, so the popup usually closes
// itself just before this runs: a click right after a close is that same click.
func (c *islandCtl) toggle() {
	c.mu.Lock()
	now := time.Now()
	if now.Sub(c.lastClick) < 300*time.Millisecond { // a double click fires twice
		c.mu.Unlock()
		return
	}
	c.lastClick = now
	running := c.cmd != nil
	justClosed := now.Sub(c.lastClosed) < 500*time.Millisecond
	c.mu.Unlock()
	switch {
	case running:
		log.Println("tray click: closing the popup")
		c.kill()
	case justClosed:
		log.Println("tray click: ignored (the popup had just closed itself)")
	default:
		log.Println("tray click: opening the popup")
		c.open()
	}
}
