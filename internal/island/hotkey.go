//go:build windows

package island

import (
	"jbnplayer/internal/hotkey"
	"jbnplayer/internal/ipc"
)

type hotkeyResult struct {
	hotkey.Status
	Error string `json:"error"`
}

// GetHotkey reports the global shortcut that opens this window. The player
// process registers it, so this asks it.
func (s *service) GetHotkey() hotkeyResult { return s.HotkeyDo("", "") }

// HotkeyDo changes the shortcut (set, with "" = off), or releases it while a
// new one is being recorded (suspend) and takes it back (resume). An empty op
// only reads it.
func (s *service) HotkeyDo(op, value string) hotkeyResult {
	cl, err := ipc.Dial(s.ipcPath)
	if err != nil {
		return hotkeyResult{Error: err.Error()}
	}
	var st hotkey.Status
	if op == "" {
		st, err = cl.Hotkey()
	} else {
		st, err = cl.HotkeyDo(op, ipc.Request{Name: value})
	}
	if err != nil {
		return hotkeyResult{Status: st, Error: err.Error()}
	}
	return hotkeyResult{Status: st}
}
