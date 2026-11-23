package offline

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// The offline root is the folder that holds one subfolder per server. It is
// chosen by the user and kept in offline.json next to the other settings;
// without that file the default folder is used.

type rootFile struct {
	Dir string `json:"dir"`
}

func rootConfigPath() string {
	if p := os.Getenv("JBN_PLAYER_OFFLINE"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "jbn-player", "offline.json")
}

// DefaultRoot is where songs go until the user picks another folder.
func DefaultRoot() string {
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = os.TempDir()
	}
	return filepath.Join(cache, "jbn-player", "offline")
}

// LoadRoot returns the folder in use: the custom one if set, else the default.
func LoadRoot() string {
	raw, err := os.ReadFile(rootConfigPath())
	if err == nil {
		var f rootFile
		if json.Unmarshal(raw, &f) == nil && filepath.IsAbs(f.Dir) {
			return f.Dir
		}
	}
	return DefaultRoot()
}

// SaveRoot makes dir the offline root ("" goes back to the default). The folder
// must be absolute, and is created and checked for write access first. Songs
// already stored in the previous folder are left where they are.
func SaveRoot(dir string) error {
	if dir != "" {
		if !filepath.IsAbs(dir) {
			return errors.New("the folder must be an absolute path, like D:/Music/offline")
		}
		dir = filepath.Clean(dir)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return err
		}
		probe, err := os.CreateTemp(dir, ".write-test-*")
		if err != nil {
			return errors.New("cannot write to that folder: " + err.Error())
		}
		probe.Close()
		_ = os.Remove(probe.Name())
	}
	path := rootConfigPath()
	if dir == "" {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, _ := json.Marshal(rootFile{Dir: dir})
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
