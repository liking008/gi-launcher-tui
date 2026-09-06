package patch

import (
	"archive/zip"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// The HDiffPatch command-line tool (hpatchz) is a C/C++ binary with no pure-Go
// equivalent. We download the official Linux build on first use and shell out
// to it to apply each .hdiff delta. The URL can be overridden via
// GIH_LANCHER_HPATCHZ_URL.
const hpatchzVersion = "v5.1.3"

func hpatchzURL() string {
	if u := os.Getenv("GIH_LANCHER_HPATCHZ_URL"); u != "" {
		return u
	}
	osName := "linux64"
	if runtime.GOOS == "windows" {
		osName = "windows64"
	}
	return fmt.Sprintf("https://github.com/sisong/HDiffPatch/releases/download/%s/hdiffpatch_%s_bin_%s.zip",
		hpatchzVersion, hpatchzVersion, osName)
}

// HpatchzPath returns the path to the hpatchz binary, downloading it into dir
// if it is not already cached.
func HpatchzPath(dir string) (string, error) {
	exe := "hpatchz"
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	path := filepath.Join(dir, exe)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(dir, "hpatchz-*.zip")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	tmp.Close()
	defer os.Remove(tmpName)

	resp, err := http.Get(hpatchzURL())
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("下载 hpatchz 失败: %s", resp.Status)
	}
	f, err := os.Create(tmpName)
	if err != nil {
		return "", err
	}
	_, err = io.Copy(f, resp.Body)
	f.Close()
	if err != nil {
		return "", err
	}
	if err := extractExe(tmpName, dir, exe); err != nil {
		return "", err
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("未找到 hpatchz: %v", err)
	}
	return path, nil
}

// extractExe pulls a single file out of the release zip.
func extractExe(zipPath, dir, want string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		if filepath.Base(zf.Name) != want {
			continue
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		defer rc.Close()
		out, err := os.OpenFile(filepath.Join(dir, want), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, rc)
		cerr := out.Close()
		if err != nil {
			return err
		}
		return cerr
	}
	return fmt.Errorf("压缩包中未找到 %s", want)
}

// ApplyFile runs `hpatchz oldPath diffPath outPath` to apply one delta.
func ApplyFile(hpatchz, oldPath, diffPath, outPath string) error {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return err
	}
	cmd := exec.Command(hpatchz, "-f", oldPath, diffPath, outPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("hpatchz: %v (%s)", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// NewManifestName is the pkg_version manifest inside a patch zip.
const NewManifestName = "pkg_version"

// ApplyPatchZip extracts a miHoYo incremental patch zip and applies it into
// installDir. Files ending in .hdiff are applied as deltas against the existing
// install; full files are copied; deletefiles.txt (when present) removes
// obsolete files. workDir is the base directory used for the temporary staging
// folder (kept under the cache, not /tmp). On success the new pkg_version
// manifest is returned.
func ApplyPatchZip(installDir, patchZip, hpatchzBin, workDir string) (string, error) {
	staging, err := os.MkdirTemp(workDir, "gi-patch-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)

	if err := extractZip(patchZip, staging); err != nil {
		return "", err
	}

	// 1) delete obsolete files listed in deletefiles.txt (game patch only).
	if err := applyDeletes(installDir, filepath.Join(staging, "deletefiles.txt")); err != nil {
		return "", err
	}

	// 2) walk the staging tree, patching or copying each file.
	err = filepath.Walk(staging, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(staging, p)
		switch rel {
		case "pkg_version", "hdifffiles.txt", "deletefiles.txt":
			return nil // handled separately
		}
		if strings.HasPrefix(rel, "Audio_") && strings.HasSuffix(rel, "_pkg_version") {
			return nil // audio manifest, copied at the end
		}
		target := filepath.Join(installDir, filepath.FromSlash(rel))
		if strings.HasSuffix(rel, ".hdiff") {
			// delta against the existing file
			oldPath := target[:len(target)-len(".hdiff")]
			out := target + ".new"
			if err := ApplyFile(hpatchzBin, oldPath, p, out); err != nil {
				return err
			}
			return os.Rename(out, oldPath)
		}
		// new full file
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return copyFile(p, target)
	})
	if err != nil {
		return "", err
	}

	// 3) copy the new game manifest into the install dir.
	nm := filepath.Join(staging, NewManifestName)
	newManifest := ""
	if st, err := os.Stat(nm); err == nil && !st.IsDir() {
		if err := copyFile(nm, filepath.Join(installDir, NewManifestName)); err != nil {
			return "", err
		}
		newManifest = NewManifestName
	}
	// 4) copy audio manifests.
	audioManifests, err := filepath.Glob(filepath.Join(staging, "Audio_*_pkg_version"))
	if err == nil {
		for _, am := range audioManifests {
			if err := copyFile(am, filepath.Join(installDir, filepath.Base(am))); err != nil {
				return "", err
			}
		}
	}
	return newManifest, nil
}

// applyDeletes removes files listed (one per line) in the delete manifest.
func applyDeletes(installDir, delFile string) error {
	data, err := os.ReadFile(delFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		_ = os.Remove(filepath.Join(installDir, filepath.FromSlash(line)))
	}
	return nil
}

func extractZip(zipPath, dest string) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()
	for _, zf := range zr.File {
		rel := strings.TrimPrefix(zf.Name, "/")
		target := filepath.Join(dest, filepath.FromSlash(rel))
		if zf.FileInfo().IsDir() {
			os.MkdirAll(target, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := zf.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
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

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, err = io.Copy(out, in)
	cerr := out.Close()
	if err != nil {
		return err
	}
	return cerr
}
