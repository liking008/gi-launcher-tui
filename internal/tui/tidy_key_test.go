package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestTidyKeyFeedback verifies pressing t on the download page immediately
// enters the tidy-running state (visible feedback) and returns a command.
func TestTidyKeyFeedback(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	m.page = pageDownload
	m.dl = &downloadState{step: "ready", sophonPlan: true}

	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("t")})
	if !m.tidyRunning {
		t.Fatal("expected tidyRunning=true immediately after pressing t")
	}
	if cmd == nil {
		t.Fatal("expected a tidy cmd from pressing t")
	}
	// The cmd should eventually produce a tidyMsg.
	msg := cmd()
	tidyMsg, ok := msg.(tidyMsg)
	if !ok {
		t.Fatalf("expected tidyMsg, got %T", msg)
	}
	_ = tidyMsg
}
