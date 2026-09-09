package tui

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/list"
	tea "github.com/charmbracelet/bubbletea"
)

// handleKey translates keyboard input into page navigation.
func (m *model) handleKey(msg tea.KeyMsg) tea.Cmd {
	if m.page != pageHome {
		// When confirming a server switch, esc cancels the confirm instead of
		// leaving the page.
		if m.verConfirm || m.accEdit {
			return nil
		}
		// non-home pages: esc returns home and clears any transient status.
		if msg.String() == "esc" {
			m.page = pageHome
			m.err = nil
			return nil
		}
	}
	return nil
}

// updatePage routes page-specific messages/keys.
func (m *model) updatePage(msg tea.Msg) tea.Cmd {
	switch m.page {
	case pageHome:
		return m.updateHome(msg)
	case pageVerify:
		return m.updateVerify(msg)
	case pageDownload:
		return m.updateDownload(msg)
	case pageVersions:
		return m.updateVersions(msg)
	case pageAccounts:
		return m.updateAccounts(msg)
	case pageSettings:
		return m.updateSettings(msg)
	}
	return nil
}

// navigate switches to a page, initialising list selection.
func (m *model) navigate(p page) tea.Cmd {
	m.page = p
	if p == pageHome {
		m.err = nil
	}
	switch p {
	case pageHome, pageAccounts:
		if !m.ready {
			m.list = list.New(menuItems(m.game.Version()), itemDelegate{}, 0, 0)
			m.ready = true
			m.list.SetSize(m.width-8, m.height-16)
		}
	}
	return nil
}

// launchDoneMsg updates the in-progress launch message.
type launchDoneMsg struct{ text string }
type launchGameStartedMsg struct{ text string }
type launchGameFailedMsg struct{ text string }
type launchGameExitedMsg struct{ text string }

// gameProcessRunning reports whether the game executable is running.
func gameProcessRunning(exeName string) bool {
	out, err := exec.Command("pgrep", "-f", exeName).Output()
	return err == nil && len(strings.Fields(string(out))) > 0
}

// reachableHTTPS reports whether an HTTPS request to host completes within a
// short timeout. Any HTTP status counts as reachable; only transport errors
// (DNS/TCP/TLS failure) report unreachable.
func reachableHTTPS(host string) bool {
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get("https://" + host)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return true
}

// watchGameExit keeps polling the game process after it started and reports a
// launchGameExitedMsg once it has stayed gone for a while (tolerating brief
// restarts such as the game relaunching itself through its anti-cheat).
func watchGameExit(ch chan<- tea.Msg, exeName string) {
	const poll = 3 * time.Second
	const confirm = 6 // 6 consecutive misses (~18s) before declaring exit
	misses := 0
	for {
		time.Sleep(poll)
		if gameProcessRunning(exeName) {
			misses = 0
			continue
		}
		misses++
		if misses >= confirm {
			ch <- launchGameExitedMsg{text: "游戏已退出"}
			return
		}
	}
}

// subscribeLaunch waits for the game-process detector result.
func (m *model) subscribeLaunch() tea.Cmd {
	return func() tea.Msg {
		if m.launchCh == nil {
			return launchGameFailedMsg{text: "启动通道未初始化"}
		}
		msg, ok := <-m.launchCh
		if !ok {
			return launchGameFailedMsg{text: "启动检测结束"}
		}
		return msg
	}
}

// launchGame starts the game executable, referencing an-anime-game-launcher's
// method: use the system dwproton (not downloaded), create a wine prefix under
// the launcher data folder, and pass the same env vars / launch args. The
// proton process is started in a goroutine that also detects when the game
// process appears, streaming progress messages back to the UI.
func (m *model) launchGame() tea.Cmd {
	exe := m.activeGame().ExePath()
	exeName := filepath.Base(exe)
	gameDir := filepath.Dir(exe)

	m.launchCh = make(chan tea.Msg, 4)
	ch := m.launchCh
	go func() {
		defer close(ch)
		if runtime.GOOS == "windows" {
			cmd := exec.Command("cmd", "/c", "start", "", exe)
			cmd.Dir = gameDir
			if err := cmd.Start(); err != nil {
				ch <- launchGameFailedMsg{text: err.Error()}
				return
			}
			ch <- launchGameStartedMsg{text: "已启动 " + exeName}
			return
		}

		// Kill stale wine/lutris processes from previous attempts.
		killStaleGameProcesses()

		// The anti-cheat driver performs a network handshake at startup; warn
		// immediately if the required host is unreachable instead of waiting
		// for the process-detection timeout.
		if host := m.currentEdition().HandshakeHost(); host != "" {
			if !reachableHTTPS(host) {
				ch <- launchDoneMsg{text: "警告: 反作弊握手服务器 " + host +
					" 无法连接。国际服启动会因 initDriver Failed 中止，请检查网络/" +
					"代理/VPN 路由后重试（CN 服务器不依赖该地址）。"}
			}
		}

		// System dwproton (a Proton build with Dawn Winery gacha-game fixes).
		proton := findDwproton()
		if proton != "" {
			prefix := m.cfg.WinePrefix
			if err := os.MkdirAll(prefix, 0o755); err != nil {
				ch <- launchGameFailedMsg{text: err.Error()}
				return
			}
			// aagl-style launch args: borderless + popup window. The
			// "waitforexitandrun" verb (not "run") is required so dwproton's
			// game-specific protonfixes execute (check_conditions() demands it).
			args := []string{"waitforexitandrun", exe, "-screen-fullscreen", "0", "-popupwindow"}
			cmd := exec.Command(proton, args...)
			cmd.Dir = gameDir
			// Run in a new session so the game is fully detached from the TUI's
			// raw-mode terminal (avoids proton/wine hanging on the TUI stdin).
			cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
			cmd.Stdin = nil
			steamRoot := filepath.Dir(filepath.Dir(filepath.Dir(proton)))
			env := append(os.Environ(),
				"WINEARCH=win64",
				"WINEPREFIX="+prefix,
				"STEAM_COMPAT_DATA_PATH="+prefix,
				"STEAM_COMPAT_CLIENT_INSTALL_PATH="+steamRoot,
				"WINE_ENABLE_TIMEOUT_FIX=1",
				"PROTON_USE_WINED3D=0",
			)
			// Let dwproton apply the game-specific fixes (umu-genshin makes it
			// run GenshinImpact.exe from its steam.exe shim for the anti-cheat
			// driver emulation).
			if umuID := m.currentEdition().UMUID(); umuID != "" {
				env = append(env, "UMU_ID="+umuID)
			}
			cmd.Env = env
			logPath := filepath.Join(gameDir, "launcher_launch.log")
			var logFile *os.File
			if f, err := os.Create(logPath); err == nil {
				logFile = f
				cmd.Stdout = f
				cmd.Stderr = f
			}
			if err := cmd.Start(); err != nil {
				ch <- launchGameFailedMsg{text: err.Error()}
				return
			}
			if logFile != nil {
				logFile.Close()
			}
			ch <- launchDoneMsg{text: "正在启动游戏（窗口可能需要一些时间出现）..."}

			// Detect when the game process actually starts.
			for i := 0; i < 180; i++ {
				if gameProcessRunning(exeName) {
					ch <- launchGameStartedMsg{text: fmt.Sprintf(
						"游戏进程已启动（%s）\n窗口可能需要一些时间出现\nprefix: %s\n日志: %s",
						exeName, prefix, logPath)}
					// Keep watching so the UI reflects when the game exits.
					watchGameExit(ch, exeName)
					return
				}
				time.Sleep(1 * time.Second)
			}
			ch <- launchGameFailedMsg{text: "超时：未检测到游戏进程，请查看日志 " + logPath}
			return
		}

		// Fallback: system wine.
		if wine, err := exec.LookPath("wine"); err == nil {
			cmd := exec.Command(wine, exe)
			cmd.Dir = gameDir
			cmd.Env = append(os.Environ(), "WINE_ENABLE_TIMEOUT_FIX=1")
			if err := cmd.Start(); err != nil {
				ch <- launchGameFailedMsg{text: err.Error()}
				return
			}
			ch <- launchGameStartedMsg{text: "已尝试用 wine 启动 " + exeName}
			return
		}
		ch <- launchGameFailedMsg{text: "未找到 dwproton/wine，请手动运行 " + exe}
	}()
	return m.subscribeLaunch()
}

// killStaleGameProcesses terminates leftover wine/lutris/proton processes from
// previous launch attempts that can block a new launch.
func killStaleGameProcesses() {
	patterns := []string{"lutris run", "YuanShen.exe", "GenshinImpact.exe"}
	for _, pat := range patterns {
		out, err := exec.Command("pgrep", "-f", pat).Output()
		if err != nil {
			continue
		}
		for _, f := range strings.Fields(string(out)) {
			if pid, err := strconv.Atoi(f); err == nil && pid > 1 {
				if p, err := os.FindProcess(pid); err == nil {
					_ = p.Kill()
				}
			}
		}
	}
}

// findDwproton locates the system-installed dwproton Proton script.
func findDwproton() string {
	if p := os.Getenv("DWPROTON"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	candidates := []string{
		"/usr/share/steam/compatibilitytools.d/dwproton/proton",
		"/opt/dwproton/proton",
		"/usr/lib/proton-dwproton/proton",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}
