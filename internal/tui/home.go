package tui

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) updateHome(msg tea.Msg) tea.Cmd {
	// esc must never reach the home list, which maps esc to tea.Quit. On the
	// main menu esc is a no-op (only q quits).
	if km, ok := msg.(tea.KeyMsg); ok && km.String() == "esc" {
		return nil
	}
	var cmd tea.Cmd
	m.list, cmd = m.list.Update(msg)

	if km, ok := msg.(tea.KeyMsg); ok {
		switch km.String() {
		case "enter":
			if it, ok := m.list.SelectedItem().(item); ok {
				return m.goMenu(it.key)
			}
		}
	}
	return cmd
}

func (m *model) goMenu(key string) tea.Cmd {
	switch key {
	case "launch":
		m.launching = true
		m.launchExited = false
		m.launchMsg = "正在初始化 wine prefix（首次需 1-2 分钟）..."
		return m.launchGame()
	case "verify":
		m.navigate(pageVerify)
		return m.startVerify()
	case "download":
		m.navigate(pageDownload)
		return m.fetchRemote()
	case "versions":
		m.navigate(pageVersions)
	case "accounts":
		m.navigate(pageAccounts)
	case "settings":
		m.navigate(pageSettings)
	}
	return nil
}

func (m *model) viewHome() string {
	g := m.activeGame()
	version := g.Version()
	installed := "已安装"
	if !g.Installed() {
		installed = "未安装"
	}
	head := titleStyle("原神 启动器 / Genshin Impact Launcher")
	sub := accentStyle("版本 "+version) + "  " + infoStyle(installed) + "  " + infoStyle("["+g.Edition().Name+"]")
	if m.launching {
		sub += "\n" + m.spinner.View() + infoStyle(m.launchMsg)
	} else if m.launchExited {
		sub += "\n" + infoStyle(m.launchMsg)
	}
	if m.err != nil {
		sub += "\n" + errStyle(m.err.Error())
	}
	m.list.Title = ""
	return lipglossBox(m.width, head+"\n\n"+sub+"\n\n"+m.list.View()+"\n"+hintStyle("↑/↓ 选择 · enter 确认 · q 退出"))
}
