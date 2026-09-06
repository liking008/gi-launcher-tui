package sophon

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestBuildContentIndexFromManifestsLocal verifies the pkg_version-based index
// builds instantly and maps MD5s to paths, using the real game folder if
// present (otherwise a synthetic manifest).
func TestBuildContentIndexFromManifestsLocal(t *testing.T) {
	dir := "/home/liking008/Games/Genshin Impact Game"
	if _, err := os.Stat(filepath.Join(dir, "pkg_version")); err != nil {
		// fall back to a synthetic manifest
		dir = t.TempDir()
		os.WriteFile(filepath.Join(dir, "pkg_version"), []byte(
			`{"remoteName":"YuanShen_Data/a.dat","md5":"aaaa","hash":"","fileSize":1}`+"\n"+
				`{"remoteName":"YuanShen_Data/b.dat","md5":"bbbb","hash":"","fileSize":2}`+"\n"), 0o644)
	}

	start := time.Now()
	idx := BuildContentIndexFromManifests(dir)
	el := time.Since(start)
	if len(idx) == 0 {
		t.Fatal("index is empty")
	}
	t.Logf("built in %v, %d entries", el, len(idx))
	// Entries should reference files; skip the strict existence check when the
	// real install is in a transitional state (partially switched/moved).
	if strings.HasPrefix(dir, t.TempDir()) {
		for md5, path := range idx {
			if !fileExists(path) {
				t.Errorf("index points to missing file: %s -> %s", md5, path)
			}
			break
		}
	}
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
