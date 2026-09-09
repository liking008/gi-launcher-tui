package game

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/liking008/gi-launcher-tui/internal/edition"
)

func TestSetEditionConfig(t *testing.T) {
	dir := t.TempDir()
	cfg := "[general]\nchannel=1\ncps=mihoyo\ngame_version=7.0.0\nsub_channel=0\n"
	if err := os.WriteFile(filepath.Join(dir, ConfigIni), []byte(cfg), 0o644); err != nil {
		t.Fatal(err)
	}
	g := New(dir)

	// Switch to Bilibili.
	if err := g.SetEdition(edition.Bilibili); err != nil {
		t.Fatalf("SetEdition bilibili: %v", err)
	}
	if ed := g.Edition(); ed.Key != "bilibili" {
		t.Errorf("expected bilibili, got %s", ed.Key)
	}
	c, _ := g.Config()
	if c["channel"] != "14" || c["sub_channel"] != "0" || c["cps"] != "gw_pc" {
		t.Errorf("config after bili switch wrong: %+v", c)
	}
	if c["uapc"] == "" {
		t.Error("uapc not set")
	}
	if _, err := os.Stat(filepath.Join(dir, "YuanShen_Data", "Plugins")); err != nil {
		t.Errorf("Plugins dir should exist after bili switch: %v", err)
	}

	// Switch to Oversea.
	if err := g.SetEdition(edition.Oversea); err != nil {
		t.Fatalf("SetEdition oversea: %v", err)
	}
	if ed := g.Edition(); ed.Key != "oversea" {
		t.Errorf("expected oversea, got %s", ed.Key)
	}
	c, _ = g.Config()
	if c["uapc"] == "" || c["channel"] != "1" || c["sub_channel"] != "0" {
		t.Errorf("config after oversea switch wrong: %+v", c)
	}
	if g.ExePath() != filepath.Join(dir, "GenshinImpact.exe") {
		t.Errorf("oversea exe path wrong: %s", g.ExePath())
	}

	// Back to official CN.
	if err := g.SetEdition(edition.OfficialCN); err != nil {
		t.Fatalf("SetEdition official: %v", err)
	}
	if ed := g.Edition(); ed.Key != "official_cn" {
		t.Errorf("expected official_cn, got %s", ed.Key)
	}
	if g.ExePath() != filepath.Join(dir, "YuanShen.exe") {
		t.Errorf("cn exe path wrong: %s", g.ExePath())
	}
}

func TestSetEditionCreatesMissingDir(t *testing.T) {
	// Target dir does not exist -> must not panic and must create the dir.
	dir := filepath.Join(t.TempDir(), "Genshin Impact Game OS")
	g := New(dir)
	if err := g.SetEdition(edition.Oversea); err != nil {
		t.Fatalf("SetEdition oversea on missing dir: %v", err)
	}
	c, err := g.Config()
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	if c["channel"] != "1" || c["cps"] != "gw_pc" || c["uapc"] == "" {
		t.Errorf("oversea config wrong: %+v", c)
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigIni)); err != nil {
		t.Errorf("config.ini not created: %v", err)
	}
}

func TestFinalizeSwitch(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(t.TempDir(), "backup")

	// Both families present (CN with shared+unique, OS with files).
	os.MkdirAll(filepath.Join(dir, "YuanShen_Data", "StreamingAssets"), 0o755)
	os.MkdirAll(filepath.Join(dir, "GenshinImpact_Data", "StreamingAssets"), 0o755)
	os.WriteFile(filepath.Join(dir, "YuanShen_Data", "StreamingAssets", "shared.blk"), []byte("SHARED"), 0o644)
	os.WriteFile(filepath.Join(dir, "GenshinImpact_Data", "StreamingAssets", "shared.blk"), []byte("SHARED"), 0o644)
	os.WriteFile(filepath.Join(dir, "YuanShen.exe"), []byte("cn-exe"), 0o644)
	os.WriteFile(filepath.Join(dir, "GenshinImpact.exe"), []byte("os-exe"), 0o644)

	// Finalize to OS: the whole CN data folder + CN exe move to backup.
	if err := FinalizeSwitch(dir, edition.Oversea, backupRoot); err != nil {
		t.Fatalf("FinalizeSwitch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "YuanShen.exe")); !os.IsNotExist(err) {
		t.Errorf("YuanShen.exe should be moved away, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "YuanShen_Data")); !os.IsNotExist(err) {
		t.Errorf("YuanShen_Data should be moved away, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "GenshinImpact.exe")); err != nil {
		t.Errorf("GenshinImpact.exe should remain: %v", err)
	}
	// Whole CN folder is preserved in backup/<YuanShen_Data>/YuanShen_Data.
	b := filepath.Join(backupRoot, edition.OfficialCN.DataFolder)
	if _, err := os.Stat(filepath.Join(b, "YuanShen_Data", "StreamingAssets", "shared.blk")); err != nil {
		t.Errorf("CN data should be in backup: %v", err)
	}
	if _, err := os.Stat(filepath.Join(b, "YuanShen.exe")); err != nil {
		t.Errorf("YuanShen.exe should be in backup: %v", err)
	}
}

func TestFinalizeSwitchReverse(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(t.TempDir(), "backup")

	// Both families present (switching OS -> CN).
	os.MkdirAll(filepath.Join(dir, "YuanShen_Data"), 0o755)
	os.WriteFile(filepath.Join(dir, "YuanShen.exe"), []byte("cn-exe"), 0o644)
	os.MkdirAll(filepath.Join(dir, "GenshinImpact_Data"), 0o755)
	os.WriteFile(filepath.Join(dir, "GenshinImpact.exe"), []byte("os-exe"), 0o644)

	// Finalize to CN: whole OS folder + exe move; CN stays.
	if err := FinalizeSwitch(dir, edition.OfficialCN, backupRoot); err != nil {
		t.Fatalf("FinalizeSwitch: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "GenshinImpact.exe")); !os.IsNotExist(err) {
		t.Errorf("GenshinImpact.exe should be moved away, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "GenshinImpact_Data")); !os.IsNotExist(err) {
		t.Errorf("GenshinImpact_Data should be moved away, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "YuanShen.exe")); err != nil {
		t.Errorf("YuanShen.exe should remain for CN target: %v", err)
	}
	b := filepath.Join(backupRoot, edition.Oversea.DataFolder)
	if _, err := os.Stat(filepath.Join(b, "GenshinImpact.exe")); err != nil {
		t.Errorf("GenshinImpact.exe should be in backup: %v", err)
	}
}

func TestRestoreFromBackup(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(t.TempDir(), "backup")
	// OS files in backup/<GenshinImpact_Data>/GenshinImpact_Data (nested).
	b := filepath.Join(backupRoot, edition.Oversea.DataFolder)
	os.MkdirAll(filepath.Join(b, "GenshinImpact_Data", "StreamingAssets"), 0o755)
	os.WriteFile(filepath.Join(b, "GenshinImpact.exe"), []byte("os-exe"), 0o644)
	os.WriteFile(filepath.Join(b, "GenshinImpact_Data", "StreamingAssets", "x.blk"), []byte("X"), 0o644)

	restored, err := RestoreFromBackup(dir, edition.Oversea, backupRoot)
	if err != nil {
		t.Fatalf("RestoreFromBackup: %v", err)
	}
	if !restored {
		t.Fatal("expected restored=true")
	}
	if _, err := os.Stat(filepath.Join(dir, "GenshinImpact.exe")); err != nil {
		t.Errorf("GenshinImpact.exe should be restored: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "GenshinImpact_Data", "StreamingAssets", "x.blk")); err != nil {
		t.Errorf("GenshinImpact_Data content should be restored: %v", err)
	}
}

func TestManagePCSDK(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(t.TempDir(), "backup")
	plugins := filepath.Join(dir, filepath.FromSlash(PCSDK))

	// B服 needs the dll: switch to B服, ensure the plugin dir exists.
	if err := ManagePCSDK(dir, edition.Bilibili, backupRoot); err != nil {
		t.Fatalf("ManagePCSDK bilibili: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(plugins)); err != nil {
		t.Errorf("Plugins dir should exist for B服: %v", err)
	}

	// Put a dll in place (as if downloaded), then switch away -> backup.
	os.MkdirAll(filepath.Dir(plugins), 0o755)
	os.WriteFile(plugins, []byte("sdk"), 0o644)
	if err := ManagePCSDK(dir, edition.OfficialCN, backupRoot); err != nil {
		t.Fatalf("ManagePCSDK official: %v", err)
	}
	if _, err := os.Stat(plugins); !os.IsNotExist(err) {
		t.Errorf("PCGameSDK.dll should be moved away for non-B server: %v", err)
	}
	backupDLL := filepath.Join(backupRoot, filepath.Dir(filepath.FromSlash(PCSDK)), "PCGameSDK.dll")
	if _, err := os.Stat(backupDLL); err != nil {
		t.Errorf("PCGameSDK.dll should be in backup: %v", err)
	}

	// Switch back to B服 -> restore from backup.
	if err := ManagePCSDK(dir, edition.Bilibili, backupRoot); err != nil {
		t.Fatalf("ManagePCSDK bilibili restore: %v", err)
	}
	if _, err := os.Stat(plugins); err != nil {
		t.Errorf("PCGameSDK.dll should be restored for B服: %v", err)
	}
}

func TestConvertInstall(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(t.TempDir(), "backup")

	// Only the OS data folder is present; switching to CN renames it.
	os.MkdirAll(filepath.Join(dir, "GenshinImpact_Data"), 0o755)
	os.WriteFile(filepath.Join(dir, "GenshinImpact.exe"), []byte("os-exe"), 0o644)

	if err := ConvertInstall(dir, edition.OfficialCN, backupRoot); err != nil {
		t.Fatalf("ConvertInstall: %v", err)
	}
	// The data folder is renamed to the CN name; the OS exe moved to backup.
	if _, err := os.Stat(filepath.Join(dir, "YuanShen_Data")); err != nil {
		t.Errorf("GenshinImpact_Data should be renamed to YuanShen_Data: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "GenshinImpact.exe")); !os.IsNotExist(err) {
		t.Errorf("GenshinImpact.exe should be moved to backup: %v", err)
	}
	b := filepath.Join(backupRoot, edition.Oversea.DataFolder)
	if _, err := os.Stat(filepath.Join(b, "GenshinImpact.exe")); err != nil {
		t.Errorf("GenshinImpact.exe should be in backup: %v", err)
	}
}

func TestConvertInstallBothPresent(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(t.TempDir(), "backup")

	// Both data folders present (residue); target is CN.
	os.MkdirAll(filepath.Join(dir, "GenshinImpact_Data"), 0o755)
	os.MkdirAll(filepath.Join(dir, "YuanShen_Data"), 0o755)
	os.WriteFile(filepath.Join(dir, "GenshinImpact_Data", "os_only.dat"), []byte("OS"), 0o644)
	os.WriteFile(filepath.Join(dir, "GenshinImpact.exe"), []byte("os-exe"), 0o644)

	if err := ConvertInstall(dir, edition.OfficialCN, backupRoot); err != nil {
		t.Fatalf("ConvertInstall: %v", err)
	}
	// The target (CN) folder stays; the residue OS folder moves to backup.
	if _, err := os.Stat(filepath.Join(dir, "YuanShen_Data")); err != nil {
		t.Errorf("YuanShen_Data should remain: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "GenshinImpact_Data")); !os.IsNotExist(err) {
		t.Errorf("GenshinImpact_Data residue should be moved to backup: %v", err)
	}
	b := filepath.Join(backupRoot, edition.Oversea.DataFolder)
	if _, err := os.Stat(filepath.Join(b, "GenshinImpact_Data", "os_only.dat")); err != nil {
		t.Errorf("residue file should be in backup: %v", err)
	}
}

func TestManageRootFiles(t *testing.T) {
	dir := t.TempDir()
	backupRoot := filepath.Join(t.TempDir(), "backup")

	// A CN-installed dir (CN anti-cheat files present); switching to oversea.
	for _, f := range edition.OfficialCN.RootFiles {
		os.WriteFile(filepath.Join(dir, f), []byte("cn"), 0o644)
	}

	if err := ManageRootFiles(dir, edition.Oversea, backupRoot); err != nil {
		t.Fatalf("ManageRootFiles oversea: %v", err)
	}
	// CN anti-cheat files must leave the dir (they crash the oversea game).
	for _, f := range edition.OfficialCN.RootFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); !os.IsNotExist(err) {
			t.Errorf("%s should be moved away for oversea: %v", f, err)
		}
		if _, err := os.Stat(filepath.Join(backupRoot, edition.OfficialCN.DataFolder, f)); err != nil {
			t.Errorf("%s should be in CN backup: %v", f, err)
		}
	}

	// Switching back to CN restores them.
	if err := ManageRootFiles(dir, edition.OfficialCN, backupRoot); err != nil {
		t.Fatalf("ManageRootFiles cn restore: %v", err)
	}
	for _, f := range edition.OfficialCN.RootFiles {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("%s should be restored for CN: %v", f, err)
		}
	}
}

func TestFixPkgVersion(t *testing.T) {
	dir := t.TempDir()
	pv := `{"remoteName": "GenshinImpact_Data/Managed/x.dat", "md5": "a", "hash": "b", "fileSize": 1}` + "\n"
	os.WriteFile(filepath.Join(dir, "pkg_version"), []byte(pv), 0o644)

	if err := FixPkgVersion(dir, edition.OfficialCN); err != nil {
		t.Fatalf("FixPkgVersion: %v", err)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "pkg_version"))
	if !bytes.Contains(data, []byte(`"YuanShen_Data/Managed/x.dat"`)) {
		t.Errorf("pkg_version should reference YuanShen_Data, got %q", data)
	}
	if bytes.Contains(data, []byte("GenshinImpact_Data")) {
		t.Errorf("pkg_version should not reference GenshinImpact_Data, got %q", data)
	}
}

func TestEditionDetectionFromChannel(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]string{
		"official_cn": "[general]\nchannel=1\nsub_channel=1\ncps=gw_pc\n",
		"bilibili":    "[general]\nchannel=14\nsub_channel=0\ncps=gw_pc\n",
	}
	for want, cfg := range cases {
		if err := os.WriteFile(filepath.Join(dir, ConfigIni), []byte(cfg), 0o644); err != nil {
			t.Fatal(err)
		}
		if got := New(dir).Edition().Key; got != want {
			t.Errorf("cfg %q: expected %s, got %s", cfg, want, got)
		}
	}
}
