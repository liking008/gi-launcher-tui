package tui

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/liking008/gi-launcher-tui/internal/edition"
	"github.com/liking008/gi-launcher-tui/internal/manifest"
	"github.com/liking008/gi-launcher-tui/internal/sophon"
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
	ed := m.currentEdition()
	hc := m.httpClient()
	return func() tea.Msg {
		// Prefer the remote manifest for the current version: the on-disk
		// pkg_version can be stale (e.g. still 7.0 while the game is 7.2),
		// which made verification flag the wrong files and "repair" download
		// the wrong version. Fall back to the local manifest when offline.
		man, err := m.remoteGameManifest(ed, hc)
		if err != nil {
			man, err = manifest.Load(filepath.Join(g.Dir, "pkg_version"))
			if err != nil {
				return errMsg{err: err}
			}
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

// remoteGameManifest fetches the current edition's game manifest from the
// branch/build API, converted to the pkg_version format used for verification.
func (m *model) remoteGameManifest(ed edition.Edition, hc *http.Client) (*manifest.Manifest, error) {
	ctx := context.Background()
	br, err := m.res.FetchBranch(ctx, ed)
	if err != nil {
		return nil, err
	}
	build, err := sophon.GetBuild(ctx, hc, ed, br.Main.PackageID, br.Main.Password, br.Main.Tag)
	if err != nil {
		return nil, err
	}
	for i := range build.Data.Manifests {
		c := &build.Data.Manifests[i]
		if c.MatchingField != "game" {
			continue
		}
		assets, err := sophon.FetchManifest(ctx, hc, c)
		if err != nil {
			return nil, err
		}
		files := make([]manifest.File, 0, len(assets))
		for _, a := range assets {
			if a.Type == 64 {
				continue
			}
			files = append(files, manifest.File{RemoteName: a.Path, MD5: a.HashMD5, FileSize: a.Size})
		}
		return &manifest.Manifest{Name: "remote-" + br.Main.Tag, Files: files}, nil
	}
	return nil, fmt.Errorf("远端清单中没有游戏资源")
}

func (m *model) updateVerify(msg tea.Msg) tea.Cmd {
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "v":
			return m.startVerify()
		case "d":
			// Repair: download only the files the verification flagged.
			if m.verify != nil && m.verify.result != nil && len(m.verify.result.Failed) > 0 {
				set := make(map[string]bool, len(m.verify.result.Failed))
				for _, fe := range m.verify.result.Failed {
					set[fe.RemoteName] = true
				}
				m.navigate(pageDownload)
				return m.fetchRemoteWith(set)
			}
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
				"%s\n\n  校验基准: %s\n  文件总数: %d\n  正常: %s\n  缺失/损坏: %s\n  需下载大小: %s",
				accentStyle("✓ 校验完成"),
				infoStyle(st.manifest.Name),
				r.Total,
				accentStyle(fmt.Sprint(r.Ok)),
				errStyle(fmt.Sprintf("%d (缺失 %d, 损坏 %d)", len(r.Failed), r.Missing, r.Mismatch)),
				accentStyle(humanBytes(r.Downloaded)),
			)
		}
		body += "\n\n" + hintStyle("按 v 重新校验 · esc 返回")
		if st.result != nil && len(st.result.Failed) > 0 {
			body += "\n" + hintStyle("按 d 只下载这些缺失/损坏的文件（自动进入下载页并开始）")
		}
	}
	head := titleStyle("校验游戏文件")
	return lipglossBox(m.width, head+"\n\n"+body)
}
