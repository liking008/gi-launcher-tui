package resource

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/liking008/gi-launcher-tui/internal/edition"
)

type Client struct {
	http *http.Client
}

func New() *Client {
	return &Client{http: &http.Client{Timeout: 30 * time.Second}}
}

// Packages is the top-level response of the hyp-connect getGamePackages API.
type Packages struct {
	Retcode int    `json:"retcode"`
	Message string `json:"message"`
	Data    Data   `json:"data"`
}

type Data struct {
	GamePackages []GamePackage `json:"game_packages"`
}

type GamePackage struct {
	Game        GameId               `json:"game"`
	Main        GameInfo             `json:"main"`
	PreDownload *GamePreDownloadInfo `json:"pre_download"`
}

type GameId struct {
	ID  string `json:"id"`
	Biz string `json:"biz"`
}

type GameInfo struct {
	Major   GameLatest `json:"major"`
	Patches []Patch    `json:"patches"`
}

type GameLatest struct {
	Version    string         `json:"version"`
	GamePkgs   []Segment      `json:"game_pkgs"`
	AudioPkgs  []AudioPackage `json:"audio_pkgs"`
	ResListURL string         `json:"res_list_url"`
}

type Segment struct {
	URL              string `json:"url"`
	MD5              string `json:"md5"`
	Size             string `json:"size"`
	DecompressedSize string `json:"decompressed_size"`
}

type AudioPackage struct {
	Language         string `json:"language"`
	URL              string `json:"url"`
	MD5              string `json:"md5"`
	Size             string `json:"size"`
	DecompressedSize string `json:"decompressed_size"`
}

type Patch struct {
	Version   string         `json:"version"`
	GamePkgs  []Segment      `json:"game_pkgs"`
	AudioPkgs []AudioPackage `json:"audio_pkgs"`
}

type GamePreDownloadInfo struct {
	Major   *GameLatest `json:"major"`
	Patches []Patch     `json:"patches"`
}

// Fetch queries getGamePackages for the given edition and returns the Genshin
// package (biz starts with "hk4e_").
func (c *Client) Fetch(ctx context.Context, ed edition.Edition) (*GamePackage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, ed.PackagesURL(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getGamePackages status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	var p Packages
	if err := json.Unmarshal(body, &p); err != nil {
		return nil, err
	}
	if p.Retcode != 0 {
		return nil, fmt.Errorf("getGamePackages retcode %d: %s", p.Retcode, p.Message)
	}
	for i := range p.Data.GamePackages {
		gp := p.Data.GamePackages[i]
		if len(gp.Game.Biz) >= 5 && gp.Game.Biz[:5] == "hk4e_" {
			return &gp, nil
		}
	}
	return nil, fmt.Errorf("未在 API 中找到原神资源 (edition=%s)", ed.Name)
}

// Branch mirrors the per-game object of the getGameBranches API, which reports
// the authoritative current version (miHoYo froze zip packages at ~5.5, so
// getGamePackages is stale; getGameBranches returns the real latest tag).
type Branch struct {
	Game        GameId      `json:"game"`
	Main        GameBranch  `json:"main"`
	PreDownload *GameBranch `json:"pre_download"`
}

// GameBranch is one branch (main or pre-download) of a game.
type GameBranch struct {
	PackageID string   `json:"package_id"`
	Branch    string   `json:"branch"`
	Password  string   `json:"password"`
	Tag       string   `json:"tag"`
	DiffTags  []string `json:"diff_tags"`
}

type branchResponse struct {
	Retcode int    `json:"retcode"`
	Message string `json:"message"`
	Data    struct {
		GameBranches []Branch `json:"game_branches"`
	} `json:"data"`
}

// FetchBranch queries getGameBranches and returns the Genshin branch (biz
// starts with "hk4e_"), giving the correct current version and pre-download.
func (c *Client) FetchBranch(ctx context.Context, ed edition.Edition) (*Branch, error) {
	url := ed.Host + "/getGameBranches?launcher_id=" + ed.LauncherID
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("getGameBranches status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	var r branchResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if r.Retcode != 0 {
		return nil, fmt.Errorf("getGameBranches retcode %d: %s", r.Retcode, r.Message)
	}
	for i := range r.Data.GameBranches {
		b := r.Data.GameBranches[i]
		if len(b.Game.Biz) >= 5 && b.Game.Biz[:5] == "hk4e_" {
			return &b, nil
		}
	}
	return nil, fmt.Errorf("未在 getGameBranches 中找到原神 (edition=%s)", ed.Name)
}

// PatchTo returns the patch whose base version equals fromVersion.
func (g *GameInfo) PatchTo(fromVersion string) *Patch {
	for i := range g.Patches {
		if g.Patches[i].Version == fromVersion {
			return &g.Patches[i]
		}
	}
	return nil
}

// Protocol is the user agreement returned by the launcher protocol API.
type Protocol struct {
	Title            string `json:"title"`
	AgreementVersion string `json:"agreement_version"`
	Content          string `json:"protocol"`
}

type protocolResponse struct {
	Retcode int      `json:"retcode"`
	Message string   `json:"message"`
	Data    Protocol `json:"data"`
}

// Protocol fetches the current server's user agreement (user protocol).
func (c *Client) Protocol(ctx context.Context, ed edition.Edition) (*Protocol, error) {
	url := fmt.Sprintf("%s?key=%s&launcher_id=%s&language=zh-cn",
		ed.ProtocolURL, ed.ProtocolKey, ed.ProtocolLauncherID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120 Safari/537.36")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("protocol status %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var r protocolResponse
	if err := json.Unmarshal(body, &r); err != nil {
		return nil, err
	}
	if r.Retcode != 0 {
		return nil, fmt.Errorf("protocol retcode %d: %s", r.Retcode, r.Message)
	}
	return &r.Data, nil
}

// ParseInt parses a size string (bytes) to int64.
func ParseInt(s string) int64 {
	n, _ := strconv.ParseInt(s, 10, 64)
	return n
}
