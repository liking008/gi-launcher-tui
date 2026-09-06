package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestQReturnsHomeFromOtherPages(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	navigateHomeToVersions(t, m)
	if m.page != pageVersions {
		t.Fatalf("expected pageVersions, got %d", m.page)
	}

	// Press q on the versions page -> back to home, not quit.
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if m.page != pageHome {
		t.Fatalf("expected pageHome after q, got %d", m.page)
	}
	if cmd != nil {
		t.Fatalf("expected no quit cmd on non-home page, got %v", cmd)
	}
}

func TestQQuitsFromHome(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	if m.page != pageHome {
		t.Fatalf("expected pageHome, got %d", m.page)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	if cmd == nil {
		t.Fatal("expected quit cmd from home")
	}
	// The cmd should be tea.Quit.
	msg := cmd()
	if msg != (tea.QuitMsg{}) {
		t.Fatalf("expected tea.QuitMsg, got %#v", msg)
	}
}
