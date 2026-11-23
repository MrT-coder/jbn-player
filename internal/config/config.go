// Package config loads the Navidrome connection settings from a .env file
// and/or process environment variables (the environment wins).
package config

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

// Config holds the Navidrome connection settings.
type Config struct {
	URL      string // base URL without trailing slash
	User     string
	Password string
}

// Load reads path (if it exists) and overlays the process environment.
func Load(path string) (Config, error) {
	vals := map[string]string{}
	if f, err := os.Open(path); err == nil {
		defer f.Close()
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			if k, v, ok := strings.Cut(line, "="); ok {
				vals[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), "\"'")
			}
		}
	}
	for _, k := range []string{"NAVIDROME_URL", "NAVIDROME_USER", "NAVIDROME_PASSWORD"} {
		if v := os.Getenv(k); v != "" {
			vals[k] = v
		}
	}
	c := Config{
		URL:      strings.TrimRight(vals["NAVIDROME_URL"], "/"),
		User:     vals["NAVIDROME_USER"],
		Password: vals["NAVIDROME_PASSWORD"],
	}
	if c.URL == "" || c.User == "" || c.Password == "" {
		return c, errors.New("NAVIDROME_URL, NAVIDROME_USER and NAVIDROME_PASSWORD are required")
	}
	return c, nil
}
