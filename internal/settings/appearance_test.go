package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAppearanceNormalizeRejectsAnythingThatIsNotAHexColor(t *testing.T) {
	bad := []string{"", "red", "#12345", "#1234567", "#gg0000", "url(javascript:alert(1))", "#fff; background:red", "#2d7be0}</style>"}
	for _, b := range bad {
		a := Appearance{Accent: b, Surface: "graphite"}
		a.Normalize()
		if a.Accent != DefaultAccent {
			t.Errorf("accent %q must fall back to the default, got %q", b, a.Accent)
		}
	}
	a := Appearance{Accent: "  #8B5CF6 ", Surface: "warm"}
	a.Normalize()
	if a.Accent != "#8b5cf6" || a.Surface != "warm" {
		t.Fatalf("valid values must be kept (lowercased): %+v", a)
	}
}

func TestAppearanceUnknownSurfaceFallsBack(t *testing.T) {
	a := Appearance{Accent: DefaultAccent, Surface: "neon"}
	a.Normalize()
	if a.Surface != "graphite" {
		t.Fatalf("got %q", a.Surface)
	}
	for _, s := range Surfaces {
		b := Appearance{Accent: DefaultAccent, Surface: s}
		b.Normalize()
		if b.Surface != s {
			t.Fatalf("listed surface %q rejected", s)
		}
	}
}

func TestAppearanceRoundTripMissingAndCorruptFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "appearance.json")
	if a, err := LoadAppearance(path); err != nil || a != DefaultAppearance() {
		t.Fatalf("missing file: %+v, %v", a, err)
	}
	want := Appearance{Accent: "#22c55e", Surface: "midnight"}
	if err := SaveAppearance(path, want); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadAppearance(path); err != nil || got != want {
		t.Fatalf("round trip: %+v, %v", got, err)
	}
	_ = os.WriteFile(path, []byte("{oops"), 0o600)
	a, err := LoadAppearance(path)
	if err == nil || a != DefaultAppearance() {
		t.Fatalf("corrupt file must be reported with usable defaults: %+v, %v", a, err)
	}
}

func TestSaveAppearanceNeverWritesAnInvalidColor(t *testing.T) {
	path := filepath.Join(t.TempDir(), "appearance.json")
	if err := SaveAppearance(path, Appearance{Accent: "javascript:evil", Surface: "x"}); err != nil {
		t.Fatal(err)
	}
	got, _ := LoadAppearance(path)
	if got != DefaultAppearance() {
		t.Fatalf("got %+v", got)
	}
}
