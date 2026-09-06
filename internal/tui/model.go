package tui

import (
	"fmt"
	"net/http"
	"os"

	"github.com/charmbracelet/bubbles/list"
	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/liking008/gi-launcher-tui/internal/account"
	"github.com/liking008/gi-launcher-tui/internal/config"
	"github.com/liking008/gi-launcher-tui/internal/game"
	"github.com/liking008/gi-launcher-tui/internal/resource"
)

type page int

const (
	pageHome page = iota
	pageVerify
	pageDownload
	pageVersions
	pageAccounts
	pageSettings
)

type model struct {
	cfg  *config.Config
	game *game.Game
	res  *resource.Client
	acc  *account.Store

	page   page
	width  int
	height int
	err    error

	spinner  spinner.Model
	progress progress.Model
	list     list.Model
	ready    bool

	verify *verifyState
	dl     *downloadState
	dlCh   chan tea.Msg

	// versions page selection + confirm state
	verSel     int
	verConfirm bool
	protocol   *resource.Protocol

	// settings page path editor
	settingsInput textinput.Model
	settingsSel   int
	settingEdit   bool

	// accounts page selection + manual label editor
	accSel   int
	accEdit  bool
	accInput textinput.Model

	// manual tidy (整理) status, shown on any page
	tidyRunning bool
	tidyResult  string
	tidyErr     error

	// launch status feedback
	launching bool
	launchMsg string
	launchCh  chan tea.Msg
}

// Msg types
type (
	errMsg    struct{ err error }
	readyMsg  struct{ version string }
	verifyMsg struct{ done, total int }
)

func New(cfg *config.Config) (*model, error) {
	g := game.New(cfg.GameDir)
	acc := account.NewStore(cfgAccountsPath(cfg), cfg.DownloadDir)
	m := &model{
		cfg:  cfg,
		game: g,
		res:  resource.New(),
		acc:  acc,
	}
	m.spinner = spinner.New()
	m.spinner.Spinner = spinner.Dot
	m.progress = progress.New(progress.WithDefaultGradient())
	m.list = list.New(menuItems(g.Version()), itemDelegate{}, 0, 0)
	m.ready = true
	m.settingsInput = textinput.New()
	m.settingsInput.Placeholder = "/path/to/folder"
	m.settingsInput.Focus()
	m.accInput = textinput.New()
	m.accInput.Placeholder = "账号标签"
	return m, nil
}

func (m *model) Init() tea.Cmd {
	return tea.Batch(m.spinner.Tick, m.refresh())
}

func (m *model) refresh() tea.Cmd {
	return func() tea.Msg {
		g := game.New(m.cfg.GameDir)
		return readyMsg{version: g.Version()}
	}
}

// httpClient returns a shared HTTP client for downloads.
func (m *model) httpClient() *http.Client {
	return &http.Client{Timeout: 0}
}

func (m *model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.progress.Width = msg.Width - 12
		if m.ready {
			m.list.SetSize(msg.Width-8, msg.Height-16)
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			return m, tea.Quit
		case "q":
			// Only quit from the home page; elsewhere return to the home page.
			if m.page == pageHome {
				return m, tea.Quit
			}
			m.page = pageHome
			m.err = nil
			return m, nil
		}
		// page-specific keys
		if cmd := m.handleKey(msg); cmd != nil {
			cmds = append(cmds, cmd)
		}
	case readyMsg:
		m.err = nil
		if m.page == pageHome {
			m.list.SetItems(menuItems(msg.version))
		}
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		cmds = append(cmds, cmd)
	case errMsg:
		m.err = msg.err
	case fetchErr:
		m.err = msg.err
	case tidyMsg:
		m.tidyRunning = false
		m.tidyResult = msg.text
		m.tidyErr = msg.err
		if msg.err != nil {
			m.err = msg.err
		}
	case switchDoneMsg:
		m.err = fmt.Errorf("%s", msg.text)
		return m, m.startTidy()
	case launchDoneMsg:
		m.launching = true
		m.launchMsg = msg.text
		return m, m.subscribeLaunch()
	case launchGameStartedMsg:
		m.launching = true
		m.launchMsg = msg.text
		return m, nil
	case launchGameFailedMsg:
		m.launching = false
		m.launchMsg = ""
		m.err = fmt.Errorf("%s", msg.text)
		return m, nil
	}

	// let page-specific updates run
	if cmd := m.updatePage(msg); cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func menuItems(version string) []list.Item {
	items := []list.Item{
		item{title: "启动游戏", desc: "Launch YuanShen.exe", key: "launch"},
		item{title: "校验游戏文件", desc: "Verify pkg_version vs local files", key: "verify"},
		item{title: "更新 / 下载", desc: "Fetch latest patches or fresh install", key: "download"},
		item{title: "版本切换", desc: "Switch between installed versions", key: "versions"},
		item{title: "账号切换", desc: "Manage and apply login profiles", key: "accounts"},
		item{title: "设置", desc: "Game dir, download dir, voice packs", key: "settings"},
	}
	if version != "" {
		_ = version
	}
	return items
}

func cfgAccountsPath(cfg *config.Config) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return fmt.Sprintf("%s/.config/gi-launcher/accounts.json", home)
}
