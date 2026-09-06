package patch

import (
	"archive/zip"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// tools returns cached hpatchz and hdiffz binaries, downloading the official
// release zip once.
func tools(t *testing.T) (hpatchz, hdiffz string) {
	t.Helper()
	dir := t.TempDir()
	if err := downloadReleaseZip(t, dir); err != nil {
		t.Fatalf("download hdiffpatch: %v", err)
	}
	return filepath.Join(dir, "hpatchz"), filepath.Join(dir, "hdiffz")
}

func downloadReleaseZip(t *testing.T, dir string) error {
	t.Helper()
	resp, err := http.Get(hpatchzURL())
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return errNotOK
	}
	tmp, err := os.CreateTemp(dir, "release-*.zip")
	if err != nil {
		return err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		return err
	}
	tmp.Close()
	zr, err := zip.OpenReader(tmp.Name())
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		name := filepath.Base(zf.Name)
		if name != "hpatchz" && name != "hdiffz" {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(filepath.Join(dir, name), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		rc.Close()
		cerr := out.Close()
		if err != nil {
			return err
		}
		if cerr != nil {
			return cerr
		}
	}
	return nil
}

var errNotOK = &errStatus{}

type errStatus struct{}

func (*errStatus) Error() string { return "non-200 from hdiffpatch release" }

// TestApplyPatchZipEndToEnd builds a delta with hdiffz, wraps it in a
// miHoYo-style patch zip, and applies it with hpatchz.
func TestApplyPatchZipEndToEnd(t *testing.T) {
	if _, err := exec.LookPath("gcc"); err != nil && !hasNetwork() {
		t.Skip("no gcc and no network for hdiffpatch binary")
	}
	hp, hd := tools(t)
	work := t.TempDir()

	oldData := strings.Repeat("old-content-v1-", 2000)
	newData := strings.Repeat("old-content-v1-", 1800) + "CHANGED-V2-" + strings.Repeat("NEW-DATA-", 300)
	oldFile := filepath.Join(work, "old.pck")
	newFile := filepath.Join(work, "new.pck")
	os.WriteFile(oldFile, []byte(oldData), 0o644)
	os.WriteFile(newFile, []byte(newData), 0o644)

	diffFile := filepath.Join(work, "diff.hdiff")
	if out, err := exec.Command(hd, "-f", "-s-64", oldFile, newFile, diffFile).CombinedOutput(); err != nil {
		t.Fatalf("hdiffz: %v (%s)", err, out)
	}

	installDir := filepath.Join(work, "install")
	os.MkdirAll(filepath.Join(installDir, "YuanShen_Data"), 0o755)
	os.WriteFile(filepath.Join(installDir, "YuanShen_Data", "old.pck"), []byte(oldData), 0o644)

	patchZip := filepath.Join(work, "game_x_y_hdiff.zip")
	pzf, _ := os.Create(patchZip)
	zw := zip.NewWriter(pzf)
	addFile(t, zw, "pkg_version", "fake-new-manifest\n")
	addFileFrom(t, zw, "YuanShen_Data/old.pck.hdiff", diffFile)
	zw.Close()
	pzf.Close()

	newManifest, err := ApplyPatchZip(installDir, patchZip, hp, work)
	if err != nil {
		t.Fatalf("ApplyPatchZip: %v", err)
	}
	if newManifest != "pkg_version" {
		t.Fatalf("expected pkg_version manifest, got %q", newManifest)
	}
	got, _ := os.ReadFile(filepath.Join(installDir, "YuanShen_Data", "old.pck"))
	if string(got) != newData {
		t.Fatalf("patched content mismatch:\n got %d bytes\nwant %d bytes", len(got), len(newData))
	}
}

func hasNetwork() bool {
	return true
}

func addFile(t *testing.T, zw *zip.Writer, name, content string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	w.Write([]byte(content))
}

func addFileFrom(t *testing.T, zw *zip.Writer, name, src string) {
	t.Helper()
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}
	w, err := zw.Create(name)
	if err != nil {
		t.Fatal(err)
	}
	w.Write(data)
}
