//go:build windows

package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTripEncryptsPasswordOnDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sub", "servers.json")
	in := Data{
		Servers: []Server{{ID: "a", Type: TypeSubsonic, Name: "Home", URL: "https://x.example", User: "bob", Password: "s3cret-pass"}},
		Active:  "a",
	}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "s3cret-pass") {
		t.Fatal("password must not be stored in plaintext")
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Servers) != 1 || out.Servers[0] != in.Servers[0] || out.Active != "a" {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}

func TestLoadMissingFileIsEmpty(t *testing.T) {
	d, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil || len(d.Servers) != 0 {
		t.Fatalf("got %+v, %v", d, err)
	}
}

func TestActiveServerFallsBackToFirst(t *testing.T) {
	d := Data{Servers: []Server{{ID: "a"}, {ID: "b"}}, Active: "zzz"}
	if s, ok := d.ActiveServer(); !ok || s.ID != "a" {
		t.Fatalf("got %+v %v", s, ok)
	}
	d.Active = "b"
	if s, _ := d.ActiveServer(); s.ID != "b" {
		t.Fatal("must honor Active")
	}
	if _, ok := (Data{}).ActiveServer(); ok {
		t.Fatal("empty data has no active server")
	}
}

func TestSongkeyTokenIsEncryptedAndOptional(t *testing.T) {
	path := filepath.Join(t.TempDir(), "servers.json")
	in := Data{Servers: []Server{{ID: "a", URL: "https://x"}}, Active: "a",
		Songkey: Songkey{URL: "https://songkey.example", Token: "tok-very-secret"}}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	if strings.Contains(string(raw), "tok-very-secret") || !strings.Contains(string(raw), "https://songkey.example") {
		t.Fatalf("token must be encrypted, URL readable: %s", raw)
	}
	out, err := Load(path)
	if err != nil || out.Songkey != in.Songkey || out.Servers[0].URL != "https://x" {
		t.Fatalf("round trip: %+v %v", out, err)
	}
	// Without songkey the file keeps its old shape (no empty "songkey" block).
	if err := Save(path, Data{Servers: in.Servers, Active: "a"}); err != nil {
		t.Fatal(err)
	}
	raw, _ = os.ReadFile(path)
	if strings.Contains(string(raw), "songkey") {
		t.Fatalf("unexpected songkey block: %s", raw)
	}
	if out, _ := Load(path); out.Songkey.Configured() {
		t.Fatal("not configured")
	}
}
