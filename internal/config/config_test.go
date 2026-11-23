package config

import (
	"os"
	"path/filepath"
	"testing"
)

func write(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestLoadTrimsTrailingSlashAndQuotes(t *testing.T) {
	p := write(t, "# comment\nNAVIDROME_URL=https://music.example.com/\nNAVIDROME_USER=\"bob\"\nNAVIDROME_PASSWORD=secret\n")
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.URL != "https://music.example.com" || c.User != "bob" || c.Password != "secret" {
		t.Fatalf("unexpected config: %+v", c)
	}
}

func TestLoadEnvOverridesFile(t *testing.T) {
	p := write(t, "NAVIDROME_URL=https://a\nNAVIDROME_USER=u\nNAVIDROME_PASSWORD=p\n")
	t.Setenv("NAVIDROME_USER", "override")
	c, err := Load(p)
	if err != nil || c.User != "override" {
		t.Fatalf("got %+v, %v", c, err)
	}
}

func TestLoadMissingValuesFails(t *testing.T) {
	t.Setenv("NAVIDROME_URL", "")
	t.Setenv("NAVIDROME_USER", "")
	t.Setenv("NAVIDROME_PASSWORD", "")
	if _, err := Load(filepath.Join(t.TempDir(), "nope.env")); err == nil {
		t.Fatal("expected error for missing settings")
	}
}
