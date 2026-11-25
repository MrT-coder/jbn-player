//go:build windows

package ipc

import (
	"errors"
	"testing"

	"jbnplayer/internal/hotkey"
)

type hkFake struct {
	*fake
	hotkey    string
	active    bool
	suspended bool
	err       error
}

func (h *hkFake) HotkeyStatus() hotkey.Status {
	return hotkey.Status{Hotkey: h.hotkey, Active: h.active && !h.suspended, Default: hotkey.Default}
}
func (h *hkFake) HotkeySet(s string) error {
	if h.err != nil {
		return h.err
	}
	h.hotkey, h.active, h.suspended = s, s != "", false
	return nil
}
func (h *hkFake) HotkeySuspend() { h.suspended = true }
func (h *hkFake) HotkeyResume()  { h.suspended = false }

func TestHotkeyRoundTrip(t *testing.T) {
	h := &hkFake{fake: &fake{}, hotkey: "Ctrl+Alt+J", active: true}
	c := serveOffline(t, h)

	if st, err := c.Hotkey(); err != nil || st.Hotkey != "Ctrl+Alt+J" || !st.Active || st.Default != hotkey.Default {
		t.Fatalf("get: %+v, %v", st, err)
	}
	if st, err := c.HotkeyDo("suspend", Request{}); err != nil || st.Active || !h.suspended {
		t.Fatalf("suspend: %+v, %v", st, err)
	}
	if st, err := c.HotkeyDo("resume", Request{}); err != nil || !st.Active {
		t.Fatalf("resume: %+v, %v", st, err)
	}
	if st, err := c.HotkeyDo("set", Request{Name: "Ctrl+Shift+K"}); err != nil || st.Hotkey != "Ctrl+Shift+K" {
		t.Fatalf("set: %+v, %v", st, err)
	}
	h.err = errors.New("another application already uses this shortcut")
	if _, err := c.HotkeyDo("set", Request{Name: "Ctrl+Alt+P"}); err == nil || err.Error() != h.err.Error() {
		t.Fatalf("conflict must reach the window, got %v", err)
	}
	if _, err := c.HotkeyDo("bogus", Request{}); err == nil {
		t.Fatal("unknown op must fail")
	}
}

func TestHotkeyUnsupported(t *testing.T) {
	c := serveOffline(t, &fake{})
	if _, err := c.Hotkey(); err == nil {
		t.Fatal("a controller without Hotkeys must answer an error")
	}
}
