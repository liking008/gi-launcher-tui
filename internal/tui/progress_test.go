package tui

import (
	"testing"
)

func TestSophonOverallProgress(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	m.dl = &downloadState{step: "running", sophonPlan: true, totItems: 2, compDone: map[string]int64{}}

	// Component A progress.
	m.dl.dldItems = 0
	m.updateDownload(dlProgress{name: "A", done: 100, size: 1000, pct: 10})
	if m.dl.totBytes != 1000 || m.dl.dldBytes != 100 {
		t.Fatalf("after A first: tot=%d dld=%d", m.dl.totBytes, m.dl.dldBytes)
	}
	m.updateDownload(dlProgress{name: "A", done: 500, size: 1000, pct: 50})
	if m.dl.totBytes != 1000 || m.dl.dldBytes != 500 {
		t.Fatalf("after A second: tot=%d dld=%d", m.dl.totBytes, m.dl.dldBytes)
	}

	// Component B starts.
	m.updateDownload(dlProgress{name: "B", done: 200, size: 2000, pct: 10})
	if m.dl.totBytes != 3000 || m.dl.dldBytes != 700 {
		t.Fatalf("after B first: tot=%d dld=%d", m.dl.totBytes, m.dl.dldBytes)
	}

	// Finish both components.
	m.updateDownload(dlFinished{name: "A", total: 1000})
	m.updateDownload(dlFinished{name: "B", total: 2000})
	if m.dl.dldItems != 2 || m.dl.step != "done" {
		t.Fatalf("completion: items=%d step=%s", m.dl.dldItems, m.dl.step)
	}
}

func TestOverallBarRenders(t *testing.T) {
	m, err := New(testConfig(t))
	if err != nil {
		t.Fatal(err)
	}
	m.width = 80
	m.dl = &downloadState{totBytes: 2000, dldBytes: 500}
	out := overallBar(m, m.dl)
	if out == "" {
		t.Fatal("overallBar returned empty")
	}
}
