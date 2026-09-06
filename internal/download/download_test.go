package download

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadAndResume(t *testing.T) {
	body := []byte("0123456789abcdef")
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data := body
		if r.Header.Get("Range") != "" {
			var start int
			if _, err := fmt.Sscanf(r.Header.Get("Range"), "bytes=%d-", &start); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			data = body[start:]
			w.Header().Set("Content-Range", "bytes 0-15/16")
			w.WriteHeader(http.StatusPartialContent)
		}
		w.Write(data)
	}))
	defer srv.Close()

	dir := t.TempDir()
	dest := filepath.Join(dir, "file.bin")

	// First pass: full download.
	mg := NewManager()
	md5hex := md5Hex(string(body))
	r1 := mg.Download(context.Background(), Task{URL: srv.URL, Dest: dest, ExpectedSize: 16, ExpectedMD5: md5hex}, nil)
	if r1.Err != nil || !r1.Verified {
		t.Fatalf("first download failed: %v verified=%v", r1.Err, r1.Verified)
	}

	// Second pass: should resume and verify.
	r2 := mg.Download(context.Background(), Task{URL: srv.URL, Dest: dest, ExpectedSize: 16, ExpectedMD5: md5hex}, nil)
	if r2.Err != nil {
		t.Fatalf("resume download failed: %v", r2.Err)
	}
	got, _ := os.ReadFile(dest)
	if string(got) != string(body) {
		t.Fatalf("content mismatch: %q", got)
	}
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}
