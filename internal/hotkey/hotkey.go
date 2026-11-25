// Package hotkey is the global keyboard shortcut that opens the island window
// (useful in a game, where the tray icon cannot be clicked). This file is the
// portable part: parsing "Ctrl+Alt+J" and saving the choice. The Windows part
// registers it with RegisterHotKey, which also tells whether another
// application already uses the same combination.
package hotkey

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Modifier flags, with the values RegisterHotKey expects.
const (
	ModAlt   uint32 = 0x0001
	ModCtrl  uint32 = 0x0002
	ModShift uint32 = 0x0004
)

// Default is the shortcut of a fresh install.
const Default = "Ctrl+Alt+J"

// Combo is a shortcut: modifiers plus one virtual-key code. The zero value
// means "no shortcut" (turned off).
type Combo struct {
	Mods uint32
	Key  uint32 // virtual-key code: 'A'-'Z', '0'-'9', or VK_F1-VK_F12
}

// IsZero reports whether the shortcut is turned off.
func (c Combo) IsZero() bool { return c.Key == 0 }

// String gives the canonical spelling ("Ctrl+Alt+J"), or "" when off.
func (c Combo) String() string {
	if c.IsZero() {
		return ""
	}
	var parts []string
	if c.Mods&ModCtrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if c.Mods&ModAlt != 0 {
		parts = append(parts, "Alt")
	}
	if c.Mods&ModShift != 0 {
		parts = append(parts, "Shift")
	}
	return strings.Join(append(parts, keyName(c.Key)), "+")
}

const vkF1 = 0x70

func keyName(vk uint32) string {
	if vk >= vkF1 && vk < vkF1+12 {
		return "F" + strconv.Itoa(int(vk-vkF1+1))
	}
	return string(rune(vk))
}

// keyCode maps a key name to its virtual-key code (0 = not allowed).
func keyCode(name string) uint32 {
	if len(name) == 1 && (name[0] >= 'A' && name[0] <= 'Z' || name[0] >= '0' && name[0] <= '9') {
		return uint32(name[0]) // letters and digits are their own VK codes
	}
	if n, err := strconv.Atoi(strings.TrimPrefix(name, "F")); err == nil && strings.HasPrefix(name, "F") && n >= 1 && n <= 12 {
		return vkF1 + uint32(n-1)
	}
	return 0
}

// Parse reads "Ctrl+Alt+J" (any order, any case). Blank means off. A shortcut
// needs Ctrl or Alt, so it can never get in the way of plain typing; the Win
// key is refused because Windows reserves its combinations.
func Parse(s string) (Combo, error) {
	if strings.TrimSpace(s) == "" {
		return Combo{}, nil
	}
	var c Combo
	for _, raw := range strings.Split(s, "+") {
		p := strings.ToUpper(strings.TrimSpace(raw))
		var mod uint32
		switch p {
		case "":
			return Combo{}, fmt.Errorf("%q is not a valid shortcut", s)
		case "CTRL", "CONTROL":
			mod = ModCtrl
		case "ALT":
			mod = ModAlt
		case "SHIFT":
			mod = ModShift
		case "WIN", "WINDOWS", "META", "SUPER":
			return Combo{}, errors.New("shortcuts with the Windows key belong to Windows; use Ctrl or Alt")
		default:
			vk := keyCode(p)
			if vk == 0 {
				return Combo{}, fmt.Errorf("%q cannot be used; use a letter, a digit or F1-F12", strings.TrimSpace(raw))
			}
			if c.Key != 0 {
				return Combo{}, errors.New("a shortcut has only one key besides Ctrl, Alt and Shift")
			}
			c.Key = vk
			continue
		}
		if c.Mods&mod != 0 {
			return Combo{}, fmt.Errorf("%q repeats a key", s)
		}
		c.Mods |= mod
	}
	if c.Key == 0 {
		return Combo{}, errors.New("a shortcut needs a letter, a digit or F1-F12")
	}
	if c.Mods&(ModCtrl|ModAlt) == 0 {
		return Combo{}, errors.New("a shortcut needs Ctrl or Alt")
	}
	return c, nil
}

// DefaultPath is where the choice is saved (next to settings.json). Only the
// player process writes it.
func DefaultPath() (string, error) {
	if p := os.Getenv("JBN_PLAYER_HOTKEY"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "jbn-player", "hotkey.json"), nil
}

type file struct {
	Hotkey string `json:"hotkey"` // "" = turned off
}

func mustDefault() Combo {
	c, _ := Parse(Default)
	return c
}

// Load reads the saved shortcut. A missing file gives Default; a corrupt or
// invalid one gives Default plus the error.
func Load(path string) (Combo, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return mustDefault(), nil
	}
	if err != nil {
		return mustDefault(), err
	}
	var f file
	if err := json.Unmarshal(raw, &f); err != nil {
		return mustDefault(), fmt.Errorf("parse %s: %w", path, err)
	}
	c, err := Parse(f.Hotkey)
	if err != nil {
		return mustDefault(), fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

// Save writes the shortcut atomically (the zero Combo saves "off").
func Save(path string, c Combo) error {
	raw, err := json.MarshalIndent(file{Hotkey: c.String()}, "", "  ")
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

// Status is what the window shows.
type Status struct {
	Hotkey  string `json:"hotkey"`  // "" = off
	Active  bool   `json:"active"`  // registered with Windows right now
	Default string `json:"default"` // Default, for the "reset" button
}
