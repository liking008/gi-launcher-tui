package tui

import (
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// pathField describes one editable path setting.
type pathField struct {
	name string
	get  func() string
	set  func(string)
}

// settingsFields lists the editable directory paths in order.
func (m *model) settingsFields() []pathField {
	return []pathField{
		{name: "游戏目录", get: func() string { return m.cfg.GameDir }, set: func(v string) { m.cfg.GameDir = v }},
		{name: "下载目录", get: func() string { return m.cfg.DownloadDir }, set: func(v string) { m.cfg.DownloadDir = v }},
		{name: "版本目录", get: func() string { return m.cfg.VersionDir }, set: func(v string) { m.cfg.VersionDir = v }},
		{name: "工具目录", get: func() string { return m.cfg.ToolsDir }, set: func(v string) { m.cfg.ToolsDir = v }},
	}
}

func (m *model) updateSettings(msg tea.Msg) tea.Cmd {
	// While editing a path, all keys go to the input.
	if m.settingEdit {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "enter":
				return m.savePathField()
			case "esc":
				m.settingEdit = false
				m.settingsInput.Blur()
				return nil
			}
		}
		var cmd tea.Cmd
		m.settingsInput, cmd = m.settingsInput.Update(msg)
		return cmd
	}

	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	switch km.String() {
	case "up", "k":
		if m.settingsSel > 0 {
			m.settingsSel--
		}
	case "down", "j":
		if m.settingsSel < len(m.settingsFields())-1 {
			m.settingsSel++
		}
	case "enter", "e":
		return m.beginEditPath()
	case "s":
		_ = m.cfg.Save()
		return nil
	}
	return nil
}

// beginEditPath focuses the input on the currently selected path field.
func (m *model) beginEditPath() tea.Cmd {
	fields := m.settingsFields()
	if m.settingsSel < 0 || m.settingsSel >= len(fields) {
		return nil
	}
	m.settingEdit = true
	m.settingsInput.Focus()
	m.settingsInput.SetValue(fields[m.settingsSel].get())
	return nil
}

// savePathField persists the edited path and refreshes relevant handles.
func (m *model) savePathField() tea.Cmd {
	fields := m.settingsFields()
	if m.settingsSel < 0 || m.settingsSel >= len(fields) {
		m.settingEdit = false
		return nil
	}
	field := fields[m.settingsSel]
	path := strings.TrimSpace(m.settingsInput.Value())
	m.settingEdit = false
	m.settingsInput.Blur()
	if path == "" {
		return nil
	}
	// The game directory must exist; others are created on demand.
	if field.name == "游戏目录" {
		if fi, err := os.Stat(path); err != nil || !fi.IsDir() {
			return func() tea.Msg {
				return errMsg{err: fmt.Errorf("目录不存在或不可用: %s", path)}
			}
		}
	}
	field.set(path)
	_ = m.cfg.Save()
	return func() tea.Msg {
		return errMsg{err: fmt.Errorf("已更新%s: %s", field.name, path)}
	}
}

func (m *model) viewSettings() string {
	var b strings.Builder
	fields := m.settingsFields()
	b.WriteString("路径设置（↑/↓ 选择 · enter 编辑）\n\n")
	for i, f := range fields {
		mark := "  "
		if i == m.settingsSel {
			mark = accentStyle("▶ ")
		}
		val := f.get()
		if m.settingEdit && i == m.settingsSel {
			b.WriteString(fmt.Sprintf("%s%s:\n     %s\n", mark, f.name, m.settingsInput.View()))
			b.WriteString("     " + hintStyle("输入路径 · enter 保存 · esc 取消") + "\n")
		} else {
			b.WriteString(fmt.Sprintf("%s%s: %s\n", mark, f.name, infoStyle(val)))
		}
	}

	b.WriteString("\n并发数: " + infoStyle(fmt.Sprint(m.cfg.Concurrency)) + "\n")
	b.WriteString("语音包: " + infoStyle(strings.Join(m.cfg.VoicePacks, ", ")) + "\n")
	if m.err != nil {
		b.WriteString("\n" + errStyle(m.err.Error()) + "\n")
	}
	b.WriteString("\n" + hintStyle("enter 编辑所选路径 · s 保存 · esc 返回"))
	return lipglossBox(m.width, titleStyle("设置")+"\n\n"+b.String())
}
