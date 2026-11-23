package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Appearance is the look of the window: the accent color and the tone of the
// surfaces. It is kept in its own file (appearance.json) because the window
// process writes it while the player process owns settings.json.
type Appearance struct {
	Accent  string `json:"accent"`  // "#rrggbb"
	Surface string `json:"surface"` // one of Surfaces
}

// Surfaces are the available background tones.
var Surfaces = []string{"graphite", "midnight", "black", "warm"}

// DefaultAccent is the blue the app started with.
const DefaultAccent = "#2d7be0"

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// DefaultAppearance is what a fresh install uses.
func DefaultAppearance() Appearance { return Appearance{Accent: DefaultAccent, Surface: "graphite"} }

// Normalize replaces anything invalid with the default, so a bad value can
// never reach the page (it ends up inside CSS).
func (a *Appearance) Normalize() {
	a.Accent = strings.ToLower(strings.TrimSpace(a.Accent))
	if !hexColor.MatchString(a.Accent) {
		a.Accent = DefaultAccent
	}
	ok := false
	for _, s := range Surfaces {
		if a.Surface == s {
			ok = true
		}
	}
	if !ok {
		a.Surface = "graphite"
	}
}

// AppearancePath is where appearance.json lives (next to settings.json).
func AppearancePath() (string, error) {
	if p := os.Getenv("JBN_PLAYER_APPEARANCE"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jbn-player", "appearance.json"), nil
}

// LoadAppearance reads the file; a missing or corrupt file gives the defaults.
func LoadAppearance(path string) (Appearance, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultAppearance(), nil
	}
	if err != nil {
		return DefaultAppearance(), err
	}
	a := DefaultAppearance()
	if err := json.Unmarshal(raw, &a); err != nil {
		return DefaultAppearance(), fmt.Errorf("parse %s: %w", path, err)
	}
	a.Normalize()
	return a, nil
}

// SaveAppearance writes the file atomically.
func SaveAppearance(path string, a Appearance) error {
	a.Normalize()
	raw, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
