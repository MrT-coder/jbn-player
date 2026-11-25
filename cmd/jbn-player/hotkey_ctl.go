//go:build windows

package main

import (
	"log"

	"jbnplayer/internal/hotkey"
)

// hotkeyCtl is the global shortcut that toggles the window, plus where the
// choice is saved. Only this process registers it and writes the file.
type hotkeyCtl struct {
	mgr  *hotkey.Manager
	path string
}

// startHotkey registers the saved shortcut (Ctrl+Alt+J on a fresh install).
// If another application has it, the choice is kept and the window says so.
func startHotkey(onPress func()) *hotkeyCtl {
	h := &hotkeyCtl{mgr: hotkey.Start(onPress)}
	path, err := hotkey.DefaultPath()
	if err != nil {
		log.Println("hotkey:", err)
	}
	h.path = path
	c, err := hotkey.Load(path)
	if err != nil {
		log.Println("hotkey:", err) // the default is used
	}
	if err := h.mgr.SetChoice(c); err != nil {
		log.Printf("hotkey %s: %v", c, err)
	}
	return h
}

func (h *hotkeyCtl) HotkeyStatus() hotkey.Status { return h.mgr.Status() }
func (h *hotkeyCtl) HotkeySuspend()              { h.mgr.Suspend() }
func (h *hotkeyCtl) HotkeyResume()               { h.mgr.Resume() }

// HotkeySet validates, registers and saves a new shortcut ("" turns it off).
// Nothing is saved when Windows refuses it.
func (h *hotkeyCtl) HotkeySet(s string) error {
	c, err := hotkey.Parse(s)
	if err != nil {
		return err
	}
	if err := h.mgr.Set(c); err != nil {
		return err
	}
	if h.path == "" {
		return nil
	}
	return hotkey.Save(h.path, c)
}
