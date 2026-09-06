package tui

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/liking008/gi-launcher-tui/internal/edition"
	"github.com/liking008/gi-launcher-tui/internal/resource"
)

// protocolMsg carries the fetched user agreement.
type protocolMsg struct {
	proto *resource.Protocol
	err   error
}

// switchDoneMsg signals that a server switch has been applied; the model then
// auto-runs a tidy (整理).
type switchDoneMsg struct{ text string }

var htmlTag = regexp.MustCompile(`(?s)<[^>]*>`)

// stripHTML converts protocol HTML into readable plain text.
func stripHTML(s string) string {
	s = htmlTag.ReplaceAllString(s, " ")
	s = strings.ReplaceAll(s, "&quot;", `"`)
	s = strings.ReplaceAll(s, "&#34;", `"`)
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "&nbsp;", " ")
	return strings.TrimSpace(s)
}

func (m *model) updateVersions(msg tea.Msg) tea.Cmd {
	// A completed protocol fetch updates the stored agreement.
	if pm, ok := msg.(protocolMsg); ok {
		m.protocol = pm.proto
		if pm.err != nil {
			m.err = pm.err
		}
		return nil
	}

	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}

	if m.verConfirm {
		// In confirm state: enter to confirm, esc to cancel.
		switch km.String() {
		case "enter", "y":
			m.verConfirm = false
			return m.doSwitchEdition()
		case "esc", "n":
			m.verConfirm = false
		}
		return nil
	}

	switch km.String() {
	case "up", "k":
		if m.verSel > 0 {
			m.verSel--
		}
	case "down", "j":
		if m.verSel < len(edition.All())-1 {
			m.verSel++
		}
	case "d":
		// Download/install the selected server's files into its directory.
		return m.goDownloadFor(m.verSel)
	case "t":
		// Immediately move the other server family's files to backup.
		return m.startTidy()
	case "p":
		// Fetch and display the current server's user agreement.
		return m.fetchProtocol()
	case "enter":
		// Confirm the currently selected server.
		sel := edition.All()[m.verSel]
		if sel.Key != m.activeGame().Edition().Key {
			m.verConfirm = true
		}
	}
	return nil
}

// fetchProtocol retrieves the active server's user agreement.
func (m *model) fetchProtocol() tea.Cmd {
	return func() tea.Msg {
		proto, err := m.res.Protocol(context.Background(), m.currentEdition())
		if err != nil {
			return protocolMsg{err: err}
		}
		return protocolMsg{proto: proto}
	}
}

// goDownloadFor navigates to the download page for the edition at index i.
func (m *model) goDownloadFor(i int) tea.Cmd {
	all := edition.All()
	if i < 0 || i >= len(all) {
		return nil
	}
	sel := all[i]
	m.cfg.Edition = sel.Key
	_ = m.cfg.Save()
	m.navigate(pageDownload)
	return m.fetchRemote()
}

// doSwitchEdition performs the confirmed switch to the selected server.
func (m *model) doSwitchEdition() tea.Cmd {
	return func() tea.Msg {
		all := edition.All()
		if m.verSel < 0 || m.verSel >= len(all) {
			return errMsg{err: fmt.Errorf("无效选择")}
		}
		next := all[m.verSel]
		if err := m.cfg.Game(next).SetEdition(next); err != nil {
			return errMsg{err: err}
		}
		m.cfg.Edition = next.Key
		_ = m.cfg.Save()
		msg := fmt.Sprintf("已切换到 %s\n安装目录: %s\nconfig.ini: channel=%s cps=%s sub_channel=%s",
			next.Name, m.cfg.InstallDir(next), next.Channel, next.CPS(), next.SubChannel)
		if !m.cfg.Game(next).Installed() {
			msg += "\n该服务器尚未安装，按 d 可下载并安装到该目录"
		}
		msg += "\n正在自动整理..."
		return switchDoneMsg{text: msg}
	}
}

func (m *model) viewVersions() string {
	var b strings.Builder
	cur := m.activeGame()
	b.WriteString("当前版本: " + accentStyle(cur.Version()) + "\n")
	b.WriteString("当前服务器: " + accentStyle(cur.Edition().Name) + "\n")
	b.WriteString("安装目录: " + infoStyle(cur.Dir) + "\n\n")

	b.WriteString("服务器列表（胡桃工具箱风格，同一目录切换）:\n")
	for i, e := range edition.All() {
		mark := "  "
		if i == m.verSel {
			mark = accentStyle("▶ ")
		}
		extra := ""
		if e.NeedsSDK {
			extra = infoStyle(" +PCGameSDK.dll")
		}
		if e.Key == cur.Edition().Key {
			extra += infoStyle(" (当前)")
		}
		b.WriteString(fmt.Sprintf("%s%s   [channel=%s sub=%s %s]%s\n",
			mark, e.Name, e.Channel, e.SubChannel, infoStyle(e.ExeName), extra))
		b.WriteString(fmt.Sprintf("        %s\n", infoStyle(m.cfg.InstallDir(e))))
	}

	// Display the fetched user agreement.
	if m.protocol != nil {
		b.WriteString("\n" + accentStyle("用户协议: "+m.protocol.Title) +
			"  " + infoStyle("v"+m.protocol.AgreementVersion) + "\n")
		content := stripHTML(m.protocol.Content)
		const maxRunes = 1200
		r := []rune(content)
		if len(r) > maxRunes {
			content = string(r[:maxRunes]) + " ..."
		}
		b.WriteString(infoStyle(content) + "\n")
	}

	// Tidy status (started from this page too).
	if m.tidyRunning {
		b.WriteString("\n" + m.spinner.View() + infoStyle(" 正在整理（恢复备份 → 移走另一版本 → 补齐缺失）...\n"))
	}
	if m.tidyResult != "" {
		b.WriteString("\n" + accentStyle(m.tidyResult) + "\n")
	}
	if m.tidyErr != nil {
		b.WriteString("\n" + errStyle("整理失败: "+m.tidyErr.Error()) + "\n")
	}

	if m.verConfirm {
		sel := edition.All()[m.verSel]
		b.WriteString("\n" + accentStyle("确认切换到 "+sel.Name+" ？") + "  (enter 确认 / esc 取消)\n")
	} else {
		b.WriteString("\n" + hintStyle("↑/↓ 选择服务器 · enter 确认切换 · d 下载并安装 · t 整理到备份 · p 查看用户协议 · esc 返回"))
	}

	if m.err != nil {
		b.WriteString("\n" + errStyle(m.err.Error()) + "\n")
	}
	return lipglossBox(m.width, titleStyle("版本 / 服务器切换")+"\n\n"+b.String())
}
