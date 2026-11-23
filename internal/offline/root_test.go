package offline

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRootDefaultsThenCustomThenReset(t *testing.T) {
	t.Setenv("JBN_PLAYER_OFFLINE", filepath.Join(t.TempDir(), "offline.json"))
	def := DefaultRoot()
	if got := LoadRoot(); got != def {
		t.Fatalf("no config: LoadRoot = %q, want default %q", got, def)
	}

	custom := filepath.Join(t.TempDir(), "my music")
	if err := SaveRoot(custom); err != nil {
		t.Fatal(err)
	}
	if got := LoadRoot(); got != custom {
		t.Fatalf("LoadRoot = %q, want %q", got, custom)
	}
	if _, err := os.Stat(custom); err != nil {
		t.Fatalf("SaveRoot must create the folder: %v", err)
	}

	if err := SaveRoot(""); err != nil { // empty = back to the default
		t.Fatal(err)
	}
	if got := LoadRoot(); got != def {
		t.Fatalf("after reset LoadRoot = %q, want %q", got, def)
	}
}

func TestSaveRootRejectsBadFolders(t *testing.T) {
	t.Setenv("JBN_PLAYER_OFFLINE", filepath.Join(t.TempDir(), "offline.json"))
	file := filepath.Join(t.TempDir(), "afile")
	_ = os.WriteFile(file, []byte("x"), 0o600)
	for _, bad := range []string{"relative/folder", file} {
		if err := SaveRoot(bad); err == nil {
			t.Fatalf("SaveRoot(%q) should fail", bad)
		}
	}
	if got := LoadRoot(); got != DefaultRoot() {
		t.Fatalf("a rejected folder must not be saved, got %q", got)
	}
}

func TestDirIsPerServerUnderRoot(t *testing.T) {
	if got := Dir(`C:\music`, "srv1"); got != filepath.Join(`C:\music`, "srv1") {
		t.Fatalf("Dir = %q", got)
	}
}
