package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/liking008/gi-launcher-tui/internal/edition"
	"github.com/liking008/gi-launcher-tui/internal/game"
)

const defaultGameDir = "/home/liking008/Games/Genshin Impact Game"

// Config holds all persistent launcher settings. It is persisted as JSON.
type Config struct {
	// GameDir is the folder containing the game executable and pkg_version files.
	GameDir string `json:"game_dir"`
	// DownloadDir is where downloaded packages/patches are cached.
	DownloadDir string `json:"download_dir"`
	// VersionDir is the root that holds multiple version snapshots for switching.
	VersionDir string `json:"version_dir"`
	// VoicePacks lists which audio languages to keep (zh-cn, en-us, ja-jp, ko-kr).
	VoicePacks []string `json:"voice_packs"`
	// Concurrency is the number of parallel download streams.
	Concurrency int `json:"concurrency"`
	// ActiveAccount is the currently applied account profile name.
	ActiveAccount string `json:"active_account"`
	// Edition is the current server key (official_cn / bilibili / oversea).
	Edition string `json:"edition"`
	// Installs maps an edition key to its own install directory. Each server
	// uses a separate directory because the files differ (CDN, exe, data
	// folder), mirroring Snap.Hutao's per-server game file system.
	Installs map[string]string `json:"installs"`
	// ToolsDir caches helper binaries (e.g. hpatchz).
	ToolsDir string `json:"tools_dir"`
	// WinePrefix is the wine/proton prefix used to launch the game.
	WinePrefix string `json:"wine_prefix"`
}

func Default() *Config {
	return &Config{
		GameDir:     defaultGameDir,
		DownloadDir: filepath.Join(os.Getenv("HOME"), ".cache", "gi-launcher", "downloads"),
		VersionDir:  filepath.Join(os.Getenv("HOME"), ".local", "share", "gi-launcher", "versions"),
		VoicePacks:  []string{"zh-cn"},
		Concurrency: 4,
		Edition:     "official_cn",
		ToolsDir:    filepath.Join(os.Getenv("HOME"), ".cache", "gi-launcher", "tools"),
		WinePrefix:  filepath.Join(os.Getenv("HOME"), ".local", "share", "gi-launcher", "wine"),
	}
}

// InstallDir returns the install directory for a given edition. Snap.Hutao
// switches servers in the SAME game folder by rewriting config.ini and managing
// the executables/data folders that coexist there, so all editions share
// GameDir by default. An explicit Installs mapping overrides this.
func (c *Config) InstallDir(ed edition.Edition) string {
	if c.Installs != nil {
		if d, ok := c.Installs[ed.Key]; ok && d != "" {
			return d
		}
	}
	return c.GameDir
}

// Game returns a Game handle for the given edition's install directory.
func (c *Config) Game(ed edition.Edition) *game.Game {
	return game.New(c.InstallDir(ed))
}

func configPath() (string, error) {
	if p := os.Getenv("GIH_LANCHER_CONFIG"); p != "" {
		return p, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gi-launcher", "config.json"), nil
}

func Load() (*Config, error) {
	cfg := Default()
	p, err := configPath()
	if err != nil {
		return cfg, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func (c *Config) Save() error {
	p, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(p, data, 0o644)
}
