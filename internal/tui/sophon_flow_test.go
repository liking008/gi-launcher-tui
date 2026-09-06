package tui

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/liking008/gi-launcher-tui/internal/sophon"
)

// pbBytes / pbVarint are minimal protobuf encoders for the test manifest.
func pbStr(field int, s string) []byte {
	out := []byte{byte(field<<3 | 2), byte(len(s))}
	return append(out, s...)
}
func pbInt(field int, v uint64) []byte {
	out := []byte{byte(field<<3 | 0)}
	for v >= 0x80 {
		out = append(out, byte(v&0x7f)|0x80)
		v >>= 7
	}
	out = append(out, byte(v))
	return out
}

// buildTestManifest encodes a single asset split into two chunks.
func buildTestManifest(path, content string) []byte {
	half := len(content) / 2
	c1 := content[:half]
	c2 := content[half:]
	chunk1 := pbStr(1, "c0")
	chunk1 = append(chunk1, pbInt(3, 0)...)
	chunk1 = append(chunk1, pbInt(4, uint64(len(c1)))...)
	chunk1 = append(chunk1, pbInt(5, uint64(len(c1)))...)
	chunk2 := pbStr(1, "c1")
	chunk2 = append(chunk2, pbInt(3, uint64(half))...)
	chunk2 = append(chunk2, pbInt(4, uint64(len(c2)))...)
	chunk2 = append(chunk2, pbInt(5, uint64(len(c2)))...)
	asset := pbStr(1, path)
	asset = append(asset, pbStr(2, string(chunk1))...)
	asset = append(asset, pbStr(2, string(chunk2))...)
	asset = append(asset, pbInt(3, 0)...)
	asset = append(asset, pbInt(4, uint64(len(content)))...)
	return append([]byte{byte(1<<3 | 2), byte(len(asset))}, asset...)
}

// TestSophonDownloadProgressFlow verifies that starting a sophon download
// actually delivers progress messages to the model (regression for the
// "stuck at 0%" bug caused by no subscriber reading the channel).
func TestSophonDownloadProgressFlow(t *testing.T) {
	content := "hello-sophon-content"
	manifest := buildTestManifest("GenshinImpact_Data/x.dat", content)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/m1":
			w.Write(manifest)
		case "/c0":
			w.Write([]byte(content[:len(content)/2]))
		case "/c1":
			w.Write([]byte(content[len(content)/2:]))
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	comp := &sophon.Component{
		Manifest:         sophon.ManifestInfo{ID: "m1"},
		ManifestDownload: sophon.DownloadConfig{URLPrefix: srv.URL},
		ChunkDownload:    sophon.DownloadConfig{URLPrefix: srv.URL},
	}

	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	installDir := t.TempDir()
	m.dl = &downloadState{step: "running", sophonPlan: true, sophonComps: []*sophon.Component{comp}, installDir: installDir}

	cmd := m.startSophonDownload()
	if cmd == nil {
		t.Fatal("startSophonDownload returned nil cmd")
	}

	// Drain messages from the channel via repeated cmd() calls.
	var sawProgress, sawFinished bool
	var lastSize int64
	var progressSteps int
	deadline := time.After(5 * time.Second)
	for {
		if sawFinished {
			break
		}
		select {
		case <-deadline:
			t.Fatal("timed out waiting for download messages")
		default:
		}
		msg := cmd()
		switch v := msg.(type) {
		case dlProgress:
			if v.size > 0 {
				sawProgress = true
				if v.done > lastSize {
					progressSteps++
					lastSize = v.done
				}
			}
		case dlFinished:
			sawFinished = true
		case dlClosed:
			return
		}
	}

	if !sawProgress {
		t.Fatal("no progress message with size>0 received")
	}
	// Per-chunk reporting must yield more than one increasing step.
	if progressSteps < 2 {
		t.Fatalf("expected multiple increasing progress steps (per-chunk), got %d", progressSteps)
	}
	if !sawFinished {
		t.Fatal("no dlFinished received")
	}
	// The asset file should have been written.
	data, err := os.ReadFile(filepath.Join(installDir, "GenshinImpact_Data", "x.dat"))
	if err != nil || string(data) != content {
		t.Fatalf("asset not written correctly: %v %q", err, data)
	}
}
