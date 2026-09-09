package tui

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/liking008/gi-launcher-tui/internal/archive"
	"github.com/liking008/gi-launcher-tui/internal/download"
	"github.com/liking008/gi-launcher-tui/internal/edition"
	"github.com/liking008/gi-launcher-tui/internal/game"
	"github.com/liking008/gi-launcher-tui/internal/patch"
	"github.com/liking008/gi-launcher-tui/internal/resource"
	"github.com/liking008/gi-launcher-tui/internal/sophon"
)

type dlItem struct {
	name     string
	size     int64
	status   string // "pending" | "downloading" | "done" | "failed"
	progress float64
	dest     string
	task     download.Task
	kind     string // "game" | "audio"
	err      error
}

type downloadState struct {
	step        string // "fetching" | "ready" | "running" | "done"
	pkg         *resource.GamePackage
	branch      *resource.Branch
	build       *sophon.Build
	items       []*dlItem
	sophonComps []*sophon.Component
	plan        string
	installDir  string
	isFull      bool
	sophonPlan  bool
	cur         int

	curName string
	curDone int64
	curSize int64
	curPct  float64
	done    int
	total   int

	// overall progress
	totBytes  int64
	dldBytes  int64
	totItems  int
	dldItems  int
	speed     int64
	compDone  map[string]int64
	lastTime  time.Time
	lastBytes int64

	// content-index (reuse) phase
	indexing bool
	idxFiles int64

	// restore / verification state
	restored      bool
	verifiedOK    int
	verifyTotal   int
	verifyMissing int64
	finalized     bool

	// install / compare state
	installStep string // "" | "installing" | "done" | "failed"
	installErr  error
}

type (
	fetchMsg struct {
		pkg    *resource.GamePackage
		branch *resource.Branch
		build  *sophon.Build
	}
	fetchErr   struct{ err error }
	installMsg struct{ err error }
	tidyMsg    struct {
		text string
		err  error
	}
	idxMsg          struct{ files int64 }
	restoreMsg      struct{}
	verifyResultMsg struct {
		ok, total int
		missing   int64
	}
	dlProgress struct {
		name       string
		done, size int64
		pct        float64
	}
	dlFinished struct {
		name  string
		err   error
		total int64
	}
	dlClosed struct{}
)

func (m *model) fetchRemote() tea.Cmd {
	m.dl = &downloadState{step: "fetching"}
	ed := m.currentEdition()
	return func() tea.Msg {
		pkg, err := m.res.Fetch(context.Background(), ed)
		if err != nil {
			return fetchErr{err: err}
		}
		branch, err := m.res.FetchBranch(context.Background(), ed)
		if err != nil {
			return fetchErr{err: err}
		}
		// The current mechanism is the sophon chunk download; fetch its build.
		var build *sophon.Build
		if branch.Main.PackageID != "" {
			build, _ = sophon.GetBuild(context.Background(), m.httpClient(), ed,
				branch.Main.PackageID, branch.Main.Password, branch.Main.Tag)
		}
		return fetchMsg{pkg: pkg, branch: branch, build: build}
	}
}

// currentEdition resolves the active edition from config, falling back to the
// edition detected from the active install's config.ini.
func (m *model) currentEdition() edition.Edition {
	if e, err := edition.Get(m.cfg.Edition); err == nil {
		return e
	}
	return m.game.Edition()
}

// activeGame returns the Game handle for the active edition's install dir.
func (m *model) activeGame() *game.Game {
	return m.cfg.Game(m.currentEdition())
}

func (m *model) updateDownload(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case fetchMsg:
		m.dl.pkg = msg.pkg
		m.dl.branch = msg.branch
		m.dl.build = msg.build
		m.buildDownloadPlan()
		m.dl.step = "ready"
		m.dl.total = len(m.dl.items)
		return nil
	case idxMsg:
		if m.dl != nil {
			m.dl.idxFiles = msg.files
		}
		return m.subscribe()
	case restoreMsg:
		if m.dl != nil {
			m.dl.restored = true
		}
		return m.subscribe()
	case verifyResultMsg:
		if m.dl != nil {
			m.dl.verifiedOK = msg.ok
			m.dl.verifyTotal = msg.total
			m.dl.verifyMissing = msg.missing
			m.dl.installStep = "verified"
		}
		return m.subscribe()
	case dlProgress:
		if m.dl == nil {
			return m.subscribe()
		}
		m.dl.curName, m.dl.curDone, m.dl.curSize, m.dl.curPct = msg.name, msg.done, msg.size, msg.pct
		if m.dl.sophonPlan && msg.size > 0 {
			if m.dl.compDone == nil {
				m.dl.compDone = map[string]int64{}
			}
			prev, seen := m.dl.compDone[msg.name]
			if !seen {
				m.dl.totBytes += msg.size
			}
			m.dl.dldBytes += msg.done - prev
			m.dl.compDone[msg.name] = msg.done
			// speed from overall bytes
			now := time.Now()
			if !m.dl.lastTime.IsZero() {
				el := now.Sub(m.dl.lastTime).Seconds()
				if el > 0 {
					m.dl.speed = int64(float64(m.dl.dldBytes-m.dl.lastBytes) / el)
				}
			}
			m.dl.lastTime = now
			m.dl.lastBytes = m.dl.dldBytes
		}
		for _, it := range m.dl.items {
			if it.name == msg.name {
				it.progress = msg.pct
				it.status = "downloading"
			}
		}
		return m.subscribe()
	case dlFinished:
		if m.dl == nil {
			return m.subscribe()
		}
		for _, it := range m.dl.items {
			if it.name == msg.name {
				if msg.err != nil {
					it.status = "failed"
					it.err = msg.err
				} else {
					it.status = "done"
					it.progress = 100
					m.dl.done++
					m.dl.dldItems++
					if msg.total > 0 {
						m.dl.dldBytes += msg.total
					}
				}
			}
		}
		if m.dl.sophonPlan {
			m.dl.dldItems++
			if m.dl.dldItems >= m.dl.totItems {
				m.dl.step = "done"
			}
		} else if m.dl.done+countFailed(m.dl.items) >= m.dl.total {
			m.dl.step = "done"
		}
		return m.subscribe()
	case dlClosed:
		if m.dl == nil {
			return nil
		}
		m.dl.step = "done"
		// Sophon chunk mode writes files directly into the install dir; then
		// move the other edition's redundant files to a backup path.
		if m.dl.sophonPlan && !m.dl.finalized {
			return m.finalizeSwitch()
		}
		return nil
	case installMsg:
		if m.dl != nil {
			m.dl.installStep = "done"
			if msg.err != nil {
				m.dl.installStep = "failed"
				m.dl.installErr = msg.err
			}
		}
		return nil
	case tea.KeyMsg:
		switch msg.String() {
		case "d":
			return m.startDownload()
		case "r":
			return m.fetchRemote()
		case "t":
			return m.startTidy()
		}
	}
	return nil
}

// startTidy shows the "正在整理" status and runs the reconcile.
func (m *model) startTidy() tea.Cmd {
	m.tidyRunning = true
	m.tidyResult = ""
	m.tidyErr = nil
	return m.tidyNow()
}

// tidyNow reconciles the install folder for the active server (Snap.Hutao
// style, minimal-copy):
//  1. restores the target's files from backup if present,
//  2. converts the present other-family data folder to the target's name by
//     renaming (the servers share most resources), and moves the other exe to
//     backup,
//  3. compares against the target manifest: moves files that do NOT belong to
//     the target into backup, copies whatever is available from backup for
//     missing files, and reports what still needs downloading.
func (m *model) tidyNow() tea.Cmd {
	installDir := m.activeGame().Dir
	target := m.currentEdition()
	backupRoot := filepath.Join(m.cfg.DownloadDir, "backup")
	backup := filepath.Join(backupRoot, target.DataFolder)
	hc := m.httpClient()
	ctx := context.Background()
	return func() tea.Msg {
		// 1) Restore the target's own files from backup.
		game.RestoreFromBackup(installDir, target, backupRoot)

		// 2) Rename the shared data folder to the target's name + move other exe.
		if err := game.ConvertInstall(installDir, target, backupRoot); err != nil {
			return tidyMsg{err: fmt.Errorf("转换失败: %w", err)}
		}
		// Rewrite pkg_version manifests to reference the active data folder.
		game.FixPkgVersion(installDir, target)

		// Keep the Bilibili-only PCGameSDK.dll in sync (restore or download).
		if err := m.ensurePCSDK(installDir, target, backupRoot); err != nil {
			return tidyMsg{err: fmt.Errorf("PCGameSDK.dll: %w", err)}
		}

		// 3) Compare against the target manifest and reconcile.
		restored := 0
		var remainingBytes int64
		if branch, err := m.res.FetchBranch(ctx, target); err == nil && branch.Main.PackageID != "" {
			if build, berr := sophon.GetBuild(ctx, hc, target, branch.Main.PackageID, branch.Main.Password, branch.Main.Tag); berr == nil {
				// Collect per-component assets once.
				var allAssets []struct {
					path string
					size int64
				}
				for i := range build.Data.Manifests {
					c := &build.Data.Manifests[i]
					if c.MatchingField != "game" && !wantsVoice(c.MatchingField, m.cfg.VoicePacks) {
						continue
					}
					assets, aerr := sophon.FetchManifest(ctx, hc, c)
					if aerr != nil {
						continue
					}
					for _, a := range assets {
						if a.Type == 64 {
							continue
						}
						allAssets = append(allAssets, struct {
							path string
							size int64
						}{filepath.ToSlash(a.Path), a.Size})
					}
				}
				// Note: files in the data folder that are NOT in the manifest
				// (e.g. added by in-game hot updates) are intentionally left in
				// place, mirroring Snap.Hutao / aagl — moving them would cause
				// re-downloads after every hot update.
				// Restore/account for missing manifest files.
				for _, a := range allAssets {
					dest := filepath.Join(installDir, filepath.FromSlash(a.path))
					if _, err := os.Stat(dest); err == nil {
						continue // present
					}
					src := filepath.Join(backup, filepath.FromSlash(a.path))
					if _, err := os.Stat(src); err == nil {
						if copyInto(src, dest) == nil {
							restored++
						} else {
							remainingBytes += a.size
						}
					} else {
						remainingBytes += a.size
					}
				}
			}
		}

		msg := fmt.Sprintf("整理完成：已重命名/保留共享资源，从备份恢复 %d 个文件，另一版本独有文件已整理到备份", restored)
		if remainingBytes > 0 {
			msg += fmt.Sprintf("\n当前服务器仍缺 %s，请按 d 下载补齐（将复用已有资源）。", humanBytes(remainingBytes))
		}
		return tidyMsg{text: msg}
	}
}

// copyInto copies src to dst, preferring a hard link so shared files are
// stored only once (deduplicated) on the same filesystem.
func copyInto(src, dst string) error {
	if err := os.Link(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, cerr := io.Copy(out, in)
	out.Close()
	return cerr
}

// wantsVoice reports whether the manifest matching field is a selected voice.
func wantsVoice(lang string, voicePacks []string) bool {
	for _, v := range voicePacks {
		if v == lang {
			return true
		}
	}
	return false
}

func countFailed(items []*dlItem) int {
	n := 0
	for _, it := range items {
		if it.status == "failed" {
			n++
		}
	}
	return n
}

func (m *model) buildDownloadPlan() {
	if m.dl == nil {
		return
	}
	cur := m.activeGame().Version()
	m.dl.items = nil
	m.dl.installDir = m.activeGame().Dir

	latest := ""
	if m.dl.branch != nil {
		latest = m.dl.branch.Main.Tag
	}
	// If we have a sophon build (real current version), prefer the chunk mode.
	if m.dl.build != nil && len(m.dl.build.Data.Manifests) > 0 {
		m.dl.sophonPlan = true
		m.dl.sophonComps = nil
		// Add the game resources plus the selected voice packs.
		for i := range m.dl.build.Data.Manifests {
			c := &m.dl.build.Data.Manifests[i]
			if c.MatchingField == "game" || m.wantsVoice(c.MatchingField) {
				m.dl.sophonComps = append(m.dl.sophonComps, c)
			}
		}
		m.dl.plan = fmt.Sprintf("[%s] 最新版本 %s (sophon 分块下载) · 当前版本 %s",
			m.currentEdition().Name, latest, cur)
		return
	}

	if m.dl.pkg == nil {
		return
	}
	major := m.dl.pkg.Main.Major
	m.dl.sophonPlan = false
	if latest == "" {
		latest = major.Version
	}
	m.dl.plan = fmt.Sprintf("[%s] 最新版本 %s · 当前版本 %s",
		m.currentEdition().Name, latest, cur)

	if patch := m.dl.pkg.Main.PatchTo(cur); patch != nil {
		m.dl.plan = fmt.Sprintf("[%s] 更新路径: %s -> %s (增量)",
			m.currentEdition().Name, cur, major.Version)
		for _, seg := range patch.GamePkgs {
			m.addSegment(seg)
		}
		for _, ap := range patch.AudioPkgs {
			m.addAudio(ap)
		}
		return
	}

	m.dl.isFull = true
	m.dl.plan = fmt.Sprintf("[%s] 全新安装: %s", m.currentEdition().Name, major.Version)
	for _, seg := range major.GamePkgs {
		m.addSegment(seg)
	}
	for _, ap := range major.AudioPkgs {
		m.addAudio(ap)
	}
}

func (m *model) addSegment(seg resource.Segment) {
	u, _ := url.Parse(seg.URL)
	name := filepath.Base(u.Path)
	m.addTask(name, resource.ParseInt(seg.Size), seg.URL, seg.MD5, "game")
}

func (m *model) addAudio(ap resource.AudioPackage) {
	if !m.wantsVoice(ap.Language) {
		return
	}
	u, _ := url.Parse(ap.URL)
	name := filepath.Base(u.Path)
	m.addTask(name, resource.ParseInt(ap.Size), ap.URL, ap.MD5, "audio")
}

func (m *model) wantsVoice(lang string) bool {
	for _, v := range m.cfg.VoicePacks {
		if v == lang {
			return true
		}
	}
	return false
}

func (m *model) addTask(name string, size int64, url, md5, kind string) {
	dest := filepath.Join(m.cfg.DownloadDir, name)
	m.dl.items = append(m.dl.items, &dlItem{
		name:   name,
		size:   size,
		status: "pending",
		dest:   dest,
		task:   download.Task{URL: url, Dest: dest, ExpectedSize: size, ExpectedMD5: strings.ToLower(md5)},
		kind:   kind,
	})
}

func (m *model) startDownload() tea.Cmd {
	if m.dl == nil || m.dl.step != "ready" {
		return nil
	}
	m.dl.step = "running"
	if m.dl.sophonPlan {
		return m.startSophonDownload()
	}
	m.dlCh = make(chan tea.Msg, 256)
	mgr := download.NewManager()
	items := append([]*dlItem(nil), m.dl.items...)
	ch := m.dlCh
	go func() {
		defer close(ch)
		for _, it := range items {
			res := mgr.Download(context.Background(), it.task, func(p download.Progress) {
				if p.Percent < 0 {
					p.Percent = 0
				}
				ch <- dlProgress{name: it.name, done: p.Downloaded, size: p.Total, pct: p.Percent}
			})
			ch <- dlFinished{name: it.name, err: res.Err, total: res.Size}
		}
	}()
	return m.subscribe()
}

// startSophonDownload downloads the sophon chunk build into the install dir.
func (m *model) startSophonDownload() tea.Cmd {
	m.dlCh = make(chan tea.Msg, 256)
	m.dl.totItems = len(m.dl.sophonComps)
	m.dl.dldItems = 0
	m.dl.compDone = map[string]int64{}
	ch := m.dlCh
	comps := append([]*sophon.Component(nil), m.dl.sophonComps...)
	installDir := m.dl.installDir
	target := m.currentEdition()
	backupRoot := filepath.Join(m.cfg.DownloadDir, "backup")
	hc := m.httpClient()
	ctx := context.Background()
	go func() {
		defer close(ch)
		// Restore the target's files from backup if they were moved away during
		// a previous switch (so switching back doesn't re-download everything).
		if restored, _ := game.RestoreFromBackup(installDir, target, backupRoot); restored {
			ch <- restoreMsg{}
		}
		// Content-reuse index from pkg_version (instant), shared across comps.
		idx := sophon.BuildContentIndexFromManifests(installDir)
		// Also reuse shared resources from the other server family's data
		// folder if it is present (avoids re-downloading shared content).
		altRoot := altDataFolderPath(installDir, target)

		for _, c := range comps {
			ch <- dlProgress{name: c.CategoryName, done: 0, size: 0, pct: 0}
			n, err := sophon.DownloadAssets(ctx, hc, c, installDir,
				sophon.Options{
					SkipExisting: true,
					ReuseContent: true,
					ReuseIndex:   idx,
					ReuseAltRoot: altRoot,
					Progress: func(done, total int64) {
						pct := 0.0
						if total > 0 {
							pct = float64(done) / float64(total) * 100
						}
						ch <- dlProgress{name: c.CategoryName, done: done, size: total, pct: pct}
					},
				})
			ch <- dlFinished{name: c.CategoryName, err: err, total: n}
		}

		// Verify completeness against the manifests after download.
		m.dl.installStep = "verifying"
		ok, total, missing := 0, 0, int64(0)
		for _, c := range comps {
			o, t, mb, err := sophon.VerifyAssets(ctx, hc, c, installDir)
			if err == nil {
				ok += o
				total += t
				missing += mb
			}
		}
		ch <- verifyResultMsg{ok: ok, total: total, missing: missing}
	}()
	return m.subscribe()
}

// applyDownloadedPatch applies the downloaded incremental (hdiff) patch into
// the active edition's install directory.
func (m *model) applyDownloadedPatch() tea.Cmd {
	m.dl.installStep = "installing"
	installDir := m.dl.installDir
	toolsDir := m.cfg.ToolsDir
	// The first game-kind download is the hdiff patch zip.
	var patchZip string
	for _, it := range m.dl.items {
		if it.kind == "game" {
			patchZip = it.dest
			break
		}
	}
	return func() tea.Msg {
		if patchZip == "" {
			return installMsg{err: fmt.Errorf("没有找到补丁包")}
		}
		hp, err := patch.HpatchzPath(toolsDir)
		if err != nil {
			return installMsg{err: err}
		}
		if _, err := patch.ApplyPatchZip(installDir, patchZip, hp, m.cfg.DownloadDir); err != nil {
			return installMsg{err: fmt.Errorf("应用补丁失败: %w", err)}
		}
		return installMsg{}
	}
}

// installFullGame joins the downloaded multi-part game zip and extracts it into
// the active edition's install directory, then verifies the result.
func (m *model) installFullGame() tea.Cmd {
	m.dl.installStep = "installing"
	installDir := m.dl.installDir
	downloadDir := m.cfg.DownloadDir
	gameParts := make([]string, 0, 8)
	for _, it := range m.dl.items {
		if it.kind == "game" {
			gameParts = append(gameParts, it.dest)
		}
	}
	return func() tea.Msg {
		zipped, err := joinZipParts(gameParts, downloadDir)
		if err != nil {
			return installMsg{err: err}
		}
		if err := archive.ExtractZip(zipped, installDir, nil); err != nil {
			return installMsg{err: fmt.Errorf("解压失败: %w", err)}
		}
		return installMsg{}
	}
}

// joinZipParts concatenates segmented zip parts (.001, .002, ...) into one .zip.
func joinZipParts(parts []string, downloadDir string) (string, error) {
	if len(parts) == 0 {
		return "", fmt.Errorf("没有游戏包分卷")
	}
	// The base name is like "<game>_<ver>.zip.001"; strip the ".NNN" suffix.
	base := parts[0]
	if i := strings.LastIndex(base, ".zip."); i > 0 {
		base = base[:i+4]
	} else {
		return parts[0], nil // single-file zip
	}
	out, err := os.Create(base)
	if err != nil {
		return "", err
	}
	defer out.Close()
	for _, p := range parts {
		f, err := os.Open(p)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(out, f); err != nil {
			f.Close()
			return "", err
		}
		f.Close()
	}
	return base, nil
}

// finalizeSwitch moves the previous edition's redundant files (exe + data
// folder) to a backup directory after the target edition has been downloaded,
// mirroring Snap.Hutao's server-switch-in-place approach.
func (m *model) finalizeSwitch() tea.Cmd {
	m.dl.finalized = true
	m.dl.installStep = "installing"
	installDir := m.dl.installDir
	target := m.currentEdition()
	backupRoot := filepath.Join(m.cfg.DownloadDir, "backup")
	return func() tea.Msg {
		// Keep the Bilibili-only PCGameSDK.dll in sync (restore or download).
		if err := m.ensurePCSDK(installDir, target, backupRoot); err != nil {
			return installMsg{err: fmt.Errorf("PCGameSDK.dll: %w", err)}
		}
		if err := game.ConvertInstall(installDir, target, backupRoot); err != nil {
			return installMsg{err: fmt.Errorf("整理旧版本文件失败: %w", err)}
		}
		return installMsg{}
	}
}

// altDataFolderPath returns the path to the other server family's data folder
// if it exists in the install dir (e.g. GenshinImpact_Data when targeting CN).
func altDataFolderPath(installDir string, target edition.Edition) string {
	for _, e := range edition.All() {
		if e.Key == target.Key || e.DataFolder == target.DataFolder {
			continue
		}
		p := filepath.Join(installDir, e.DataFolder)
		if fi, err := os.Stat(p); err == nil && fi.IsDir() {
			return p
		}
	}
	return ""
}

// ensurePCSDK keeps the Bilibili-only PCGameSDK.dll in sync:
//   - B服: restore from backup first (fast, no download); only download the
//     SDK package when there is no backup.
//   - other servers: move the dll into backup so it can be restored later.
func (m *model) ensurePCSDK(installDir string, target edition.Edition, backupRoot string) error {
	sdk := filepath.Join(installDir, filepath.FromSlash(game.PCSDK))
	backupDLL := filepath.Join(backupRoot, filepath.Dir(filepath.FromSlash(game.PCSDK)), "PCGameSDK.dll")
	// The full Bilibili SDK also ships BLPlatform64 (browser platform).
	fullSDK := filepath.Join(installDir, "YuanShen_Data", "Plugins", "BLPlatform64")
	fullOK := func() bool {
		_, e1 := os.Stat(sdk)
		_, e2 := os.Stat(fullSDK)
		return e1 == nil && e2 == nil
	}

	if target.NeedsSDK {
		if fullOK() {
			return nil // complete SDK already extracted
		}
		// Restore the dll from backup first (fast).
		if _, err := os.Stat(backupDLL); err == nil {
			if err := os.MkdirAll(filepath.Dir(sdk), 0o755); err != nil {
				return err
			}
			if err := os.Rename(backupDLL, sdk); err != nil {
				return err
			}
			if fullOK() {
				return nil
			}
		}
		// Ensure the FULL SDK package is extracted (reuses the cached zip,
		// only downloads when there is no valid cache).
		return m.downloadPCSDK(installDir)
	}

	// Other servers don't need the Bilibili SDK -> keep it in backup.
	if _, err := os.Stat(sdk); err == nil {
		if err := os.MkdirAll(filepath.Dir(backupDLL), 0o755); err != nil {
			return err
		}
		_ = os.Remove(backupDLL)
		return os.Rename(sdk, backupDLL)
	}
	return nil
}

// downloadPCSDK fetches the current Bilibili channel SDK (HoyoPlay
// getGameChannelSDKs) and extracts it into the game directory.
func (m *model) downloadPCSDK(installDir string) error {
	// Bilibili Genshin channel SDK endpoint (launcher umfgRO5gh5, game T2S0Gz4Dr2).
	const sdkEndpoint = "https://hyp-api.mihoyo.com/hyp/hyp-connect/api/getGameChannelSDKs?channel=14&game_ids[]=T2S0Gz4Dr2&launcher_id=umfgRO5gh5&sub_channel=0"
	body, err := httpGet(context.Background(), m.httpClient(), sdkEndpoint)
	if err != nil {
		return fmt.Errorf("获取 B服 SDK 信息失败: %w", err)
	}
	var res struct {
		Data struct {
			GameChannelSDKs []struct {
				Version       string `json:"version"`
				ChannelSdkPkg struct {
					URL  string `json:"url"`
					MD5  string `json:"md5"`
					Size string `json:"size"`
				} `json:"channel_sdk_pkg"`
				PkgVersionFileName string `json:"pkg_version_file_name"`
			} `json:"game_channel_sdks"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return fmt.Errorf("解析 B服 SDK 信息失败: %w", err)
	}
	if len(res.Data.GameChannelSDKs) == 0 || res.Data.GameChannelSDKs[0].ChannelSdkPkg.URL == "" {
		return fmt.Errorf("B服通道 SDK 接口未返回下载地址")
	}
	info := res.Data.GameChannelSDKs[0]
	wantMD5 := strings.ToLower(info.ChannelSdkPkg.MD5)

	zipPath := filepath.Join(m.cfg.DownloadDir, "bilibili_sdk.zip")
	if err := os.MkdirAll(m.cfg.DownloadDir, 0o755); err != nil {
		return err
	}
	// Reuse an existing, valid cached copy instead of re-downloading.
	if wantMD5 != "" && md5HexFile(zipPath) == wantMD5 {
		// already cached and valid
	} else {
		var downloaded bool
		for attempt := 0; attempt < 3; attempt++ {
			if err := httpDownload(context.Background(), m.httpClient(), info.ChannelSdkPkg.URL, zipPath); err != nil {
				os.Remove(zipPath)
				if attempt == 2 {
					return fmt.Errorf("下载 B服 SDK (%s) 失败: %w", info.Version, err)
				}
				continue
			}
			if wantMD5 == "" || md5HexFile(zipPath) == wantMD5 {
				downloaded = true
				break
			}
			os.Remove(zipPath) // corrupt/incomplete -> retry
		}
		if !downloaded {
			return fmt.Errorf("下载 B服 SDK 多次校验失败（MD5 不符），请重试")
		}
	}

	// Extract the whole SDK package, preserving the YuanShen_Data/Plugins paths.
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("SDK 压缩包无法打开: %w", err)
	}
	defer zr.Close()
	for _, zf := range zr.File {
		rel := strings.TrimPrefix(filepath.ToSlash(zf.Name), "/")
		if rel == "" || strings.HasSuffix(rel, "/") {
			continue
		}
		if err := extractZipEntry(zf, filepath.Join(installDir, filepath.FromSlash(rel))); err != nil {
			return fmt.Errorf("解压 %s: %w", rel, err)
		}
	}
	return nil
}

func md5HexFile(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := md5.New()
	io.Copy(h, f)
	return hex.EncodeToString(h.Sum(nil))
}

func extractZipEntry(zf *zip.File, dest string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, cerr := io.Copy(out, rc)
	out.Close()
	return cerr
}

// httpGet performs a GET request and returns the response body.
func httpGet(ctx context.Context, hc *http.Client, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120 Safari/537.36")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %s", resp.Status)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 16<<20))
}

// httpDownload streams a URL into a local file.
func httpDownload(ctx context.Context, hc *http.Client, url, dest string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/120 Safari/537.36")
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("http %s", resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	_, cerr := io.Copy(out, resp.Body)
	out.Close()
	return cerr
}

// subscribe blocks until the download channel emits a message.
func (m *model) subscribe() tea.Cmd {
	return func() tea.Msg {
		if m.dlCh == nil {
			return dlClosed{}
		}
		v, ok := <-m.dlCh
		if !ok {
			return dlClosed{}
		}
		return v
	}
}
