package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/progress"

	"github.com/liking008/gi-launcher-tui/internal/resource"
)

// branchInfo renders the authoritative version + pre-download status.
func branchInfo(b *resource.Branch) string {
	var out strings.Builder
	out.WriteString(infoStyle("权威版本: " + b.Main.Tag))
	if len(b.Main.DiffTags) > 0 {
		out.WriteString("  (可从 " + infoStyle(strings.Join(b.Main.DiffTags, ", ")) + " 更新)")
	}
	if b.PreDownload != nil && b.PreDownload.Tag != "" {
		out.WriteString("\n" + accentStyle("预下载可用: "+b.PreDownload.Tag))
	} else {
		out.WriteString("\n" + infoStyle("预下载: 无"))
	}
	return out.String()
}

func (m *model) viewDownload() string {
	st := m.dl
	head := titleStyle("更新 / 下载")
	var body strings.Builder
	if st == nil {
		body.WriteString(m.spinner.View() + " 获取服务器资源...\n")
		return lipglossBox(m.width, head+"\n\n"+body.String())
	}

	switch st.step {
	case "fetching":
		body.WriteString(m.spinner.View() + " 查询启动器资源 API...\n")
	case "ready":
		body.WriteString(accentStyle(st.plan) + "\n")
		if st.branch != nil {
			body.WriteString(branchInfo(st.branch) + "\n")
		}
		body.WriteString(infoStyle("安装目录: "+st.installDir) + "\n\n")
		if m.tidyRunning {
			body.WriteString(m.spinner.View() + infoStyle(" 正在整理（恢复备份 → 扫描缺失 → 移走另一版本）...\n"))
		}
		if m.tidyResult != "" {
			body.WriteString(accentStyle(m.tidyResult) + "\n")
		}
		if m.tidyErr != nil {
			body.WriteString(errStyle("整理失败: "+m.tidyErr.Error()) + "\n")
		}
		if st.sophonPlan {
			for _, c := range st.sophonComps {
				body.WriteString(fmt.Sprintf("  • %s\n", infoStyle(c.CategoryName)))
			}
			body.WriteString("\n" + hintStyle("sophon 分块模式 · 按 d 开始下载 · t 整理(移走另一版本) · r 重新获取 · esc 返回"))
			return lipglossBox(m.width, head+"\n\n"+body.String())
		}
		for _, it := range st.items {
			body.WriteString(fmt.Sprintf("  • %s  %s\n", it.name, infoStyle(humanBytes(it.size))))
		}
		body.WriteString("\n" + hintStyle("按 d 开始下载 · r 重新获取 · esc 返回"))
	case "running":
		body.WriteString(accentStyle(st.plan) + "\n")
		if st.branch != nil {
			body.WriteString(branchInfo(st.branch) + "\n")
		}
		body.WriteString(infoStyle("安装目录: "+st.installDir) + "\n\n")

		// Overall progress bar.
		body.WriteString(overallBar(m, st) + "\n\n")

		if st.indexing {
			body.WriteString(m.spinner.View() + infoStyle(fmt.Sprintf(" 正在建立本地内容索引，以复用已有文件 (已扫描 %d 个文件)...", st.idxFiles)) + "\n")
			body.WriteString(infoStyle("这只需一次，用于切换服务器时只下载差异部分") + "\n")
			body.WriteString("\n" + hintStyle("esc 返回"))
			return lipglossBox(m.width, head+"\n\n"+body.String())
		}

		if st.sophonPlan {
			body.WriteString(m.progress.ViewAs(st.curPct/100) + "\n")
			body.WriteString(infoStyle(fmt.Sprintf("  正在下载: %s  %s / %s  (%.1f%%)",
				st.curName, humanBytes(st.curDone), humanBytes(st.curSize), st.curPct)) + "\n\n")
			for i, c := range st.sophonComps {
				body.WriteString(compLine(st, i, c.CategoryName) + "\n")
			}
			body.WriteString("\n" + hintStyle(fmt.Sprintf("总体 %d/%d 组件 · 速度 %s/s · esc 返回",
				st.dldItems, st.totItems, humanBytes(st.speed))))
			return lipglossBox(m.width, head+"\n\n"+body.String())
		}

		if st.curName != "" {
			body.WriteString(fmt.Sprintf("  正在下载: %s\n", st.curName))
			p := progress.New(progress.WithDefaultGradient())
			p.Width = m.width - 14
			body.WriteString(p.ViewAs(st.curPct/100) + "\n")
			if st.curSize > 0 {
				body.WriteString(infoStyle(fmt.Sprintf("  %s / %s\n",
					humanBytes(st.curDone), humanBytes(st.curSize))))
			}
		}
		body.WriteString("\n")
		for _, it := range st.items {
			body.WriteString(statusLine(it, m) + "\n")
		}
	case "done":
		body.WriteString(accentStyle("✓ 全部完成") + "\n\n")
		if st.sophonPlan {
			for i, c := range st.sophonComps {
				body.WriteString(compLine(st, i, c.CategoryName) + "\n")
			}
			if st.restored {
				body.WriteString("\n" + accentStyle("已从备份恢复目标服务器文件"))
			}
			switch st.installStep {
			case "installing":
				body.WriteString("\n" + m.spinner.View() + infoStyle(" 正在将旧版本多余文件移至备份目录..."))
			case "verified", "done":
				body.WriteString("\n" + accentStyle(fmt.Sprintf("✓ 完整性校验：%d/%d 个文件完整", st.verifiedOK, st.verifyTotal)))
				if st.verifyMissing > 0 {
					body.WriteString("\n" + errStyle(fmt.Sprintf("  仍缺 %s（可再次下载补齐）", humanBytes(st.verifyMissing))))
				}
				if st.installStep == "done" {
					body.WriteString("\n" + accentStyle("✓ 已整理：另一版本文件移至 "+m.cfg.DownloadDir+"/backup"))
				}
			case "failed":
				body.WriteString("\n" + errStyle("失败: "+st.installErr.Error()))
			default:
				body.WriteString("\n" + hintStyle("将把另一版本的 exe/数据目录移至备份"))
			}
			body.WriteString("\n" + hintStyle("文件已下载到 "+st.installDir+" · 可到「校验」页对比 · esc 返回"))
			return lipglossBox(m.width, head+"\n\n"+body.String())
		}
		for _, it := range st.items {
			body.WriteString(statusLine(it, m) + "\n")
		}
		switch st.installStep {
		case "installing":
			if st.isFull {
				body.WriteString("\n" + m.spinner.View() + infoStyle(" 正在解压安装到 "+st.installDir+" ..."))
			} else {
				body.WriteString("\n" + m.spinner.View() + infoStyle(" 正在应用增量补丁到 "+st.installDir+" ..."))
			}
		case "done":
			if st.isFull {
				body.WriteString("\n" + accentStyle("✓ 已安装到 "+st.installDir))
			} else {
				body.WriteString("\n" + accentStyle("✓ 补丁应用完成"))
			}
			body.WriteString("\n" + hintStyle("可到「校验」页对比文件确认完整性"))
		case "failed":
			body.WriteString("\n" + errStyle("失败: "+st.installErr.Error()))
		default:
			if st.isFull {
				body.WriteString("\n" + hintStyle("将解压安装到 "+st.installDir))
			} else {
				body.WriteString("\n" + hintStyle("将应用增量补丁到 "+st.installDir))
			}
		}
		body.WriteString("\n\n" + hintStyle("下载文件位于 "+m.cfg.DownloadDir+" · esc 返回"))
	}
	return lipglossBox(m.width, head+"\n\n"+body.String())
}

// overallBar renders the aggregate download progress.
func overallBar(m *model, st *downloadState) string {
	bar := progress.New(progress.WithDefaultGradient())
	bar.Width = m.width - 10
	pct := 0.0
	if st.totBytes > 0 {
		pct = float64(st.dldBytes) / float64(st.totBytes)
		if pct > 1 {
			pct = 1
		}
	}
	var label string
	if st.totBytes > 0 {
		label = fmt.Sprintf("  总体 %s / %s (%.1f%%)",
			humanBytes(st.dldBytes), humanBytes(st.totBytes), pct*100)
	} else {
		label = "  总体进度..."
	}
	return bar.ViewAs(pct) + "\n" + infoStyle(label)
}

// compLine renders one sophon component status line.
func compLine(st *downloadState, i int, name string) string {
	mark := " "
	if i < st.dldItems {
		mark = accentStyle("✓")
	} else if i == st.dldItems {
		mark = "▶"
	}
	return fmt.Sprintf("%s %s", mark, name)
}

func statusLine(it *dlItem, m *model) string {
	var mark string
	switch it.status {
	case "done":
		mark = accentStyle("✓")
	case "failed":
		mark = errStyle("✗")
	case "downloading":
		mark = m.spinner.View()
	default:
		mark = " "
	}
	name := it.name
	if it.status == "downloading" && it.progress > 0 {
		name = fmt.Sprintf("%s %.1f%%", name, it.progress)
	}
	extra := ""
	if it.err != nil {
		extra = errStyle("  " + it.err.Error())
	}
	return fmt.Sprintf("%s %s %s%s", mark, name, infoStyle(humanBytes(it.size)), extra)
}
