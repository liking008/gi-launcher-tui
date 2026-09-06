package config

import (
	"testing"

	"github.com/liking008/gi-launcher-tui/internal/edition"
)

func TestInstallDirPerEdition(t *testing.T) {
	c := Default()
	c.GameDir = "/games/Genshin Impact Game"

	// All editions default to the SAME game folder (Snap.Hutao-style switching).
	for _, e := range edition.All() {
		if got := c.InstallDir(e); got != c.GameDir {
			t.Errorf("[%s] expected %s, got %s", e.Key, c.GameDir, got)
		}
	}

	// Custom mapping overrides the default.
	c.Installs = map[string]string{"oversea": "/games/gi-os"}
	if got := c.InstallDir(edition.Oversea); got != "/games/gi-os" {
		t.Errorf("custom oversea expected /games/gi-os, got %s", got)
	}
}
