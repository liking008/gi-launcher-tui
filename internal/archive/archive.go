package archive

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// OnEntry is called for each file as it is extracted (relative path, size).
type OnEntry func(rel string, size int64)

// ExtractZip extracts a zip archive into dest. It strips leading path
// components so that files land directly under dest. Entries whose name
// contains "ScatteredFiles" or a version subfolder are flattened.
func ExtractZip(zipPath, dest string, onEntry OnEntry) error {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer zr.Close()

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}
	for _, zf := range zr.File {
		rel := sanitize(zf.Name)
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}
		target := filepath.Join(dest, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		if err := extractFile(zf, target); err != nil {
			return fmt.Errorf("%s: %w", zf.Name, err)
		}
		if onEntry != nil {
			onEntry(rel, int64(zf.UncompressedSize64))
		}
	}
	return nil
}

func extractFile(zf *zip.File, target string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	// zips from miHoYo can use deflate; ensure a plain writer.
	mode := os.FileMode(0o644)
	if zf.Mode().IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	if zf.Mode()&0o111 != 0 {
		mode = 0o755
	}
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
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

// sanitize flattens miHoYo package layout: "YuanShen_4.0.0/..." and
// ".../ScatteredFiles/..." are unwrapped to their tail.
func sanitize(name string) string {
	name = strings.ReplaceAll(name, "\\", "/")
	name = strings.TrimPrefix(name, "/")
	parts := strings.Split(name, "/")

	// If it's the standalone single-file zip, keep the basename.
	// Otherwise find the first meaningful subtree after a version/ScatteredFiles marker.
	for i, p := range parts {
		if strings.HasPrefix(p, "YuanShen_") || strings.HasPrefix(p, "GenshinImpact_") {
			rest := parts[i+1:]
			if len(rest) == 0 {
				return parts[len(parts)-1]
			}
			return strings.Join(rest, "/")
		}
	}
	// ScatteredFiles folder: drop it.
	kept := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "ScatteredFiles" {
			continue
		}
		kept = append(kept, p)
	}
	return strings.Join(kept, "/")
}
