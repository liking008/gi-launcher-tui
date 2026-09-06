package sophon

import (
	"context"
	"net/http"
	"testing"

	"github.com/liking008/gi-launcher-tui/internal/edition"
	"github.com/liking008/gi-launcher-tui/internal/resource"
)

// TestFetchSingleChunkLive downloads one real chunk and verifies its size,
// validating the chunk fetch + zstd decompression pipeline.
func TestFetchSingleChunkLive(t *testing.T) {
	ctx := context.Background()
	hc := &http.Client{}
	res := resource.New()
	ed := edition.OfficialCN

	branch, err := res.FetchBranch(ctx, ed)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	build, err := GetBuild(ctx, hc, ed, branch.Main.PackageID, branch.Main.Password, branch.Main.Tag)
	if err != nil {
		t.Fatalf("getBuild: %v", err)
	}
	var game *Component
	for i := range build.Data.Manifests {
		if build.Data.Manifests[i].MatchingField == "game" {
			game = &build.Data.Manifests[i]
			break
		}
	}
	assets, err := FetchManifest(ctx, hc, game)
	if err != nil {
		t.Fatalf("manifest: %v", err)
	}
	// Pick the first file asset and download its first chunk.
	for _, a := range assets {
		if a.Type == 64 || len(a.Chunks) == 0 {
			continue
		}
		body, err := fetchChunk(ctx, hc, game, &a.Chunks[0])
		if err != nil {
			t.Fatalf("fetchChunk: %v", err)
		}
		if int64(len(body)) != a.Chunks[0].DecompressedSize {
			t.Fatalf("chunk %s size %d != expected %d", a.Chunks[0].Name, len(body), a.Chunks[0].DecompressedSize)
		}
		t.Logf("asset=%s chunk=%s decompressed=%d bytes OK", a.Path, a.Chunks[0].Name, len(body))
		return
	}
	t.Fatal("no file asset with chunks found")
}

// TestDecodeAssets uses a tiny synthetic protobuf to exercise the decoder.
func TestDecodeAssets(t *testing.T) {
	// Manually build: assets[0]{path="a/b", type=0, size=10, chunks[0]{name="1",offset=0}}
	p := func(field int, s string) []byte {
		out := []byte{byte(field<<3 | 2), byte(len(s))}
		return append(out, s...)
	}
	v := func(field int, val uint64) []byte {
		var out []byte
		out = append(out, byte(field<<3|0))
		// varint encode val
		for val >= 0x80 {
			out = append(out, byte(val&0x7f)|0x80)
			val >>= 7
		}
		out = append(out, byte(val))
		return out
	}
	chunk := append(p(1, "1"), v(3, 0)...)
	chunk = append(chunk, v(4, 100)...)
	chunk = append(chunk, v(5, 100)...)
	chunkMsg := append([]byte{byte(2<<3 | 2), byte(len(chunk))}, chunk...)
	asset := append(p(1, "a/b"), chunkMsg...)
	asset = append(asset, v(3, 0)...)
	asset = append(asset, v(4, 10)...)
	assetMsg := append([]byte{byte(1<<3 | 2), byte(len(asset))}, asset...)

	assets, err := DecodeAssets(assetMsg)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(assets))
	}
	a := assets[0]
	if a.Path != "a/b" || a.Type != 0 || a.Size != 10 || len(a.Chunks) != 1 {
		t.Fatalf("asset mismatch: %+v", a)
	}
	if a.Chunks[0].Name != "1" || a.Chunks[0].Offset != 0 || a.Chunks[0].CompressedSize != 100 {
		t.Fatalf("chunk mismatch: %+v", a.Chunks[0])
	}
}
func TestGetBuildAndManifestLive(t *testing.T) {
	ctx := context.Background()
	hc := &http.Client{}
	res := resource.New()
	ed := edition.OfficialCN

	branch, err := res.FetchBranch(ctx, ed)
	if err != nil {
		t.Fatalf("branch: %v", err)
	}
	if branch.Main.Tag == "" || branch.Main.PackageID == "" {
		t.Fatalf("empty branch info: %+v", branch.Main)
	}
	t.Logf("latest version: %s", branch.Main.Tag)

	build, err := GetBuild(ctx, hc, ed, branch.Main.PackageID, branch.Main.Password, branch.Main.Tag)
	if err != nil {
		t.Fatalf("getBuild: %v", err)
	}
	if build.Data.Tag != branch.Main.Tag {
		t.Fatalf("build tag %s != branch tag %s", build.Data.Tag, branch.Main.Tag)
	}
	if len(build.Data.Manifests) == 0 {
		t.Fatal("no manifests in build")
	}

	// Decode the game resources manifest (matching_field == "game").
	var game *Component
	for i := range build.Data.Manifests {
		if build.Data.Manifests[i].MatchingField == "game" {
			game = &build.Data.Manifests[i]
			break
		}
	}
	if game == nil {
		t.Fatal("game manifest not found")
	}
	assets, err := FetchManifest(ctx, hc, game)
	if err != nil {
		t.Fatalf("fetch manifest: %v", err)
	}
	t.Logf("game assets: %d", len(assets))
	if len(assets) == 0 {
		t.Fatal("empty game asset list")
	}

	var files, total int64
	for _, a := range assets {
		if a.Type != 64 {
			files++
			total += a.Size
		}
	}
	t.Logf("files=%d total_bytes=%d (%.2f GB)", files, total, float64(total)/1e9)
	if files == 0 {
		t.Fatal("no files in manifest")
	}
}
