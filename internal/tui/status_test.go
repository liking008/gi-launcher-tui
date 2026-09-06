package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestStatusClearsOnHome verifies that a transient status message (e.g. the
// server-switch confirmation) is cleared when returning to the home page.
func TestStatusClearsOnHome(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a status shown on the versions page.
	navigateHomeToVersions(t, m)
	m.err = statusErr("已切换到 B服")
	if m.err == nil {
		t.Fatal("precondition: m.err should be set")
	}

	// esc returns home and clears the status.
	m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	if m.page != pageHome {
		t.Fatalf("expected home, got %d", m.page)
	}
	if m.err != nil {
		t.Fatalf("status should be cleared on returning home, got %v", m.err)
	}
}

type statusErr string

func (s statusErr) Error() string { return string(s) }
