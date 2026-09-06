package tui

import (
	"fmt"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/liking008/gi-launcher-tui/internal/manifest"
)

func lipglossBox(width int, content string) string {
	return lipgloss.NewStyle().
		Width(width).
		Padding(1, 2).
		Render(content)
}

type verifyState struct {
	manifest *manifest.Manifest
	step     string // "loading" | "hashing" | "done"
	done     int
	total    int
	result   *manifest.VerifyResult
}

func (m *model) startVerify() tea.Cmd {
	m.verify = &verifyState{step: "loading"}
	g := m.activeGame()
	return func() tea.Msg {
		man, err := manifest.Load(filepath.Join(g.Dir, "pkg_version"))
		if err != nil {
			return errMsg{err: err}
		}
		m.verify.manifest = man
		m.verify.total = len(man.Files)
		m.verify.step = "hashing"
		res := man.Verify(g.Dir, m.cfg.Concurrency, func(done, total int) {
			m.verify.done = done
		})
		m.verify.result = res
		m.verify.step = "done"
		return verifyMsg{done: res.Ok + len(res.Failed), total: res.Total}
	}
}

func (m *model) updateVerify(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "v":
			return m.startVerify()
		}
	}
	return nil
}

func (m *model) viewVerify() string {
	st := m.verify
	if st == nil {
		st = &verifyState{step: "loading"}
	}
	var body string
	switch st.step {
	case "loading":
		body = m.spinner.View() + " 读取 pkg_version 清单..."
	case "hashing":
		body = m.spinner.View() + fmt.Sprintf(" 校验中 %d / %d ...", st.done, st.total)
	case "done":
		r := st.result
		if r != nil {
			body = fmt.Sprintf(
				"%s\n\n  文件总数: %d\n  正常: %s\n  缺失/损坏: %s\n  需下载大小: %s",
				accentStyle("✓ 校验完成"),
				r.Total,
				accentStyle(fmt.Sprint(r.Ok)),
				errStyle(fmt.Sprintf("%d (缺失 %d, 损坏 %d)", len(r.Failed), r.Missing, r.Mismatch)),
				accentStyle(humanBytes(r.Downloaded)),
			)
		}
		body += "\n\n" + hintStyle("按 v 重新校验 · esc 返回")
	}
	head := titleStyle("校验游戏文件")
	return lipglossBox(m.width, head+"\n\n"+body)
}
