package download

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Progress is delivered periodically during a download.
type Progress struct {
	Downloaded int64
	Total      int64 // -1 if unknown
	Percent    float64
}

// Task describes one resumable file download.
type Task struct {
	URL          string
	Dest         string
	ExpectedSize int64
	ExpectedMD5  string
}

// Result reports the outcome of a download.
type Result struct {
	URL      string
	Dest     string
	Size     int64
	Verified bool
	Err      error
}

// Manager runs downloads with resume support. Each destination file is written
// to <dest>.part and, once fully received and MD5-verified, renamed to <dest>.
type Manager struct {
	http *http.Client
}

func NewManager() *Manager {
	return &Manager{http: &http.Client{Timeout: 0}}
}

// Download fetches one task, resuming from any existing .part file, and calls
// onProgress as bytes arrive. A partial file whose MD5 already matches the
// target is considered complete.
func (m *Manager) Download(ctx context.Context, t Task, onProgress func(Progress)) Result {
	part := t.Dest + ".part"
	if err := os.MkdirAll(filepath.Dir(part), 0o755); err != nil {
		return Result{URL: t.URL, Dest: t.Dest, Err: err}
	}

	// Resume if the existing .part is already the full size and valid.
	if t.ExpectedMD5 != "" {
		if h, ok := fileMD5(part); ok && (t.ExpectedSize <= 0 || size(part) == t.ExpectedSize) && h == t.ExpectedMD5 {
			os.Rename(part, t.Dest)
			return Result{URL: t.URL, Dest: t.Dest, Size: t.ExpectedSize, Verified: true}
		}
	}

	f, err := os.OpenFile(part, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return Result{URL: t.URL, Dest: t.Dest, Err: err}
	}
	offset, _ := f.Seek(0, io.SeekEnd)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		f.Close()
		return Result{URL: t.URL, Dest: t.Dest, Err: err}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	req.Header.Set("Accept-Encoding", "identity")
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := m.http.Do(req)
	if err != nil {
		f.Close()
		return Result{URL: t.URL, Dest: t.Dest, Err: err}
	}
	defer resp.Body.Close()

	total := t.ExpectedSize
	if total <= 0 {
		total = resp.ContentLength
	}
	if resp.StatusCode == http.StatusOK {
		// Server ignored the range request; restart from scratch.
		offset = 0
		if _, err := f.Seek(0, io.SeekStart); err == nil {
			f.Truncate(0)
		}
	} else if resp.StatusCode != http.StatusPartialContent && resp.StatusCode != http.StatusOK {
		f.Close()
		return Result{URL: t.URL, Dest: t.Dest, Err: fmt.Errorf("http status %s", resp.Status)}
	}

	downloaded := offset
	last := time.Now()
	buf := make([]byte, 256*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return Result{URL: t.URL, Dest: t.Dest, Err: werr}
			}
			downloaded += int64(n)
			if onProgress != nil {
				p := Progress{Downloaded: downloaded, Total: total}
				if total > 0 {
					p.Percent = float64(downloaded) / float64(total) * 100
				} else {
					p.Percent = -1
				}
				if time.Since(last) >= 100*time.Millisecond {
					onProgress(p)
					last = time.Now()
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			return Result{URL: t.URL, Dest: t.Dest, Err: rerr}
		}
	}
	f.Close()

	verified := false
	if t.ExpectedMD5 != "" {
		if h, ok := fileMD5(part); ok {
			verified = strings.EqualFold(h, t.ExpectedMD5)
		}
		if !verified {
			os.Remove(part)
			return Result{URL: t.URL, Dest: t.Dest, Size: downloaded, Err: fmt.Errorf("md5 mismatch: got %s want %s", md5Short(part), t.ExpectedMD5)}
		}
	} else if t.ExpectedSize > 0 {
		verified = size(part) == t.ExpectedSize
	}

	if err := os.Rename(part, t.Dest); err != nil {
		return Result{URL: t.URL, Dest: t.Dest, Size: downloaded, Err: err}
	}
	if onProgress != nil {
		onProgress(Progress{Downloaded: downloaded, Total: total, Percent: 100})
	}
	return Result{URL: t.URL, Dest: t.Dest, Size: downloaded, Verified: verified}
}

func fileMD5(path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", false
	}
	return hex.EncodeToString(h.Sum(nil)), true
}

func md5Short(path string) string {
	h, _ := fileMD5(path)
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func size(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}
