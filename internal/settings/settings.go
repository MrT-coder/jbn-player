// Package settings holds the player's audio preferences: volume, output
// device and equalizer. They live in %APPDATA%\jbn-player\settings.json.
package settings

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Bands are the equalizer's center frequencies in Hz (octave spacing).
var Bands = [10]float64{31, 62, 125, 250, 500, 1000, 2000, 4000, 8000, 16000}

const (
	// MaxGain is the equalizer limit in dB, up and down.
	MaxGain = 12.0
	// AutoDevice follows the system default output device.
	AutoDevice = "auto"
)

// PresetOrder is how presets are listed; Presets holds their gains in dB per band.
var PresetOrder = []string{"flat", "bass", "treble", "vocal", "rock", "pop", "dance"}

// Presets maps a preset name to its gains in dB per band.
var Presets = map[string][10]float64{
	"flat":   {0, 0, 0, 0, 0, 0, 0, 0, 0, 0},
	"bass":   {6, 5, 4, 2, 0, 0, 0, 0, 0, 0},
	"treble": {0, 0, 0, 0, 0, 1, 2, 4, 5, 6},
	"vocal":  {-2, -2, -1, 1, 3, 4, 3, 1, 0, -1},
	"rock":   {5, 4, 2, -1, -2, -1, 2, 4, 5, 5},
	"pop":    {-1, 1, 3, 4, 3, 0, -1, -1, 1, 2},
	"dance":  {6, 5, 3, 0, 1, 3, 4, 3, 2, 0},
}

// EQ is the equalizer state.
type EQ struct {
	Enabled bool        `json:"enabled"`
	Preset  string      `json:"preset"` // a Presets key, or "custom"
	Gains   [10]float64 `json:"gains"`
}

// Settings is everything that is saved.
type Settings struct {
	Volume      int    `json:"volume"` // 0-100: the player's own gain, the Windows volume is untouched
	AudioDevice string `json:"audioDevice"`
	EQ          EQ     `json:"eq"`
}

// Default is what a fresh install uses.
func Default() Settings {
	return Settings{Volume: 100, AudioDevice: AutoDevice, EQ: EQ{Preset: "flat"}}
}

// Normalize clamps values to what the player accepts.
func (s *Settings) Normalize() {
	if s.Volume < 0 {
		s.Volume = 0
	}
	if s.Volume > 100 {
		s.Volume = 100
	}
	if strings.TrimSpace(s.AudioDevice) == "" {
		s.AudioDevice = AutoDevice
	}
	s.EQ.Normalize()
}

// Normalize clamps the gains and fills in the preset name.
func (e *EQ) Normalize() {
	for i, g := range e.Gains {
		if g > MaxGain {
			e.Gains[i] = MaxGain
		}
		if g < -MaxGain {
			e.Gains[i] = -MaxGain
		}
	}
	if e.Preset == "" {
		e.Preset = "custom"
	}
}

// Filter returns the mpv audio filter (the "af" value) for this equalizer, or
// "" when it is off or flat: no filter at all means zero extra CPU.
func (e EQ) Filter() string {
	if !e.Enabled {
		return ""
	}
	e.Normalize()
	var parts []string
	for i, g := range e.Gains {
		if g > -0.05 && g < 0.05 {
			continue
		}
		parts = append(parts, fmt.Sprintf("equalizer=f=%g:t=o:w=1:g=%.1f", Bands[i], g))
	}
	if len(parts) == 0 {
		return ""
	}
	return "lavfi=[" + strings.Join(parts, ",") + "]"
}

// DefaultPath is where the settings file lives. JBN_PLAYER_SETTINGS overrides
// it (used by tests so they never touch the real settings).
func DefaultPath() (string, error) {
	if p := os.Getenv("JBN_PLAYER_SETTINGS"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jbn-player", "settings.json"), nil
}

// Load reads the file; a missing file gives the defaults. A corrupt file is
// reported, together with usable defaults.
func Load(path string) (Settings, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	}
	if err != nil {
		return Default(), err
	}
	s := Default()
	if err := json.Unmarshal(raw, &s); err != nil {
		return Default(), fmt.Errorf("parse %s: %w", path, err)
	}
	s.Normalize()
	return s, nil
}

// Save writes the file atomically.
func Save(path string, s Settings) error {
	raw, err := json.MarshalIndent(s, "", "  ")
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
