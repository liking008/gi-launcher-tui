package tui

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestLaunchGameExecutesCommand(t *testing.T) {
	cfg := testConfig(t)
	os.WriteFile(filepath.Join(cfg.GameDir, "YuanShen.exe"), []byte("fake"), 0o644)
	cfg.WinePrefix = filepath.Join(cfg.GameDir, "prefix")

	marker := filepath.Join(cfg.GameDir, "proton_ran")
	proton := filepath.Join(cfg.GameDir, "fake-proton")
	script := "#!/bin/sh\necho proton-started\necho \"$@\" > " + marker + "\nwhile true; do sleep 0.1; done\n"
	os.WriteFile(proton, []byte(script), 0o755)
	t.Setenv("DWPROTON", proton)

	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter}) // select first item = launch
	if cmd == nil {
		t.Fatal("expected a cmd from launch")
	}
	msg := cmd()
	if _, ok := msg.(launchDoneMsg); !ok {
		t.Fatalf("expected launchDoneMsg, got %#v", msg)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			data, _ := os.ReadFile(marker)
			t.Logf("proton executed with args: %s", data)
			time.Sleep(500 * time.Millisecond)
			logData, logErr := os.ReadFile(filepath.Join(cfg.GameDir, "launcher_launch.log"))
			t.Logf("log len=%d, err=%v", len(logData), logErr)
			if len(logData) == 0 {
				t.Fatal("launch log is empty: child stdout/stderr fds were closed before Start")
			}
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("proton script was never executed")
}
