package sophon

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/klauspost/compress/zstd"

	"github.com/liking008/gi-launcher-tui/internal/edition"
	"github.com/liking008/gi-launcher-tui/internal/manifest"
)

// Build is the getBuild API response (sophon chunk download mode).
type Build struct {
	Retcode int       `json:"retcode"`
	Message string    `json:"message"`
	Data    BuildData `json:"data"`
}

type BuildData struct {
	BuildID   string      `json:"build_id"`
	Tag       string      `json:"tag"`
	Manifests []Component `json:"manifests"`
}

// Component is one manifest category (game resources or a voice pack).
type Component struct {
	CategoryID       string         `json:"category_id"`
	CategoryName     string         `json:"category_name"`
	Manifest         ManifestInfo   `json:"manifest"`
	ChunkDownload    DownloadConfig `json:"chunk_download"`
	ManifestDownload DownloadConfig `json:"manifest_download"`
	MatchingField    string         `json:"matching_field"`
}

type ManifestInfo struct {
	ID               string `json:"id"`
	Checksum         string `json:"checksum"`
	CompressedSize   string `json:"compressed_size"`
	UncompressedSize string `json:"uncompressed_size"`
}

type DownloadConfig struct {
	Encryption  int    `json:"encryption"`
	Password    string `json:"password"`
	Compression int    `json:"compression"`
	URLPrefix   string `json:"url_prefix"`
	URLSuffix   string `json:"url_suffix"`
}

// GetBuild queries the getBuild API for the given edition/package.
func GetBuild(ctx context.Context, hc *http.Client, ed edition.Edition, packageID, password, tag string) (*Build, error) {
	url := fmt.Sprintf("%s/downloader/sophon_chunk/api/getBuild?branch=main&password=%s&package_id=%s&tag=%s",
		ed.SophonDataURL(), password, packageID, tag)
	body, err := httpGet(ctx, hc, url)
	if err != nil {
		return nil, err
	}
	var b Build
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, err
	}
	if b.Retcode != 0 {
		return nil, fmt.Errorf("getBuild retcode %d: %s", b.Retcode, b.Message)
	}
	return &b, nil
}

// FetchManifest downloads and decodes one component's asset manifest.
func FetchManifest(ctx context.Context, hc *http.Client, comp *Component) ([]Asset, error) {
	if comp.ManifestDownload.Encryption != 0 {
		return nil, fmt.Errorf("manifest 加密暂不支持 (%s)", comp.CategoryName)
	}
	url := comp.ManifestDownload.URLPrefix + comp.ManifestDownload.URLSuffix + "/" + comp.Manifest.ID
	data, err := httpGet(ctx, hc, url)
	if err != nil {
		return nil, err
	}
	if comp.ManifestDownload.Compression != 0 {
		data, err = zstdDecompress(data)
		if err != nil {
			return nil, err
		}
	}
	return DecodeAssets(data)
}

// Options controls the asset download behaviour.
type Options struct {
	// SkipExisting skips files already present at their target path with a
	// matching MD5 (resume / verify-before-download).
	SkipExisting bool
	// ReuseContent looks up an asset's content elsewhere in the install (e.g.
	// from another server's data folder) by MD5 and copies it instead of
	// downloading. This makes server switching download only genuinely new
	// files. If ReuseIndex is nil it is built once from installDir.
	ReuseContent bool
	// ReuseIndex is a prebuilt MD5 -> path map (see BuildContentIndex). When
	// provided it is used instead of re-scanning installDir, which is much
	// faster when downloading multiple components.
	ReuseIndex map[string]string
	// ReuseAltRoot is another server family's data folder (e.g. the
	// GenshinImpact_Data path when downloading the CN client). Assets whose
	// content is shared between servers are reused from it by relative path +
	// MD5 instead of being re-downloaded.
	ReuseAltRoot string
	// Progress is invoked with (written, total) bytes as assets complete.
	Progress func(done, total int64)
}

// DownloadAssets downloads all chunks of a manifest and assembles the asset
// files into installDir. Files that already exist and match are skipped; with
// ReuseContent, matching content found elsewhere in the install is copied
// instead of re-downloaded. Returns the number of bytes written.
func DownloadAssets(ctx context.Context, hc *http.Client, comp *Component, installDir string, opts Options) (int64, error) {
	assets, err := FetchManifest(ctx, hc, comp)
	if err != nil {
		return 0, err
	}
	if comp.ChunkDownload.Encryption != 0 {
		return 0, fmt.Errorf("chunk 加密暂不支持 (%s)", comp.CategoryName)
	}
	var total int64
	for _, a := range assets {
		total += a.Size
	}

	// Optional content index for cross-folder reuse.
	var reuseIdx map[string]string
	if opts.ReuseContent {
		if opts.ReuseIndex != nil {
			reuseIdx = opts.ReuseIndex
		} else {
			reuseIdx = BuildContentIndex(installDir, nil)
		}
	}

	// Deduplicate chunks by name so shared chunks are only downloaded once.
	chunkCache := map[string][]byte{}
	var written int64

	report := func() {
		if opts.Progress != nil {
			opts.Progress(written, total)
		}
	}

	for _, a := range assets {
		if a.Type == 64 { // directory
			if a.Path != "" {
				os.MkdirAll(filepath.Join(installDir, filepath.FromSlash(a.Path)), 0o755)
			}
			continue
		}
		dest := filepath.Join(installDir, filepath.FromSlash(a.Path))
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return written, err
		}

		// 1) Already present at the target with the same size -> skip. Size is
		//    used instead of MD5 so files changed by in-game hot updates (which
		//    keep block files the same size) are preserved, not re-downloaded.
		if opts.SkipExisting && fileSize(dest) == a.Size {
			written += a.Size
			report()
			continue
		}
		// 2) Content exists elsewhere in the install -> copy it.
		if opts.ReuseContent && a.HashMD5 != "" {
			if src, ok := reuseIdx[strings.ToLower(a.HashMD5)]; ok && src != dest {
				if copyFile(src, dest) == nil {
					written += a.Size
					report()
					continue
				}
			}
		}
		// 2b) Reuse from the other server family by relative path + MD5.
		if opts.ReuseAltRoot != "" && a.HashMD5 != "" {
			if cand := altCounterpart(opts.ReuseAltRoot, a.Path, dest); cand != "" &&
				md5File(cand) == strings.ToLower(a.HashMD5) {
				if copyFile(cand, dest) == nil {
					written += a.Size
					report()
					continue
				}
			}
		}

		// 3) Download chunks (progress reported per chunk for smooth bars).
		if err := assembleAsset(ctx, hc, comp, &a, dest, chunkCache, func(w int64) {
			written += w
			report()
		}); err != nil {
			return written, err
		}
		if a.HashMD5 != "" && !strings.EqualFold(md5File(dest), a.HashMD5) {
			return written, fmt.Errorf("文件校验失败: %s", a.Path)
		}
	}
	return written, nil
}

func assembleAsset(ctx context.Context, hc *http.Client, comp *Component, a *Asset, dest string, chunkCache map[string][]byte, onChunk func(int64)) error {
	f, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	for _, c := range a.Chunks {
		body, ok := chunkCache[c.Name]
		if !ok {
			body, err = fetchChunk(ctx, hc, comp, &c)
			if err != nil {
				f.Close()
				return err
			}
			chunkCache[c.Name] = body
		}
		if _, err := f.WriteAt(body, c.Offset); err != nil {
			f.Close()
			return err
		}
		if onChunk != nil {
			onChunk(int64(len(body)))
		}
	}
	return f.Close()
}

// BuildContentIndexFromManifests builds the reuse index instantly from the
// existing pkg_version / Audio_*_pkg_version manifests instead of hashing the
// whole install tree. Each manifest already records every file's MD5 and path,
// which is exactly what we need to reuse shared content (e.g. when converting
// a CN install to the international server in the same folder).
func BuildContentIndexFromManifests(installDir string) map[string]string {
	idx := make(map[string]string)
	manifests := []string{
		"pkg_version",
		"Audio_Chinese_pkg_version",
		"Audio_English(US)_pkg_version",
		"Audio_Japanese_pkg_version",
		"Audio_Korean_pkg_version",
	}
	for _, name := range manifests {
		m, err := manifest.Load(filepath.Join(installDir, name))
		if err != nil {
			continue
		}
		for _, f := range m.Files {
			if f.MD5 == "" {
				continue
			}
			idx[strings.ToLower(f.MD5)] = filepath.Join(installDir, filepath.FromSlash(f.RemoteName))
		}
	}
	return idx
}

// BuildContentIndex walks installDir and maps each file's MD5 to its path.
// It hashes files in parallel and calls onProgress with the number of files
// processed so far (for UI feedback during long scans).
func BuildContentIndex(installDir string, onProgress func(files int64)) map[string]string {
	var paths []string
	filepath.Walk(installDir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() || info.Size() == 0 {
			return nil
		}
		paths = append(paths, p)
		return nil
	})

	idx := make(map[string]string)
	const workers = 8
	sem := make(chan struct{}, workers)
	var mu sync.Mutex
	var done int64
	var wg sync.WaitGroup
	for _, p := range paths {
		p := p
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			h := md5File(p)
			if h != "" {
				mu.Lock()
				idx[strings.ToLower(h)] = p
				mu.Unlock()
			}
			if onProgress != nil {
				done++
				onProgress(done)
			}
		}()
	}
	wg.Wait()
	return idx
}

func copyFile(src, dst string) error {
	// Try a hardlink first (same filesystem -> instant); fall back to copying.
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
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

// VerifyAssets checks each file in the component's manifest against installDir,
// returning how many files are complete (present with matching MD5) out of the
// total, and how many bytes are missing/invalid.
func VerifyAssets(ctx context.Context, hc *http.Client, comp *Component, installDir string) (ok, total int, missingBytes int64, err error) {
	assets, err := FetchManifest(ctx, hc, comp)
	if err != nil {
		return 0, 0, 0, err
	}
	for _, a := range assets {
		if a.Type == 64 {
			continue
		}
		total++
		// Size-based: hot-updated files (same size) count as complete.
		if fileSize(filepath.Join(installDir, filepath.FromSlash(a.Path))) == a.Size {
			ok++
		} else {
			missingBytes += a.Size
		}
	}
	return ok, total, missingBytes, nil
}

// altCounterpart maps an asset path (e.g. "YuanShen_Data/StreamingAssets/x")
// into the other server family's data folder by replacing the leading data
// folder segment. Returns "" if it maps onto dest itself.
func altCounterpart(altRoot, assetPath, dest string) string {
	i := strings.Index(assetPath, "/")
	if i < 0 {
		return ""
	}
	cand := filepath.Join(altRoot, filepath.FromSlash(assetPath[i+1:]))
	if cand == dest {
		return ""
	}
	return cand
}

// MissingAssets returns the assets in the component's manifest that are not
// present (or have a wrong MD5) in installDir, plus their total size.
func MissingAssets(ctx context.Context, hc *http.Client, comp *Component, installDir string) ([]Asset, int64, error) {
	assets, err := FetchManifest(ctx, hc, comp)
	if err != nil {
		return nil, 0, err
	}
	var missing []Asset
	var size int64
	for _, a := range assets {
		if a.Type == 64 {
			continue
		}
		dest := filepath.Join(installDir, filepath.FromSlash(a.Path))
		if a.HashMD5 != "" && md5File(dest) == strings.ToLower(a.HashMD5) {
			continue
		}
		missing = append(missing, a)
		size += a.Size
	}
	return missing, size, nil
}

func fileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return st.Size()
}

// MissingAssetsFast counts how many files in the component's manifest are
// absent from installDir, using existence checks only (no hashing). This is
// much faster than MissingAssets for deciding whether an install is complete.
func MissingAssetsFast(ctx context.Context, hc *http.Client, comp *Component, installDir string) (int, int64, error) {
	assets, err := FetchManifest(ctx, hc, comp)
	if err != nil {
		return 0, 0, err
	}
	var missing int
	var size int64
	for _, a := range assets {
		if a.Type == 64 {
			continue
		}
		dest := filepath.Join(installDir, filepath.FromSlash(a.Path))
		if _, err := os.Stat(dest); err != nil {
			missing++
			size += a.Size
		}
	}
	return missing, size, nil
}

func fetchChunk(ctx context.Context, hc *http.Client, comp *Component, c *Chunk) ([]byte, error) {
	url := comp.ChunkDownload.URLPrefix + comp.ChunkDownload.URLSuffix + "/" + c.Name
	body, err := httpGet(ctx, hc, url)
	if err != nil {
		return nil, err
	}
	if comp.ChunkDownload.Compression != 0 {
		body, err = zstdDecompress(body)
		if err != nil {
			return nil, fmt.Errorf("解压 chunk %s: %w", c.Name, err)
		}
	}
	if int64(len(body)) != c.DecompressedSize {
		return nil, fmt.Errorf("chunk %s 大小不符: got %d want %d", c.Name, len(body), c.DecompressedSize)
	}
	return body, nil
}

func httpGet(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 2<<30))
}

func zstdDecompress(data []byte) ([]byte, error) {
	dec, err := zstd.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer dec.Close()
	return io.ReadAll(dec)
}

func md5File(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := md5.New()
	io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}
