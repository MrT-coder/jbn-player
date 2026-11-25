//go:build windows

package hotkey

import (
	"errors"
	"testing"
)

// An unusual combination, so the test does not depend on what this PC runs.
const testCombo = "Ctrl+Alt+Shift+F11"

func TestSecondRegistrationReportsInUse(t *testing.T) {
	c, _ := Parse(testCombo)
	a := Start(func() {})
	defer a.Stop()
	if err := a.Set(c); err != nil {
		t.Skipf("%s is taken on this machine: %v", testCombo, err)
	}
	if st := a.Status(); st.Hotkey != testCombo || !st.Active {
		t.Fatalf("a: %+v", st)
	}

	b := Start(func() {})
	defer b.Stop()
	if err := b.Set(c); !errors.Is(err, ErrInUse) {
		t.Fatalf("b.Set = %v, want ErrInUse", err)
	}
	if st := b.Status(); st.Active || st.Hotkey != "" {
		t.Fatalf("a failed Set must not change b: %+v", st)
	}

	// Suspending a frees the combination (the window does it while recording a new one).
	a.Suspend()
	if err := b.Set(c); err != nil {
		t.Fatalf("b.Set after a.Suspend: %v", err)
	}
	a.Resume() // b holds it now: a stays inactive but keeps its choice
	if st := a.Status(); st.Active || st.Hotkey != testCombo {
		t.Fatalf("a after resume: %+v", st)
	}
	b.Stop()
	a.Resume()
	if st := a.Status(); !st.Active {
		t.Fatalf("a must take it back once free: %+v", st)
	}
}

func TestFailedSetKeepsThePreviousShortcut(t *testing.T) {
	c, _ := Parse(testCombo)
	holder := Start(func() {})
	defer holder.Stop()
	if err := holder.Set(c); err != nil {
		t.Skipf("%s is taken on this machine: %v", testCombo, err)
	}

	m := Start(func() {})
	defer m.Stop()
	old, _ := Parse("Ctrl+Alt+Shift+F10")
	if err := m.Set(old); err != nil {
		t.Skipf("Ctrl+Alt+Shift+F10 is taken on this machine: %v", err)
	}
	if err := m.Set(c); !errors.Is(err, ErrInUse) {
		t.Fatalf("Set = %v, want ErrInUse", err)
	}
	if st := m.Status(); st.Hotkey != old.String() || !st.Active {
		t.Fatalf("previous shortcut lost: %+v", st)
	}
	if err := m.Set(Combo{}); err != nil { // off
		t.Fatal(err)
	}
	if st := m.Status(); st.Active || st.Hotkey != "" {
		t.Fatalf("off: %+v", st)
	}
}
