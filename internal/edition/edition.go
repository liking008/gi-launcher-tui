package edition

import "fmt"

// Edition describes one Genshin server/client variant (官服 / B服 / 国际服).
// Switching between editions rewrites config.ini and manages the account SDK
// file, mirroring Snap.Hutao's LaunchScheme switching.
type Edition struct {
	// Name is the display name shown in the launcher.
	Name string
	// Key is a stable identifier.
	Key string
	// LauncherID is used by the hyp-connect getGamePackages API.
	LauncherID string
	// Host is the hyp-connect API host.
	Host string
	// Config values written into config.ini.
	Channel    string
	SubChannel string
	// IsOversea selects the international game (hk4e_global).
	IsOversea bool
	// NeedsSDK: the Bilibili client ships an extra PCGameSDK.dll plugin.
	NeedsSDK bool
	// GameBiz is the miHoYo business id used in config.ini's uapc field.
	GameBiz string
	// ExeName is the game executable.
	ExeName string
	// DataFolder is the game's data folder.
	DataFolder string
	// ProtocolURL / ProtocolKey / ProtocolLauncherID fetch the user agreement
	// via the launcher protocol API (UIGF-documented).
	ProtocolURL        string
	ProtocolKey        string
	ProtocolLauncherID string
}

const cnHost = "https://hyp-api.mihoyo.com/hyp/hyp-connect/api"
const osHost = "https://sg-hyp-api.hoyoverse.com/hyp/hyp-connect/api"

const cnProtocol = "https://sdk-static.mihoyo.com/hk4e_cn/mdk/launcher/api/protocol"
const osProtocol = "https://hk4e-launcher-static.hoyoverse.com/hk4e_global/mdk/launcher/api/protocol"

// cps written into config.ini (Snap.Hutao uses gw_pc for every server).
const configCps = "gw_pc"

var (
	OfficialCN = Edition{
		Name:               "官服",
		Key:                "official_cn",
		LauncherID:         "jGHBHlcOq1",
		Host:               cnHost,
		Channel:            "1",
		SubChannel:         "1",
		GameBiz:            "hk4e_cn",
		ExeName:            "YuanShen.exe",
		DataFolder:         "YuanShen_Data",
		ProtocolURL:        cnProtocol,
		ProtocolKey:        "KAtdSsoQ",
		ProtocolLauncherID: "17",
	}
	Bilibili = Edition{
		Name:               "B服",
		Key:                "bilibili",
		LauncherID:         "umfgRO5gh5",
		Host:               cnHost,
		Channel:            "14",
		SubChannel:         "0",
		GameBiz:            "hk4e_cn",
		ExeName:            "YuanShen.exe",
		DataFolder:         "YuanShen_Data",
		NeedsSDK:           true,
		ProtocolURL:        cnProtocol,
		ProtocolKey:        "KAtdSsoQ",
		ProtocolLauncherID: "17",
	}
	Oversea = Edition{
		Name:               "国际服",
		Key:                "oversea",
		LauncherID:         "VYTpXlbWo8",
		Host:               osHost,
		Channel:            "1",
		SubChannel:         "0",
		IsOversea:          true,
		GameBiz:            "hk4e_global",
		ExeName:            "GenshinImpact.exe",
		DataFolder:         "GenshinImpact_Data",
		ProtocolURL:        osProtocol,
		ProtocolKey:        "gcStgarh",
		ProtocolLauncherID: "10",
	}
)

// All returns every selectable edition.
func All() []Edition {
	return []Edition{OfficialCN, Bilibili, Oversea}
}

// Get returns the edition matching key.
func Get(key string) (Edition, error) {
	for _, e := range All() {
		if e.Key == key {
			return e, nil
		}
	}
	return Edition{}, fmt.Errorf("未知服务器: %s", key)
}

// PackagesURL builds the getGamePackages URL for this edition.
func (e Edition) PackagesURL() string {
	return e.Host + "/getGamePackages?launcher_id=" + e.LauncherID
}

// CPS returns the cps value written to config.ini.
func (e Edition) CPS() string { return configCps }

// UAPC returns the config.ini uapc value embedding the game biz.
func (e Edition) UAPC() string {
	return fmt.Sprintf(`{"%s":{"uapc":""},"hyp":{"uapc":""}}`, e.GameBiz)
}

// SophonDataURL returns the base URL for the sophon chunk download API.
func (e Edition) SophonDataURL() string {
	if e.IsOversea {
		return "https://sg-public-api.hoyoverse.com"
	}
	return "https://api-takumi.mihoyo.com"
}

// RegistryKeyValue returns the wine registry key and value name holding the
// miHoYo SDK auto-login blob (MIHOYOSDK_ADL_PROD_*) for this edition, mirroring
// Snap.Hutao's scheme mapping. The blob lives in $WINEPREFIX/user.reg.
func (e Edition) RegistryKeyValue() (key, value string) {
	if e.IsOversea {
		return `Software\miHoYo\Genshin Impact`, "MIHOYOSDK_ADL_PROD_OVERSEA_h1158948810"
	}
	return `Software\miHoYo\原神`, "MIHOYOSDK_ADL_PROD_CN_h3123967166"
}
