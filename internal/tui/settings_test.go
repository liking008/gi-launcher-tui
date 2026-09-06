package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestSettingsSaveGameDir(t *testing.T) {
	cfg := testConfig(t)
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.navigate(pageSettings)

	// Press enter to begin editing the selected path field (游戏目录 is index 0).
	_, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.settingEdit {
		t.Fatal("expected settingEdit after pressing enter")
	}
	// Replace the prefilled value with the temp dir.
	m.settingsInput.SetValue(cfg.GameDir)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	if msg := cmd(); msg == nil {
		t.Fatal("expected save result message")
	}
	if m.settingEdit {
		t.Fatal("expected settingEdit false after save")
	}
	if m.cfg.GameDir != cfg.GameDir {
		t.Fatalf("game dir not updated: %s", m.cfg.GameDir)
	}
}

func TestSettingsEditDownloadDir(t *testing.T) {
	cfg := testConfig(t)
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	m.navigate(pageSettings)

	// Move to 下载目录 (index 1) and edit it.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if !m.settingEdit {
		t.Fatal("expected settingEdit")
	}
	newDL := cfg.DownloadDir + "-new"
	m.settingsInput.SetValue(newDL)
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if msg := cmd(); msg == nil {
		t.Fatal("expected save message")
	}
	if m.cfg.DownloadDir != newDL {
		t.Fatalf("download dir not updated: %s", m.cfg.DownloadDir)
	}
}

func TestVersionsGoDownloadFor(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	navigateHomeToVersions(t, m)

	// Select 国际服 and press d to download for it.
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m.Update(tea.KeyMsg{Type: tea.KeyDown})
	cmd := m.updateVersions(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})

	if cmd == nil {
		t.Fatal("expected a download cmd")
	}
	if m.page != pageDownload {
		t.Fatalf("expected pageDownload, got %d", m.page)
	}
	if m.cfg.Edition != "oversea" {
		t.Fatalf("expected edition oversea, got %s", m.cfg.Edition)
	}
}
