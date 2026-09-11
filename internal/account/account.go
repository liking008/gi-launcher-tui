package account

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Profile is a named account record. MihoyoSDK holds the raw registry blob
// (MIHOYOSDK_ADL_PROD_*) that the game SDK writes on login; applying a profile
// writes that blob back into the wine prefix registry so the game auto-logs in.
type Profile struct {
	Name      string `json:"name"`
	MihoyoSDK string `json:"mihoyo_sdk,omitempty"` // auto-login blob read from user.reg
	Cookie    string `json:"cookie,omitempty"`     // legacy full Cookie header value
	UID       string `json:"uid,omitempty"`
	Remark    string `json:"remark,omitempty"`
}

// Store persists account profiles as JSON.
type Store struct {
	path     string
	dir      string
	profiles map[string]Profile
}

func NewStore(path, dir string) *Store {
	s := &Store{path: path, dir: dir, profiles: map[string]Profile{}}
	s.load()
	return s
}

func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, &s.profiles)
}

func (s *Store) Save() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s.profiles, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0o600)
}

func (s *Store) All() []Profile {
	names := make([]string, 0, len(s.profiles))
	for n := range s.profiles {
		names = append(names, n)
	}
	sort.Strings(names)
	out := make([]Profile, 0, len(names))
	for _, n := range names {
		out = append(out, s.profiles[n])
	}
	return out
}

func (s *Store) Get(name string) (Profile, bool) {
	p, ok := s.profiles[name]
	return p, ok
}

func (s *Store) Upsert(p Profile) error {
	s.profiles[p.Name] = p
	return s.Save()
}

func (s *Store) Remove(name string) error {
	delete(s.profiles, name)
	return s.Save()
}

// Rename changes a profile's label (key), preserving its data.
func (s *Store) Rename(old, new string) error {
	p, ok := s.profiles[old]
	if !ok {
		return fmt.Errorf("账号 %s 不存在", old)
	}
	if _, exists := s.profiles[new]; exists {
		return fmt.Errorf("账号 %s 已存在", new)
	}
	delete(s.profiles, old)
	p.Name = new
	s.profiles[new] = p
	return s.Save()
}

// Apply writes the profile's auto-login blob into the wine prefix registry so
// the game starts with that account. key/value identify the MIHOYOSDK registry
// entry for the active edition (see edition.RegistryKeyValue). wine is the wine
// binary used to reach the live registry (may be empty to edit user.reg).
func (s *Store) Apply(wine, prefix, key, value string, p Profile) error {
	if p.MihoyoSDK == "" {
		return fmt.Errorf("账号 %s 没有保存的登录数据", p.Name)
	}
	return SetRegistryBinary(wine, prefix, key, value, p.MihoyoSDK)
}

// userRegPath locates the HKCU registry backing file inside a wine prefix.
// Proton (used to launch the game) materialises its real prefix under
// <STEAM_COMPAT_DATA_PATH>/pfx, so the user.reg may live one level deeper.
// Prefer whichever path already exists; fall back to the direct location.
func userRegPath(prefix string) string {
	candidates := []string{
		filepath.Join(prefix, "user.reg"),
		filepath.Join(prefix, "pfx", "user.reg"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return candidates[0]
}

// escapeRegKey converts a registry key path into wine's user.reg section header
// escaping: non-ASCII characters become \xNN, backslashes double up. Spaces are
// kept literal (matching how wine writes "Genshin Impact").
func escapeRegKey(key string) string {
	var b strings.Builder
	for _, r := range key {
		switch {
		case r == '\\':
			b.WriteString(`\\`)
		case r >= 0x21 && r <= 0x7e && r != '"':
			b.WriteRune(r)
		default:
			fmt.Fprintf(&b, `\x%02x`, r)
		}
	}
	return b.String()
}

// formatHex renders bytes as wine's hex:aa,bb,... value, wrapped every 30 bytes
// with wine-style line continuations.
func formatHex(s string) string {
	b := []byte(s)
	var sb strings.Builder
	for i := 0; i < len(b); i++ {
		if i > 0 {
			if i%30 == 0 {
				sb.WriteString(",\\\n  ")
			} else {
				sb.WriteString(",")
			}
		}
		fmt.Fprintf(&sb, "%02x", b[i])
	}
	return sb.String()
}

func parseHex(s string) ([]byte, error) {
	s = strings.ReplaceAll(s, "\\", "")
	s = strings.ReplaceAll(s, ",", "")
	s = strings.ReplaceAll(s, " ", "")
	return hex.DecodeString(s)
}

func writeAtomic(path string, lines []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	data := []byte(strings.Join(lines, "\n") + "\n")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SetRegistryBinary writes a REG_BINARY value to the active registry. When wine
// is given it goes through `wine reg` so a running game sees the change and the
// live value is updated; otherwise the on-disk user.reg is edited directly.
func SetRegistryBinary(wine, prefix, key, valueName, blob string) error {
	if wine != "" {
		if err := setRegistryBinaryLive(wine, prefix, key, valueName, blob); err == nil {
			return nil
		}
	}
	return setRegistryBinaryFile(prefix, key, valueName, blob)
}

// ReadRegistryBinary reads a REG_BINARY value from the active registry. When
// wine is given it queries the live registry (a running game's login data is
// held in wineserver memory and only flushed to user.reg on exit, so reading the
// file would be stale); otherwise it parses user.reg.
func ReadRegistryBinary(wine, prefix, key, valueName string) (string, bool) {
	if wine != "" {
		if v, ok := readRegistryBinaryLive(wine, prefix, key, valueName); ok {
			return v, true
		}
	}
	return readRegistryBinaryFile(prefix, key, valueName)
}

// wineReg runs a `wine reg` subcommand against prefix and returns its output.
func wineReg(wine, prefix string, args ...string) (string, error) {
	cmd := exec.Command(wine, append([]string{"reg"}, args...)...)
	cmd.Env = append(os.Environ(), "WINEPREFIX="+prefix)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// hkcuKey qualifies a HKCU-relative registry path (the form used in user.reg)
// with the root that `wine reg` expects.
func hkcuKey(key string) string {
	if strings.HasPrefix(key, "HKCU") || strings.HasPrefix(key, "HKEY_") {
		return key
	}
	return `HKCU\` + key
}

// setRegistryBinaryLive writes via `wine reg add`, reaching the live registry.
func setRegistryBinaryLive(wine, prefix, key, valueName, blob string) error {
	hexData := hex.EncodeToString([]byte(blob + "\x00"))
	_, err := wineReg(wine, prefix, "add", hkcuKey(key), "/v", valueName,
		"/t", "REG_BINARY", "/d", hexData, "/f")
	return err
}

// readRegistryBinaryLive queries via `wine reg query` and parses the REG_BINARY
// hex dump, returning the blob without its trailing NUL.
func readRegistryBinaryLive(wine, prefix, key, valueName string) (string, bool) {
	out, err := wineReg(wine, prefix, "query", hkcuKey(key), "/v", valueName)
	if err != nil {
		return "", false
	}
	for _, line := range strings.Split(out, "\n") {
		if !strings.Contains(line, "REG_BINARY") {
			continue
		}
		hexStr := strings.TrimSpace(line[strings.Index(line, "REG_BINARY")+len("REG_BINARY"):])
		hexStr = strings.NewReplacer(",", "", " ", "").Replace(hexStr)
		raw, err := hex.DecodeString(hexStr)
		if err != nil {
			return "", false
		}
		return strings.TrimSuffix(string(raw), "\x00"), true
	}
	return "", false
}

// setRegistryBinaryFile writes a REG_BINARY value into a key section of the
// prefix's user.reg, creating the section if needed. The value is the UTF-8 blob
// followed by a NUL terminator, exactly as the game SDK expects.
func setRegistryBinaryFile(prefix, key, valueName, blob string) error {
	path := userRegPath(prefix)
	var lines []string
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			return err
		}
	} else {
		lines = strings.Split(string(data), "\n")
	}

	header := "[" + escapeRegKey(key) + "]"
	valueLine := `"` + valueName + `"=`
	valueText := valueLine + "hex:" + formatHex(blob+"\x00")

	for i := 0; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(t, "[") || !strings.HasPrefix(t, header) {
			continue
		}
		// Section found. Locate an existing value to replace, else insert.
		for j := i + 1; j < len(lines); j++ {
			t2 := strings.TrimSpace(lines[j])
			if strings.HasPrefix(t2, "[") {
				break // next section
			}
			if strings.HasPrefix(t2, valueLine) {
				end := j + 1
				for end < len(lines) && isContinuation(lines[end]) {
					end++
				}
				out := append([]string{}, lines[:j]...)
				out = append(out, valueText)
				out = append(out, lines[end:]...)
				return writeAtomic(path, out)
			}
		}
		insertAt := i + 1
		for insertAt < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[insertAt]), "#time=") {
			insertAt++
		}
		out := append([]string{}, lines[:insertAt]...)
		out = append(out, valueText)
		out = append(out, lines[insertAt:]...)
		return writeAtomic(path, out)
	}

	// No matching section: append one at the end.
	if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) != "" {
		lines = append(lines, "")
	}
	lines = append(lines, header+" "+fmt.Sprint(time.Now().Unix()), valueText)
	return writeAtomic(path, lines)
}

// readRegistryBinaryFile parses a REG_BINARY value from the prefix's user.reg,
// returning the blob without its trailing NUL. ok is false when absent.
func readRegistryBinaryFile(prefix, key, valueName string) (string, bool) {
	data, err := os.ReadFile(userRegPath(prefix))
	if err != nil {
		return "", false
	}
	lines := strings.Split(string(data), "\n")
	header := "[" + escapeRegKey(key) + "]"
	valueLine := `"` + valueName + `"=`
	inSection := false
	for i := 0; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "[") {
			inSection = strings.HasPrefix(t, header)
			continue
		}
		if !inSection || !strings.HasPrefix(t, valueLine) {
			continue
		}
		var sb strings.Builder
		sb.WriteString(strings.TrimPrefix(t[len(valueLine):], "hex:"))
		for i+1 < len(lines) && isContinuation(lines[i+1]) {
			i++
			sb.WriteString(strings.TrimSpace(lines[i]))
		}
		raw, err := parseHex(sb.String())
		if err != nil {
			return "", false
		}
		return strings.TrimSuffix(string(raw), "\x00"), true
	}
	return "", false
}

// isContinuation reports whether a user.reg line continues the previous value
// (wine indents wrapped hex values with leading whitespace).
func isContinuation(line string) bool {
	return strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t")
}
