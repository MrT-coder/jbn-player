//go:build windows

package island

import (
	"path/filepath"
	"testing"

	"jbnplayer/internal/settings"
)

func TestAppearanceThroughTheService(t *testing.T) {
	s := &service{appearancePath: filepath.Join(t.TempDir(), "appearance.json")}
	if r := s.GetAppearance(); r.Error != "" || r.Accent != settings.DefaultAccent || len(r.Surfaces) != len(settings.Surfaces) {
		t.Fatalf("defaults: %+v", r)
	}
	r := s.SetAppearance(settings.Appearance{Accent: "#EC4899", Surface: "black"})
	if r.Error != "" || r.Accent != "#ec4899" || r.Surface != "black" {
		t.Fatalf("set: %+v", r)
	}
	if got := s.GetAppearance(); got.Accent != "#ec4899" || got.Surface != "black" {
		t.Fatalf("it must persist: %+v", got)
	}
	// Whatever the page sends, only a valid color can come back (it is used inside CSS).
	if r := s.SetAppearance(settings.Appearance{Accent: "red; x:y", Surface: "zzz"}); r.Accent != settings.DefaultAccent || r.Surface != "graphite" {
		t.Fatalf("invalid input must fall back: %+v", r)
	}
}
