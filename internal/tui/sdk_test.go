package tui

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/liking008/gi-launcher-tui/internal/edition"
	"github.com/liking008/gi-launcher-tui/internal/game"
)

// TestEnsurePCSDKBackupFirst verifies the dll is restored from backup (no
// download) when switching to B服, and moved to backup when switching away.
func TestEnsurePCSDKBackupFirst(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	installDir := t.TempDir()
	backupRoot := filepath.Join(t.TempDir(), "backup")
	backupDLL := filepath.Join(backupRoot, filepath.Dir(filepath.FromSlash(game.PCSDK)), "PCGameSDK.dll")
	os.MkdirAll(filepath.Dir(backupDLL), 0o755)
	os.WriteFile(backupDLL, []byte("backup-sdk"), 0o644)

	// Simulate the full SDK (BLPlatform64) already extracted, only the dll is
	// missing -> restore from backup, do NOT download the 85MB SDK.
	os.MkdirAll(filepath.Join(installDir, "YuanShen_Data", "Plugins", "BLPlatform64"), 0o755)
	if err := m.ensurePCSDK(installDir, edition.Bilibili, backupRoot); err != nil {
		t.Fatalf("ensurePCSDK bilibili: %v", err)
	}
	sdk := filepath.Join(installDir, filepath.FromSlash(game.PCSDK))
	data, err := os.ReadFile(sdk)
	if err != nil || string(data) != "backup-sdk" {
		t.Fatalf("expected dll restored from backup, got %q err=%v", data, err)
	}

	// Switching away to 官服 -> dll moved into backup.
	if err := m.ensurePCSDK(installDir, edition.OfficialCN, backupRoot); err != nil {
		t.Fatalf("ensurePCSDK official: %v", err)
	}
	if _, err := os.Stat(sdk); !os.IsNotExist(err) {
		t.Errorf("dll should be moved away for non-B server: %v", err)
	}
	if _, err := os.Stat(backupDLL); err != nil {
		t.Errorf("dll should be in backup: %v", err)
	}
}
func TestDownloadPCSDKLive(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	installDir := t.TempDir()
	if err := m.downloadPCSDK(installDir); err != nil {
		t.Fatalf("downloadPCSDK: %v", err)
	}
	sdk := filepath.Join(installDir, filepath.FromSlash(game.PCSDK))
	if _, err := os.Stat(sdk); err != nil {
		t.Fatalf("PCGameSDK.dll not extracted: %v", err)
	}
	if _, err := os.Stat(filepath.Join(installDir, "sdk_pkg_version")); err != nil {
		t.Fatalf("sdk_pkg_version not extracted: %v", err)
	}
	// The modern SDK also ships the BLPlatform64 browser files.
	if _, err := os.Stat(filepath.Join(installDir, "YuanShen_Data", "Plugins", "BLPlatform64")); err != nil {
		t.Fatalf("BLPlatform64 (new SDK) not extracted: %v", err)
	}
}
