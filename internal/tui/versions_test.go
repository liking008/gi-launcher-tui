package tui

import (
	"os"
	"path/filepath"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/liking008/gi-launcher-tui/internal/config"
)

// testConfig returns a config bound to temp dirs so tests never touch the real
// game installation.
func testConfig(t *testing.T) *config.Config {
	t.Helper()
	root := t.TempDir()
	os.MkdirAll(root, 0o755)
	// Route config writes to the temp dir so tests never touch the real
	// ~/.config/gi-launcher/config.json.
	t.Setenv("GIH_LANCHER_CONFIG", filepath.Join(root, "config.json"))
	return &config.Config{
		GameDir:     root,
		DownloadDir: filepath.Join(root, "dl"),
		VersionDir:  filepath.Join(root, "versions"),
		VoicePacks:  []string{"zh-cn"},
		Concurrency: 4,
		Edition:     "official_cn",
		Installs: map[string]string{
			"oversea": filepath.Join(root, "os-install"),
		},
		ToolsDir: filepath.Join(root, "tools"),
	}
}

// navigateHomeToVersions presses down+enter to open the versions page.
func navigateHomeToVersions(t *testing.T, m *model) {
	t.Helper()
	for i := 0; i < 3; i++ {
		_, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	}
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if m.page != pageVersions {
		t.Fatalf("expected pageVersions, got %d", m.page)
	}
}

func TestVersionSwitchRequiresConfirm(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	navigateHomeToVersions(t, m)

	// Select 国际服 (index 2) and press enter to request a switch.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.verConfirm {
		t.Fatal("expected verConfirm=true after selecting a different server and pressing enter")
	}

	// Cancel the confirm -> no switch happens.
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.verConfirm {
		t.Fatal("expected verConfirm=false after cancel")
	}
	if m.cfg.Edition != "official_cn" {
		t.Fatalf("edition should not change after cancel, got %s", m.cfg.Edition)
	}
}

func TestVersionSwitchConfirmExecutes(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	navigateHomeToVersions(t, m)

	// Select 国际服 and confirm.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})           // confirm prompt
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // confirm switch

	if msg := cmd(); msg == nil {
		t.Fatal("expected a result message from switch")
	}
	if m.cfg.Edition != "oversea" {
		t.Fatalf("expected edition oversea after confirm, got %s", m.cfg.Edition)
	}
	// The new install dir should have been created with a config.ini.
	if _, err := os.Stat(filepath.Join(m.cfg.InstallDir(m.currentEdition()), "config.ini")); err != nil {
		t.Fatalf("oversea config.ini not created: %v", err)
	}
}
