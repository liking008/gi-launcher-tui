package manifest

import (
	"crypto/md5"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, content string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return md5Hex(content)
}

func md5Hex(s string) string {
	h := md5.Sum([]byte(s))
	return hex.EncodeToString(h[:])
}

func TestVerify(t *testing.T) {
	root := t.TempDir()

	okContent := "hello world"
	okMD5 := md5Hex(okContent)
	badContent := "tampered"

	writeTestFile(t, filepath.Join(root, "good.txt"), okContent)
	writeTestFile(t, filepath.Join(root, "bad.txt"), badContent)

	manifestPath := filepath.Join(root, "pkg_version")
	content := "{\"remoteName\":\"good.txt\",\"md5\":\"" + okMD5 + "\",\"hash\":\"\",\"fileSize\":11}\n" +
		"{\"remoteName\":\"missing.txt\",\"md5\":\"00000000000000000000000000000000\",\"hash\":\"\",\"fileSize\":5}\n" +
		"{\"remoteName\":\"bad.txt\",\"md5\":\"ffffffffffffffffffffffffffffffff\",\"hash\":\"\",\"fileSize\":8}\n"
	if err := os.WriteFile(manifestPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	m, err := Load(manifestPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(m.Files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(m.Files))
	}

	res := m.Verify(root, 4, nil)
	if res.Ok != 1 {
		t.Errorf("expected 1 ok, got %d", res.Ok)
	}
	if res.Missing != 1 {
		t.Errorf("expected 1 missing, got %d", res.Missing)
	}
	if res.Mismatch != 1 {
		t.Errorf("expected 1 mismatch, got %d", res.Mismatch)
	}
	if res.Downloaded != 5+8 {
		t.Errorf("expected downloaded 13 bytes, got %d", res.Downloaded)
	}
}
