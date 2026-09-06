package tui

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/liking008/gi-launcher-tui/internal/archive"
)

func TestJoinZipParts(t *testing.T) {
	dir := t.TempDir()
	part1 := filepath.Join(dir, "YuanShen_5.5.0.zip.001")
	part2 := filepath.Join(dir, "YuanShen_5.5.0.zip.002")
	os.WriteFile(part1, []byte("AAAA"), 0o644)
	os.WriteFile(part2, []byte("BBBB"), 0o644)

	out, err := joinZipParts([]string{part1, part2}, dir)
	if err != nil {
		t.Fatalf("joinZipParts: %v", err)
	}
	want := filepath.Join(dir, "YuanShen_5.5.0.zip")
	if out != want {
		t.Fatalf("expected %s, got %s", want, out)
	}
	data, _ := os.ReadFile(out)
	if string(data) != "AAAABBBB" {
		t.Fatalf("joined content wrong: %q", data)
	}
}

func TestInstallFullGameExtracts(t *testing.T) {
	// Build a small zip with a nested "YuanShen_5.5.0/" prefix.
	dir := t.TempDir()
	zipPath := filepath.Join(dir, "game.zip")
	zf, _ := os.Create(zipPath)
	zw := zip.NewWriter(zf)
	w, _ := zw.Create("YuanShen_5.5.0/pkg_version")
	w.Write([]byte("test"))
	zw.Close()
	zf.Close()

	installDir := filepath.Join(dir, "install")
	if err := archive.ExtractZip(zipPath, installDir, nil); err != nil {
		t.Fatalf("extract: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "pkg_version")); err != nil {
		t.Fatalf("pkg_version not extracted: %v", err)
	}
}
