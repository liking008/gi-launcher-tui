package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/liking008/gi-launcher-tui/internal/account"
	"github.com/liking008/gi-launcher-tui/internal/edition"
)

func (m *model) updateAccounts(msg tea.Msg) tea.Cmd {
	// While editing a label, all keys go to the input.
	if m.accEdit {
		switch km := msg.(type) {
		case tea.KeyMsg:
			switch km.String() {
			case "enter":
				return m.saveAccountLabel()
			case "esc":
				m.accEdit = false
				m.accInput.Blur()
				return nil
			}
		}
		var cmd tea.Cmd
		m.accInput, cmd = m.accInput.Update(msg)
		return cmd
	}

	km, ok := msg.(tea.KeyMsg)
	if !ok {
		return nil
	}
	profs := m.acc.All()
	switch km.String() {
	case "up", "k":
		if m.accSel > 0 {
			m.accSel--
		}
	case "down", "j":
		if m.accSel < len(profs)-1 {
			m.accSel++
		}
	case "a":
		// save the currently-logged-in account (from the prefix registry)
		// as a new profile record.
		return m.saveCurrentAccount()
	case "e":
		return m.beginEditAccount()
	case "d":
		return m.deleteAccount()
	case "enter":
		return m.applyAccount()
	}
	return nil
}

// registryTarget returns the prefix path and the registry key/value that hold
// the current edition's auto-login blob.
func (m *model) registryTarget() (prefix, key, value string, err error) {
	ed, err := edition.Get(m.cfg.Edition)
	if err != nil {
		return "", "", "", err
	}
	key, value = ed.RegistryKeyValue()
	return m.cfg.WinePrefix, key, value, nil
}

// saveCurrentAccount captures the MIHOYOSDK blob currently in the prefix and
// stores it as a new account record, mirroring Snap.Hutao's save flow.
func (m *model) saveCurrentAccount() tea.Cmd {
	return func() tea.Msg {
		prefix, key, value, err := m.registryTarget()
		if err != nil {
			return errMsg{err: err}
		}
		blob, ok := account.ReadRegistryBinary(prefix, key, value)
		if !ok {
			return errMsg{err: fmt.Errorf("当前 prefix 未检测到登录数据，请先在游戏中登录一次")}
		}
		name := m.nextAccountName()
		if err := m.acc.Upsert(account.Profile{Name: name, MihoyoSDK: blob}); err != nil {
			return errMsg{err: err}
		}
		return errMsg{err: fmt.Errorf("已保存当前账号为 %s，按 e 可修改标签", name)}
	}
}

func (m *model) nextAccountName() string {
	for i := 1; ; i++ {
		name := fmt.Sprintf("账号%d", i)
		if _, ok := m.acc.Get(name); !ok {
			return name
		}
	}
}

func (m *model) selectedProfile() (account.Profile, bool) {
	profs := m.acc.All()
	if m.accSel < 0 || m.accSel >= len(profs) {
		return account.Profile{}, false
	}
	return profs[m.accSel], true
}

func (m *model) beginEditAccount() tea.Cmd {
	p, ok := m.selectedProfile()
	if !ok {
		return nil
	}
	m.accEdit = true
	m.accInput.Focus()
	m.accInput.SetValue(p.Name)
	return nil
}

func (m *model) saveAccountLabel() tea.Cmd {
	p, ok := m.selectedProfile()
	label := strings.TrimSpace(m.accInput.Value())
	m.accEdit = false
	m.accInput.Blur()
	if !ok || label == "" || label == p.Name {
		return nil
	}
	if err := m.acc.Rename(p.Name, label); err != nil {
		return func() tea.Msg { return errMsg{err: err} }
	}
	return func() tea.Msg { return errMsg{err: fmt.Errorf("已将账号重命名为 %s", label)} }
}

func (m *model) deleteAccount() tea.Cmd {
	p, ok := m.selectedProfile()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		if err := m.acc.Remove(p.Name); err != nil {
			return errMsg{err: err}
		}
		if m.accSel > 0 {
			m.accSel--
		}
		if m.cfg.ActiveAccount == p.Name {
			m.cfg.ActiveAccount = ""
			_ = m.cfg.Save()
		}
		return errMsg{err: fmt.Errorf("已删除账号 %s", p.Name)}
	}
}

func (m *model) applyAccount() tea.Cmd {
	p, ok := m.selectedProfile()
	if !ok {
		return nil
	}
	return func() tea.Msg {
		prefix, key, value, err := m.registryTarget()
		if err != nil {
			return errMsg{err: err}
		}
		if err := m.acc.Apply(prefix, key, value, p); err != nil {
			return errMsg{err: err}
		}
		m.cfg.ActiveAccount = p.Name
		_ = m.cfg.Save()
		return errMsg{err: fmt.Errorf("已应用账号 %s，下次启动即生效", p.Name)}
	}
}

func (m *model) viewAccounts() string {
	var b strings.Builder
	b.WriteString("账号列表（当前: " + accentStyle(m.cfg.ActiveAccount) + "）\n\n")
	profs := m.acc.All()
	if len(profs) == 0 {
		b.WriteString("  " + infoStyle("（无账号，请先在游戏中登录后按 a 保存）") + "\n")
	}
	for i, p := range profs {
		mark := "  "
		if i == m.accSel {
			mark = accentStyle("▶ ")
		}
		has := ""
		if p.MihoyoSDK == "" {
			has = infoStyle(" (无登录数据) ")
		}
		uid := p.UID
		if uid == "" {
			uid = "-"
		}
		b.WriteString(fmt.Sprintf("%s%s  UID: %s%s\n", mark, p.Name, uid, has))
	}
	if m.accEdit {
		p, _ := m.selectedProfile()
		b.WriteString("\n" + accentStyle("修改标签 "+p.Name+":") + "\n")
		b.WriteString("     " + m.accInput.View() + "\n")
		b.WriteString("     " + hintStyle("输入标签 · enter 保存 · esc 取消") + "\n")
	} else {
		b.WriteString("\n" + hintStyle("↑/↓ 选择账号 · enter 应用 · a 保存当前账号 · e 改标签 · d 删除 · esc 返回"))
	}
	if m.err != nil {
		b.WriteString("\n" + errStyle(m.err.Error()) + "\n")
	}
	return lipglossBox(m.width, titleStyle("账号切换")+"\n\n"+b.String())
}
