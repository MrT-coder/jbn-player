//go:build windows

package hotkey

import (
	"errors"
	"runtime"
	"syscall"
	"unsafe"
)

// ErrInUse means another application already registered the combination.
var ErrInUse = errors.New("another application already uses this shortcut")

var (
	user32                = syscall.NewLazyDLL("user32.dll")
	kernel32              = syscall.NewLazyDLL("kernel32.dll")
	procRegisterHotKey    = user32.NewProc("RegisterHotKey")
	procUnregisterHotKey  = user32.NewProc("UnregisterHotKey")
	procGetMessageW       = user32.NewProc("GetMessageW")
	procPeekMessageW      = user32.NewProc("PeekMessageW")
	procPostThreadMessage = user32.NewProc("PostThreadMessageW")
	procGetCurrentThread  = kernel32.NewProc("GetCurrentThreadId")
)

const (
	wmQuit     = 0x0012
	wmHotkey   = 0x0312
	wmApp      = 0x8000
	modNoRepet = 0x4000 // holding the keys down fires once
	hotkeyID   = 1
	errInUse   = 1409 // ERROR_HOTKEY_ALREADY_REGISTERED
)

type msg struct {
	hwnd    uintptr
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      [2]int32
	private uint32
}

// Manager owns one global shortcut. RegisterHotKey ties a shortcut to the
// thread that registered it, so a dedicated OS thread registers it and waits
// in GetMessage (no CPU while idle); the other methods run on that thread.
type Manager struct {
	tid  uint32
	ops  chan func()
	done chan struct{}

	// Only touched on the manager's thread.
	combo      Combo
	registered bool
	suspended  bool
}

// Start launches the shortcut thread. onPress runs on its own goroutine each
// time the shortcut is pressed, whichever application has the focus.
func Start(onPress func()) *Manager {
	m := &Manager{ops: make(chan func(), 4), done: make(chan struct{})}
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread() // never unlocked: the thread dies with the goroutine
		defer close(m.done)
		tid, _, _ := procGetCurrentThread.Call()
		m.tid = uint32(tid)
		var q msg
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&q)), 0, 0, 0, 0) // creates the message queue
		close(ready)
		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&q)), 0, 0, 0)
			if int32(r) <= 0 { // WM_QUIT or error
				m.unregister()
				return
			}
			switch q.message {
			case wmHotkey:
				if q.wParam == hotkeyID {
					go onPress()
				}
			case wmApp:
				for drained := false; !drained; {
					select {
					case f := <-m.ops:
						f()
					default:
						drained = true
					}
				}
			}
		}
	}()
	<-ready
	return m
}

// run executes f on the shortcut thread and waits for it.
func (m *Manager) run(f func()) {
	finished := make(chan struct{})
	select {
	case <-m.done:
		return
	case m.ops <- func() { f(); close(finished) }:
	}
	procPostThreadMessage.Call(uintptr(m.tid), wmApp, 0, 0)
	select {
	case <-finished:
	case <-m.done:
	}
}

func (m *Manager) register(c Combo) error {
	r, _, err := procRegisterHotKey.Call(0, hotkeyID, uintptr(c.Mods|modNoRepet), uintptr(c.Key))
	if r == 0 {
		if errno, ok := err.(syscall.Errno); ok && errno == errInUse {
			return ErrInUse
		}
		return err
	}
	return nil
}

func (m *Manager) unregister() {
	if m.registered {
		procUnregisterHotKey.Call(0, hotkeyID)
		m.registered = false
	}
}

// Set switches to c (the zero Combo turns the shortcut off). If Windows
// refuses c, the previous shortcut stays as it was and the error says why
// (ErrInUse when another application has it). Set also ends a Suspend.
func (m *Manager) Set(c Combo) (err error) {
	m.run(func() {
		if !c.IsZero() && c == m.combo && m.registered {
			m.suspended = false
			return
		}
		prev, wasRegistered := m.combo, m.registered
		m.unregister()
		if !c.IsZero() {
			if err = m.register(c); err != nil {
				if wasRegistered {
					m.registered = m.register(prev) == nil
				}
				return
			}
			m.registered = true
		}
		m.combo, m.suspended = c, false
	})
	return err
}

// Suspend releases the shortcut for a while (the window does it while the
// user records a new one, so pressing the current one is not swallowed).
func (m *Manager) Suspend() {
	m.run(func() {
		m.unregister()
		m.suspended = true
	})
}

// Resume registers the current shortcut again after a Suspend. It is also a
// retry when it could not be registered at startup.
func (m *Manager) Resume() {
	m.run(func() {
		m.suspended = false
		if !m.combo.IsZero() && !m.registered {
			m.registered = m.register(m.combo) == nil
		}
	})
}

// Status reports the chosen shortcut and whether Windows accepted it.
func (m *Manager) Status() (s Status) {
	s.Default = Default
	m.run(func() {
		s.Hotkey = m.combo.String()
		s.Active = m.registered
	})
	return s
}

// SetChoice records c as the chosen shortcut and tries to register it; unlike
// Set, the choice is kept even when Windows refuses it (used at startup, so
// the window can say the saved shortcut is taken).
func (m *Manager) SetChoice(c Combo) (err error) {
	m.run(func() {
		m.unregister()
		m.combo, m.suspended = c, false
		if !c.IsZero() {
			err = m.register(c)
			m.registered = err == nil
		}
	})
	return err
}

// Stop releases the shortcut and ends the thread.
func (m *Manager) Stop() {
	select {
	case <-m.done:
		return
	default:
	}
	procPostThreadMessage.Call(uintptr(m.tid), wmQuit, 0, 0)
	<-m.done
}
