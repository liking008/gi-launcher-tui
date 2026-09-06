package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/liking008/gi-launcher-tui/internal/edition"
)

func TestTidyIncompleteKeepsOther(t *testing.T) {
	cfg := testConfig(t)
	root := cfg.GameDir
	// Only the OS data folder is present (from a previous 国际服 session);
	// switching to 官服 (CN) renames it to YuanShen_Data and moves OS-unique
	// files to backup.
	os.MkdirAll(filepath.Join(root, "GenshinImpact_Data"), 0o755)
	os.WriteFile(filepath.Join(root, "GenshinImpact.exe"), []byte("os"), 0o644)
	os.WriteFile(filepath.Join(root, "GenshinImpact_Data", "os_only.dat"), []byte("OS"), 0o644)

	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.cfg.Edition = edition.OfficialCN.Key
	msg := m.tidyNow()()

	tm, ok := msg.(tidyMsg)
	if !ok {
		t.Fatalf("expected tidyMsg, got %T", msg)
	}
	// The data folder is renamed to the target (CN) name.
	if _, err := os.Stat(filepath.Join(root, "YuanShen_Data")); err != nil {
		t.Errorf("data folder should be renamed to YuanShen_Data: %v", err)
	}
	// The OS exe moved to backup; unknown files (e.g. hot-updated/foreign) are
	// preserved in the data folder, like in-game hot updates.
	if _, err := os.Stat(filepath.Join(root, "GenshinImpact.exe")); !os.IsNotExist(err) {
		t.Errorf("GenshinImpact.exe should be moved away: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "YuanShen_Data", "os_only.dat")); err != nil {
		t.Errorf("unknown file should be kept (not re-downloaded): %v", err)
	}
	// The report should mention the remaining missing data for the target.
	if !strings.Contains(tm.text, "仍缺") {
		t.Errorf("expected '仍缺' in tidy report, got: %q", tm.text)
	}
}
