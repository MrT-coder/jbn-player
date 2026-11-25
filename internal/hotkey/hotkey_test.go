package hotkey

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseAcceptsAndNormalizes(t *testing.T) {
	cases := map[string]string{
		"Ctrl+Alt+J":         "Ctrl+Alt+J",
		" alt + ctrl + j ":   "Ctrl+Alt+J",
		"control+shift+F5":   "Ctrl+Shift+F5",
		"Alt+7":              "Alt+7",
		"shift+alt+ctrl+f12": "Ctrl+Alt+Shift+F12",
	}
	for in, want := range cases {
		c, err := Parse(in)
		if err != nil {
			t.Fatalf("Parse(%q): %v", in, err)
		}
		if got := c.String(); got != want {
			t.Fatalf("Parse(%q).String() = %q, want %q", in, got, want)
		}
	}
}

func TestParseEmptyMeansOff(t *testing.T) {
	c, err := Parse("  ")
	if err != nil || !c.IsZero() || c.String() != "" {
		t.Fatalf("Parse(blank) = %+v, %v", c, err)
	}
}

func TestParseRejects(t *testing.T) {
	for _, in := range []string{
		"J",            // no modifier
		"Shift+J",      // would break typing
		"Win+L",        // Windows owns the Win key shortcuts
		"Ctrl+Alt",     // no key
		"Ctrl+J+K",     // two keys
		"Ctrl+Ctrl+J",  // repeated modifier
		"Ctrl+Alt+Del", // not an allowed key
		"Ctrl+F13",     // out of range
		"Ctrl++J",      // empty part
	} {
		if c, err := Parse(in); err == nil {
			t.Fatalf("Parse(%q) = %v, want an error", in, c)
		}
	}
}

func TestParseVirtualKeys(t *testing.T) {
	c, _ := Parse("Ctrl+Alt+J")
	if c.Mods != ModCtrl|ModAlt || c.Key != 'J' {
		t.Fatalf("Ctrl+Alt+J = %+v", c)
	}
	f, _ := Parse("Alt+F1")
	if f.Key != 0x70 {
		t.Fatalf("F1 vk = %#x", f.Key)
	}
}

func TestLoadSave(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "hotkey.json")

	c, err := Load(path)
	if err != nil || c.String() != Default {
		t.Fatalf("missing file: %v, %v (want the default %s)", c, err, Default)
	}

	alt, _ := Parse("Ctrl+Shift+K")
	if err := Save(path, alt); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(path); err != nil || c != alt {
		t.Fatalf("after save: %v, %v", c, err)
	}

	if err := Save(path, Combo{}); err != nil { // turned off
		t.Fatal(err)
	}
	if c, err := Load(path); err != nil || !c.IsZero() {
		t.Fatalf("off must stay off, got %v, %v", c, err)
	}

	if err := os.WriteFile(path, []byte(`{"hotkey":"Win+L"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if c, err := Load(path); err == nil || c.String() != Default {
		t.Fatalf("invalid file: %v, %v (want the default and an error)", c, err)
	}
}
