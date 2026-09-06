package tui

func (m *model) View() string {
	switch m.page {
	case pageHome:
		return m.viewHome()
	case pageVerify:
		return m.viewVerify()
	case pageDownload:
		return m.viewDownload()
	case pageVersions:
		return m.viewVersions()
	case pageAccounts:
		return m.viewAccounts()
	case pageSettings:
		return m.viewSettings()
	}
	return m.viewHome()
}
