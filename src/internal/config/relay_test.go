package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/clazic/kosis/internal/relay"
)

// 중계 모드: 설정 파일 · KOSIS_API_KEY 의 키를 쓰지 않고 자리표시를 돌려준다. 설정 파일은 건드리지 않는다.
func TestRelayIgnoresConfigKeys(t *testing.T) {
	dir := t.TempDir()
	SetConfigDirForTesting(dir)
	t.Cleanup(func() { SetConfigDirForTesting(""); relay.ResetForTesting(false) })
	file := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(file, []byte("api_keys:\n  - FILEKEY1\n  - FILEKEY2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KOSIS_API_KEY", "")

	// 중계 모드 아님 — 지금과 같다
	t.Setenv("PES_RELAY_URL", "")
	relay.ResetForTesting(true)
	keys, err := GetAPIKeys()
	if err != nil || len(keys) != 2 || keys[0] != "FILEKEY1" {
		t.Fatalf("중계 모드가 아니면 설정 파일 키: %v %v", keys, err)
	}

	t.Setenv("PES_RELAY_URL", "http://127.0.0.1:53999")
	t.Setenv("PES_RELAY_KOSIS_KEYS", "3")
	t.Setenv("KOSIS_API_KEY", "ENVKEY")
	keys, err = GetAPIKeys()
	if err != nil || len(keys) != 3 || keys[0] != "__PES_KEY_0__" || keys[2] != "__PES_KEY_2__" {
		t.Fatalf("중계 모드 키: %v %v", keys, err)
	}
	if first, _ := GetFirstAPIKey(); first != "__PES_KEY_0__" {
		t.Errorf("GetFirstAPIKey: %s", first)
	}
	if !HasAPIKey() {
		t.Error("HasAPIKey 는 PES 키 수를 따라야 한다")
	}

	// 키 수 0: 설정 파일 · 환경변수에 키가 있어도 없다고 알린다
	t.Setenv("PES_RELAY_KOSIS_KEYS", "0")
	if _, err := GetAPIKeys(); err == nil || err.Error() != relay.NoKeyMessage {
		t.Errorf("키 없음 글: %v", err)
	}
	if HasAPIKey() {
		t.Error("PES 키가 0 이면 HasAPIKey 는 false")
	}

	// Load 는 그대로(자리표시가 설정 파일에 저장되는 일이 없게), 파일도 그대로
	t.Setenv("KOSIS_API_KEY", "")
	cfg, err := Load()
	if err != nil || len(cfg.APIKeys) != 2 || cfg.APIKeys[0] != "FILEKEY1" {
		t.Errorf("Load 는 설정 파일 그대로여야 한다: %v %v", cfg, err)
	}
	b, _ := os.ReadFile(file)
	if string(b) != "api_keys:\n  - FILEKEY1\n  - FILEKEY2\n" {
		t.Errorf("설정 파일이 바뀌었다")
	}
}
