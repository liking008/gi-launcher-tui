package account

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetReadRegistryBinaryRoundTrip(t *testing.T) {
	prefix := t.TempDir()
	key := `Software\miHoYo\原神`
	value := "MIHOYOSDK_ADL_PROD_CN_h3123967166"

	blob := "MDAwMDAwMDAtMDAwMC0wMDAwLTAwMDAtMDAwMDAwMDAwMDAw"
	if err := SetRegistryBinary("", prefix, key, value, blob); err != nil {
		t.Fatal(err)
	}
	got, ok := ReadRegistryBinary("", prefix, key, value)
	if !ok {
		t.Fatal("expected blob to be readable")
	}
	if got != blob {
		t.Fatalf("round-trip mismatch: got %q want %q", got, blob)
	}
}

func TestSetReplacesExistingValue(t *testing.T) {
	prefix := t.TempDir()
	key := `Software\miHoYo\原神`
	value := "MIHOYOSDK_ADL_PROD_CN_h3123967166"

	if err := SetRegistryBinary("", prefix, key, value, "first"); err != nil {
		t.Fatal(err)
	}
	if err := SetRegistryBinary("", prefix, key, value, "second-value"); err != nil {
		t.Fatal(err)
	}
	got, ok := ReadRegistryBinary("", prefix, key, value)
	if !ok || got != "second-value" {
		t.Fatalf("replacement failed: got %q ok=%v", got, ok)
	}
}

func TestEscapedKeyReadFromExistingFile(t *testing.T) {
	// Simulate a wine-written user.reg where 原神 is stored as \x539f\x795e.
	prefix := t.TempDir()
	reg := `WINE REGISTRY Version 2
[Software\\miHoYo\\\x539f\x795e] 1788733372
"GENERAL_DATA_h2389025596"=hex:31,32,33
"MIHOYOSDK_ADL_PROD_CN_h3123967166"=hex:61,62,63,64,65,00
`
	if err := os.WriteFile(filepath.Join(prefix, "user.reg"), []byte(reg), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := ReadRegistryBinary("", prefix, `Software\miHoYo\原神`, "MIHOYOSDK_ADL_PROD_CN_h3123967166")
	if !ok || got != "abcde" {
		t.Fatalf("expected abcde, got %q ok=%v", got, ok)
	}
}

func TestReadFromProtonPfxSubdir(t *testing.T) {
	// Proton materialises the real prefix under <compat>/pfx.
	prefix := t.TempDir()
	pfx := filepath.Join(prefix, "pfx")
	if err := os.MkdirAll(pfx, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := `WINE REGISTRY Version 2
[Software\\miHoYo\\\x539f\x795e] 1788734632
"MIHOYOSDK_ADL_PROD_CN_h3123967166"=hex:61,62,63,64,65,66,00
`
	if err := os.WriteFile(filepath.Join(pfx, "user.reg"), []byte(reg), 0o600); err != nil {
		t.Fatal(err)
	}
	got, ok := ReadRegistryBinary("", prefix, `Software\miHoYo\原神`, "MIHOYOSDK_ADL_PROD_CN_h3123967166")
	if !ok || got != "abcdef" {
		t.Fatalf("expected abcdef from pfx subdir, got %q ok=%v", got, ok)
	}

	// Set must also edit the pfx user.reg, not create a fresh direct one.
	if err := SetRegistryBinary("", prefix, `Software\miHoYo\原神`, "MIHOYOSDK_ADL_PROD_CN_h3123967166", "newblob"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(prefix, "user.reg")); err == nil {
		t.Fatal("should not create user.reg at prefix root when pfx/user.reg exists")
	}
	got, ok = ReadRegistryBinary("", prefix, `Software\miHoYo\原神`, "MIHOYOSDK_ADL_PROD_CN_h3123967166")
	if !ok || got != "newblob" {
		t.Fatalf("expected newblob after Set, got %q ok=%v", got, ok)
	}
}

func TestStoreRename(t *testing.T) {
	path := filepath.Join(t.TempDir(), "accounts.json")
	s := NewStore(path, t.TempDir())
	if err := s.Upsert(Profile{Name: "账号1", MihoyoSDK: "blob"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("账号1", "主号"); err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Get("账号1"); ok {
		t.Fatal("old key should be gone")
	}
	p, ok := s.Get("主号")
	if !ok || p.MihoyoSDK != "blob" {
		t.Fatalf("renamed profile lost data: %+v ok=%v", p, ok)
	}
	// renaming to an existing name must fail.
	if err := s.Upsert(Profile{Name: "其他号", MihoyoSDK: "b"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Rename("主号", "其他号"); err == nil {
		t.Fatal("expected conflict error")
	}
}
