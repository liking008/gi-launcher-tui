package resource

import (
	"context"
	"testing"

	"github.com/liking008/gi-launcher-tui/internal/edition"
)

// TestBranchLive verifies getGameBranches returns the real current version.
func TestBranchLive(t *testing.T) {
	c := New()
	for _, ed := range edition.All() {
		b, err := c.FetchBranch(context.Background(), ed)
		if err != nil {
			t.Fatalf("[%s] branch failed: %v", ed.Name, err)
		}
		if b.Main.Tag == "" {
			t.Errorf("[%s] empty branch tag", ed.Name)
		}
		t.Logf("[%s] latest=%s diff_tags=%v pre_download=%v",
			ed.Name, b.Main.Tag, b.Main.DiffTags, preDownloadTag(b))
	}
}

func preDownloadTag(b *Branch) string {
	if b.PreDownload == nil {
		return "<无>"
	}
	return b.PreDownload.Tag
}
func TestProtocolLive(t *testing.T) {
	c := New()
	for _, ed := range []edition.Edition{edition.OfficialCN, edition.Oversea} {
		proto, err := c.Protocol(context.Background(), ed)
		if err != nil {
			t.Fatalf("[%s] protocol failed: %v", ed.Name, err)
		}
		if proto.Title == "" {
			t.Errorf("[%s] empty protocol title", ed.Name)
		}
		if proto.AgreementVersion == "" {
			t.Errorf("[%s] empty agreement version", ed.Name)
		}
		t.Logf("[%s] title=%q version=%s len=%d", ed.Name, proto.Title, proto.AgreementVersion, len(proto.Content))
	}
}
func TestFetchLive(t *testing.T) {
	c := New()
	for _, ed := range edition.All() {
		pkg, err := c.Fetch(context.Background(), ed)
		if err != nil {
			t.Fatalf("[%s] fetch failed: %v", ed.Name, err)
		}
		if pkg.Game.Biz != ed.GameBiz {
			t.Errorf("[%s] expected biz %s, got %s", ed.Name, ed.GameBiz, pkg.Game.Biz)
		}
		if pkg.Main.Major.Version == "" {
			t.Errorf("[%s] empty major version", ed.Name)
		}
		t.Logf("[%s] version=%s segments=%d patches=%d",
			ed.Name, pkg.Main.Major.Version, len(pkg.Main.Major.GamePkgs), len(pkg.Main.Patches))
	}
}
