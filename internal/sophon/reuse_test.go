package sophon

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
)

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

// pbLen encodes a length-delimited field.
func pbBytes(field int, s string) []byte {
	out := []byte{byte(field<<3 | 2), byte(len(s))}
	return append(out, s...)
}

// pbVarint encodes a varint field.
func pbVarint(field int, val uint64) []byte {
	var out []byte
	out = append(out, byte(field<<3|0))
	for val >= 0x80 {
		out = append(out, byte(val&0x7f)|0x80)
		val >>= 7
	}
	out = append(out, byte(val))
	return out
}

// buildManifest wraps each asset message as a repeated field-1 entry.
func buildManifest(t *testing.T, assetMsgs [][]byte) []byte {
	t.Helper()
	var out []byte
	for _, a := range assetMsgs {
		out = append(out, pbBytes(1, string(a))...)
	}
	return out
}

// buildAsset encodes an asset message with path, chunk entries and hash_md5.
func buildAsset(path string, chunkFields []byte, hash string, size uint64) []byte {
	asset := pbBytes(1, path) // path
	if chunkFields != nil {   // chunks (field 2, length-delimited)
		asset = append(asset, pbBytes(2, string(chunkFields))...)
	}
	asset = append(asset, pbVarint(3, 0)...) // type = file
	asset = append(asset, pbVarint(4, size)...)
	asset = append(asset, pbBytes(5, hash)...) // hash_md5
	return asset
}

func TestDownloadAssetsReuseAndSkip(t *testing.T) {
	contentA := "shared-content"
	contentB := "new-content-needs-download"

	// Existing CN install has a file with contentA.
	installDir := t.TempDir()
	cnPath := filepath.Join(installDir, "YuanShen_Data", "StreamingAssets", "foo.dat")
	os.MkdirAll(filepath.Dir(cnPath), 0o755)
	os.WriteFile(cnPath, []byte(contentA), 0o644)

	// The OS asset points at a DIFFERENT path but same content -> reuse.
	osFoo := "GenshinImpact_Data/StreamingAssets/foo.dat"
	osBar := "GenshinImpact_Data/StreamingAssets/bar.dat"

	// Chunk for bar (only bar actually needs a download).
	chunkBar := pbBytes(1, "c_bar")
	chunkBar = append(chunkBar, pbVarint(3, 0)...)
	chunkBar = append(chunkBar, pbVarint(4, uint64(len(contentB)))...)
	chunkBar = append(chunkBar, pbVarint(5, uint64(len(contentB)))...)

	assetFoo := buildAsset(osFoo, nil, md5Hex(contentA), uint64(len(contentA)))
	assetBar := buildAsset(osBar, chunkBar, md5Hex(contentB), uint64(len(contentB)))

	manifestData := buildManifest(t, [][]byte{assetFoo, assetBar})

	var chunkHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest_1" {
			w.Write(manifestData)
			return
		}
		if r.URL.Path == "/c_bar" {
			chunkHits.Add(1)
			w.Write([]byte(contentB))
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()

	comp := &Component{
		Manifest:         ManifestInfo{ID: "manifest_1", CompressedSize: "0", UncompressedSize: "0"},
		ManifestDownload: DownloadConfig{URLPrefix: srv.URL, Compression: 0},
		ChunkDownload:    DownloadConfig{URLPrefix: srv.URL, Compression: 0},
	}

	_, err := DownloadAssets(context.Background(), srv.Client(), comp, installDir,
		Options{ReuseContent: true})
	if err != nil {
		t.Fatalf("DownloadAssets: %v", err)
	}

	// foo (shared content) was reused -> copied, no chunk download.
	fooOut := filepath.Join(installDir, filepath.FromSlash(osFoo))
	data, err := os.ReadFile(fooOut)
	if err != nil {
		t.Fatalf("foo not copied: %v", err)
	}
	if string(data) != contentA {
		t.Fatalf("foo content wrong: %q", data)
	}
	// bar needed a real chunk download (exactly one).
	barOut := filepath.Join(installDir, filepath.FromSlash(osBar))
	data, err = os.ReadFile(barOut)
	if err != nil {
		t.Fatalf("bar not downloaded: %v", err)
	}
	if string(data) != contentB {
		t.Fatalf("bar content wrong: %q", data)
	}
	if got := chunkHits.Load(); got != 1 {
		t.Fatalf("expected 1 chunk download (only bar), got %d", got)
	}
}

func TestVerifyAssets(t *testing.T) {
	content := "verify-content"
	manifest := buildManifest(t, [][]byte{
		buildAsset("GenshinImpact_Data/a.dat", nil, md5Hex(content), uint64(len(content))), // file, no chunks, hash set
		buildAsset("GenshinImpact_Data/b.dat", nil, "deadbeef", uint64(len(content))),      // missing
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/m1" {
			w.Write(manifest)
			return
		}
		w.WriteHeader(404)
	}))
	defer srv.Close()
	comp := &Component{Manifest: ManifestInfo{ID: "m1"}, ManifestDownload: DownloadConfig{URLPrefix: srv.URL}}

	installDir := t.TempDir()
	// a.dat present and valid.
	os.MkdirAll(filepath.Join(installDir, "GenshinImpact_Data"), 0o755)
	os.WriteFile(filepath.Join(installDir, "GenshinImpact_Data", "a.dat"), []byte(content), 0o644)

	ok, total, missing, err := VerifyAssets(context.Background(), srv.Client(), comp, installDir)
	if err != nil {
		t.Fatalf("VerifyAssets: %v", err)
	}
	if total != 2 {
		t.Fatalf("expected total 2, got %d", total)
	}
	if ok != 1 {
		t.Fatalf("expected 1 valid, got %d", ok)
	}
	if missing <= 0 {
		t.Fatalf("expected missing bytes > 0, got %d", missing)
	}
}

func TestDownloadAssetsReuseAltRoot(t *testing.T) {
	shared := "shared-resource-content"
	installDir := t.TempDir()
	// The other server family's data folder already has the shared file.
	os.MkdirAll(filepath.Join(installDir, "GenshinImpact_Data", "StreamingAssets"), 0o755)
	os.WriteFile(filepath.Join(installDir, "GenshinImpact_Data", "StreamingAssets", "x.blk"), []byte(shared), 0o644)

	// Target CN asset with the same content (by relative path under a
	// different data folder) must be reused from the alt root.
	chunk := pbBytes(1, "c_x")
	chunk = append(chunk, pbVarint(3, 0)...)
	chunk = append(chunk, pbVarint(4, uint64(len(shared)))...)
	chunk = append(chunk, pbVarint(5, uint64(len(shared)))...)
	asset := buildAsset("YuanShen_Data/StreamingAssets/x.blk", chunk, md5Hex(shared), uint64(len(shared)))
	manifestData := buildManifest(t, [][]byte{asset})

	var chunkHits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/m1" {
			w.Write(manifestData)
			return
		}
		chunkHits.Add(1)
		w.WriteHeader(404)
	}))
	defer srv.Close()
	comp := &Component{
		Manifest:         ManifestInfo{ID: "m1"},
		ManifestDownload: DownloadConfig{URLPrefix: srv.URL},
		ChunkDownload:    DownloadConfig{URLPrefix: srv.URL},
	}
	altRoot := filepath.Join(installDir, "GenshinImpact_Data")

	_, err := DownloadAssets(context.Background(), srv.Client(), comp, installDir,
		Options{ReuseAltRoot: altRoot})
	if err != nil {
		t.Fatalf("DownloadAssets: %v", err)
	}
	// CN target file must exist with shared content, and no chunk downloaded.
	out := filepath.Join(installDir, "YuanShen_Data", "StreamingAssets", "x.blk")
	data, err := os.ReadFile(out)
	if err != nil || string(data) != shared {
		t.Fatalf("CN file not reused correctly: %v %q", err, data)
	}
	if got := chunkHits.Load(); got != 0 {
		t.Fatalf("expected 0 chunk downloads (reused from alt root), got %d", got)
	}
}

func TestReuseHardlinksSharedFiles(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.dat")
	os.WriteFile(src, []byte("shared-content"), 0o644)

	// copyFile must hardlink (same inode) so shared files are stored once.
	if err := copyFile(src, filepath.Join(dir, "dst.dat")); err != nil {
		t.Fatalf("copyFile: %v", err)
	}
	si, _ := os.Stat(src)
	di, _ := os.Stat(filepath.Join(dir, "dst.dat"))
	if !os.SameFile(si, di) {
		t.Fatal("expected hard link (same inode) for shared file, got a copy")
	}
}

func TestDownloadAssetsSkipExisting(t *testing.T) {
	content := "already-valid-content"
	installDir := t.TempDir()
	// Target already present and valid.
	target := filepath.Join(installDir, "GenshinImpact_Data", "x.dat")
	os.MkdirAll(filepath.Dir(target), 0o755)
	os.WriteFile(target, []byte(content), 0o644)

	chunk := pbBytes(1, "c_x")
	chunk = append(chunk, pbVarint(3, 0)...)
	chunk = append(chunk, pbVarint(4, uint64(len(content)))...)
	chunk = append(chunk, pbVarint(5, uint64(len(content)))...)
	asset := buildAsset("GenshinImpact_Data/x.dat", chunk, md5Hex(content), uint64(len(content)))
	manifestData := buildManifest(t, [][]byte{asset})

	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/manifest_1" {
			w.Write(manifestData)
			return
		}
		hits.Add(1)
		w.WriteHeader(404)
	}))
	defer srv.Close()

	comp := &Component{
		Manifest:         ManifestInfo{ID: "manifest_1"},
		ManifestDownload: DownloadConfig{URLPrefix: srv.URL},
		ChunkDownload:    DownloadConfig{URLPrefix: srv.URL},
	}
	_, err := DownloadAssets(context.Background(), srv.Client(), comp, installDir,
		Options{SkipExisting: true})
	if err != nil {
		t.Fatalf("DownloadAssets: %v", err)
	}
	if got := hits.Load(); got != 0 {
		t.Fatalf("expected no chunk download (file valid), got %d", got)
	}
}
