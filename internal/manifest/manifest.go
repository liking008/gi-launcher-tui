package manifest

import (
	"bufio"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// File describes one entry line of a pkg_version manifest.
type File struct {
	RemoteName string `json:"remoteName"`
	MD5        string `json:"md5"`
	Hash       string `json:"hash"`
	FileSize   int64  `json:"fileSize"`
}

// Manifest is the set of files defined by a single pkg_version file.
type Manifest struct {
	Name  string
	Files []File
}

// Load reads a pkg_version file (one JSON object per line).
func Load(path string) (*Manifest, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	m := &Manifest{Name: filepath.Base(path)}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var fe File
		if err := json.Unmarshal([]byte(line), &fe); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		m.Files = append(m.Files, fe)
	}
	return m, sc.Err()
}

// VerifyResult summarises a verification pass over one manifest.
type VerifyResult struct {
	Total      int
	Ok         int
	Mismatch   int
	Missing    int
	Downloaded int64 // bytes the game is missing / different
	Failed     []File
}

// Verify checks each manifest file against the game directory. Missing and
// mismatched files are collected for a later download pass.
func (m *Manifest) Verify(gameDir string, workers int, onProgress func(done, total int)) *VerifyResult {
	res := &VerifyResult{Total: len(m.Files)}
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)
	var mu sync.Mutex

	for _, fe := range m.Files {
		fe := fe
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			local := filepath.Join(gameDir, filepath.FromSlash(fe.RemoteName))
			if verifyFile(local, fe) {
				res.Ok++
			} else {
				mu.Lock()
				res.Failed = append(res.Failed, fe)
				mu.Unlock()
			}
			if onProgress != nil {
				onProgress(res.Ok+len(res.Failed), res.Total)
			}
		}()
	}
	wg.Wait()

	for _, fe := range res.Failed {
		if fileExists(filepath.Join(gameDir, filepath.FromSlash(fe.RemoteName))) {
			res.Mismatch++
		} else {
			res.Missing++
		}
		res.Downloaded += fe.FileSize
	}
	return res
}

func verifyFile(local string, fe File) bool {
	st, err := os.Stat(local)
	if err != nil || st.Size() != fe.FileSize {
		return false
	}
	return md5OfFile(local) == strings.ToLower(fe.MD5)
}

func md5OfFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}

// TotalSize sums the bytes that failed verification.
func (r *VerifyResult) TotalSize() int64 {
	var n int64
	for _, fe := range r.Failed {
		n += fe.FileSize
	}
	return n
}
