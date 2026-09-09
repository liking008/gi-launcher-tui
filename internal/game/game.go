package game

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/liking008/gi-launcher-tui/internal/edition"
)

const (
	ConfigIni   = "config.ini"
	PkgVersion  = "pkg_version"
	AudioPrefix = "Audio_"
	// PCSDK is the account plugin file only present on the Bilibili client.
	PCSDK = "YuanShen_Data/Plugins/PCGameSDK.dll"
)

// Game inspects and manages a single Genshin Impact installation.
type Game struct {
	Dir string
}

func New(dir string) *Game {
	return &Game{Dir: dir}
}

// Installed reports whether the current edition's executable exists.
func (g *Game) Installed() bool {
	_, err := os.Stat(filepath.Join(g.Dir, g.Edition().ExeName))
	return err == nil
}

// ExePath returns the path to the current edition's executable.
func (g *Game) ExePath() string {
	return filepath.Join(g.Dir, g.Edition().ExeName)
}

// Config returns the parsed key=value pairs from config.ini.
func (g *Game) Config() (map[string]string, error) {
	f, err := os.Open(filepath.Join(g.Dir, ConfigIni))
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := map[string]string{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out, sc.Err()
}

// Version returns the installed game version (config.ini -> game_version).
func (g *Game) Version() string {
	cfg, err := g.Config()
	if err != nil {
		return ""
	}
	return cfg["game_version"]
}

// Channel returns the launcher channel (config.ini -> channel).
func (g *Game) Channel() string {
	cfg, err := g.Config()
	if err != nil {
		return ""
	}
	return cfg["channel"]
}

// SetVersion writes game_version back into config.ini, preserving other keys.
func (g *Game) SetVersion(v string) error {
	cfg, err := g.Config()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	cfg["game_version"] = v
	return g.writeConfig(cfg)
}

// Edition detects the current server from config.ini. It prefers an exact
// channel+cps match, then falls back to matching by channel, then to the
// presence of the oversea executable.
func (g *Game) Edition() edition.Edition {
	cfg, err := g.Config()
	if err != nil {
		return edition.OfficialCN
	}
	channel := cfg["channel"]
	subChannel := cfg["sub_channel"]

	for _, e := range edition.All() {
		if e.Channel == channel && e.SubChannel == subChannel && e.CPS() == cfg["cps"] {
			return e
		}
	}
	for _, e := range edition.All() {
		if e.Channel == channel && e.SubChannel == subChannel {
			return e
		}
	}
	// Oversea installs use GenshinImpact.exe.
	if _, err := os.Stat(filepath.Join(g.Dir, edition.Oversea.ExeName)); err == nil {
		return edition.Oversea
	}
	return edition.OfficialCN
}

// SetEdition rewrites config.ini for the target server (Snap.Hutao format) and
// manages the account SDK file that differs between official and Bilibili.
func (g *Game) SetEdition(e edition.Edition) error {
	cfg, err := g.Config()
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if cfg == nil {
		cfg = map[string]string{}
	}
	cfg["channel"] = e.Channel
	cfg["sub_channel"] = e.SubChannel
	cfg["cps"] = e.CPS()
	cfg["uapc"] = e.UAPC()
	// The target install directory may not exist yet (e.g. a fresh 国际服).
	if err := os.MkdirAll(g.Dir, 0o755); err != nil {
		return err
	}
	if err := g.writeConfig(cfg); err != nil {
		return err
	}
	// Manage the Bilibili-only PCGameSDK.dll plugin.
	sdk := filepath.Join(g.Dir, filepath.FromSlash(PCSDK))
	if e.NeedsSDK {
		return os.MkdirAll(filepath.Dir(sdk), 0o755)
	}
	_ = os.Remove(sdk)
	return nil
}

func (g *Game) writeConfig(cfg map[string]string) error {
	var b strings.Builder
	b.WriteString("[general]\n")
	for k, v := range cfg {
		b.WriteString(k)
		b.WriteString("=")
		b.WriteString(v)
		b.WriteString("\n")
	}
	return os.WriteFile(filepath.Join(g.Dir, ConfigIni), []byte(b.String()), 0o644)
}

// FinalizeSwitch moves the other server family's files out of the shared
// install directory into backupRoot/<DataFolder>, so the folder cleanly
// represents only the active server. The whole other data folder and its
// executable are moved to a per-family backup slot (ready to be restored when
// switching back); renames on the same filesystem do not duplicate disk usage.
func FinalizeSwitch(dir string, target edition.Edition, backupRoot string) error {
	for _, e := range edition.All() {
		if e.Key == target.Key {
			continue
		}
		if e.DataFolder == target.DataFolder {
			continue // same server family, shares files
		}
		b := filepath.Join(backupRoot, e.DataFolder)
		if err := os.MkdirAll(b, 0o755); err != nil {
			return err
		}
		if err := moveAway(filepath.Join(dir, e.ExeName), b); err != nil {
			return err
		}
		if err := moveAway(filepath.Join(dir, e.DataFolder), b); err != nil {
			return err
		}
	}
	return nil
}

// ConvertInstall converts the shared install directory to the target server by
// RENAMING the currently-present other-family data folder to the target's name
// (the two servers share most resource content, so this preserves everything
// with a near-zero copy cost even across partitions), and moves the other
// family's executable into backup. If the target's data folder already exists
// while the other family's folder is also present, the other folder is residue
// and is moved wholesale into its backup slot.
func ConvertInstall(dir string, target edition.Edition, backupRoot string) error {
	for _, e := range edition.All() {
		if e.Key == target.Key {
			continue
		}
		if e.DataFolder == target.DataFolder {
			continue // same server family, shares files
		}
		b := filepath.Join(backupRoot, e.DataFolder)
		if err := os.MkdirAll(b, 0o755); err != nil {
			return err
		}
		if err := moveAway(filepath.Join(dir, e.ExeName), b); err != nil {
			return err
		}
		otherDir := filepath.Join(dir, e.DataFolder)
		targetDir := filepath.Join(dir, target.DataFolder)
		if _, err := os.Stat(otherDir); err != nil {
			continue // nothing to convert
		}
		if _, terr := os.Stat(targetDir); os.IsNotExist(terr) {
			// Rename the shared data folder to the target's name.
			if err := os.Rename(otherDir, targetDir); err != nil {
				return err
			}
		} else {
			// Both present: the other folder is the previous version's residue
			// -> move it into its own backup slot.
			if err := moveAway(otherDir, b); err != nil {
				return err
			}
		}
	}
	return nil
}

// FixPkgVersion rewrites the root pkg_version manifest files so their
// remoteName paths point to the active server's data folder. After ConvertInstall
// renames a data folder (e.g. GenshinImpact_Data -> YuanShen_Data) the old
// manifest still references the old folder name, which would make verification
// report every file as missing.
func FixPkgVersion(dir string, target edition.Edition) error {
	files := []string{
		"pkg_version",
		"Audio_Chinese_pkg_version",
		"Audio_English(US)_pkg_version",
		"Audio_Japanese_pkg_version",
		"Audio_Korean_pkg_version",
	}
	for _, f := range files {
		path := filepath.Join(dir, f)
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		changed := false
		for _, e := range edition.All() {
			if e.DataFolder == target.DataFolder {
				continue
			}
			old := []byte(`"` + e.DataFolder + `/`)
			if bytes.Contains(data, old) {
				data = bytes.ReplaceAll(data, old, []byte(`"`+target.DataFolder+`/`))
				changed = true
			}
		}
		if changed {
			if err := os.WriteFile(path, data, 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

// RestoreFromBackup moves the target edition's files back from
// backupRoot/<DataFolder> into dir, so switching to a server whose files were
// previously moved away does not require re-downloading them. Returns whether
// anything was restored.
func RestoreFromBackup(dir string, target edition.Edition, backupRoot string) (bool, error) {
	b := filepath.Join(backupRoot, target.DataFolder)
	restored := false
	if _, err := os.Stat(filepath.Join(b, target.ExeName)); err == nil {
		if err := moveAway(filepath.Join(b, target.ExeName), dir); err != nil {
			return restored, err
		}
		restored = true
	}
	if _, err := os.Stat(filepath.Join(b, target.DataFolder)); err == nil {
		if err := moveAway(filepath.Join(b, target.DataFolder), dir); err != nil {
			return restored, err
		}
		restored = true
	}
	return restored, nil
}

// ManagePCSDK keeps the Bilibili-only PCGameSDK.dll in sync with the active
// server: when the target needs it (B服) it is restored from backup (or the
// plugin dir is created for a later download); otherwise it is moved into the
// backup folder.
func ManagePCSDK(dir string, target edition.Edition, backupRoot string) error {
	sdkRel := filepath.FromSlash(PCSDK) // YuanShen_Data/Plugins/PCGameSDK.dll
	sdk := filepath.Join(dir, sdkRel)
	backupDir := filepath.Join(backupRoot, filepath.Dir(sdkRel))
	backupDLL := filepath.Join(backupDir, "PCGameSDK.dll")

	if target.NeedsSDK {
		if _, err := os.Stat(backupDLL); err == nil {
			if err := os.MkdirAll(filepath.Dir(sdk), 0o755); err != nil {
				return err
			}
			return os.Rename(backupDLL, sdk)
		}
		// Not backed up yet -> create the plugin dir (dll comes via download).
		return os.MkdirAll(filepath.Dir(sdk), 0o755)
	}

	// Other servers don't need the Bilibili SDK -> move it into backup.
	if _, err := os.Stat(sdk); err != nil {
		return nil
	}
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		return err
	}
	_ = os.Remove(backupDLL)
	return os.Rename(sdk, backupDLL)
}

// moveAway relocates src into backupDir, removing any existing entry there.
// Missing sources are a no-op; errors on vanished files are tolerated so a
// partially-moved install never aborts tidy.
func moveAway(src, backupDir string) error {
	st, err := os.Stat(src)
	if err != nil {
		return nil // nothing to move
	}
	dest := filepath.Join(backupDir, filepath.Base(src))
	_ = os.RemoveAll(dest)
	if err := os.Rename(src, dest); err != nil {
		// Cross-device/subvolume fallback: copy then delete. If the file
		// disappeared meanwhile, treat it as already moved.
		if _, serr := os.Stat(src); serr != nil {
			return nil
		}
		if cerr := copyTree(src, dest); cerr != nil {
			return fmt.Errorf("move %s: %w", src, cerr)
		}
		return os.RemoveAll(src)
	}
	_ = st
	return nil
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			// File vanished during walk; skip rather than abort.
			return nil
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		in, err := os.Open(p)
		if err != nil {
			return nil
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
		if err != nil {
			return err
		}
		_, cerr := io.Copy(out, in)
		out.Close()
		return cerr
	})
}
