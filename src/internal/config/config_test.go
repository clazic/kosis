package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Error("DefaultConfig() should not return nil")
	}

	if cfg.DefaultFormat != "table" {
		t.Errorf("expected default_format=table, got %s", cfg.DefaultFormat)
	}

	if cfg.CacheTTLHours != 24 {
		t.Errorf("expected cache_ttl_hours=24, got %d", cfg.CacheTTLHours)
	}

	if cfg.AI.Default != DefaultAIProvider {
		t.Errorf("expected AI default=%s, got %s", DefaultAIProvider, cfg.AI.Default)
	}

	if _, ok := cfg.AI.Providers[DefaultAIProvider]; !ok {
		t.Errorf("기본 공급자 %s가 Providers에 없음", DefaultAIProvider)
	}
}

func TestConfigDir(t *testing.T) {
	SetConfigDirForTesting("")
	defer SetConfigDirForTesting("")

	dir := ConfigDir()
	if dir == "" {
		t.Error("ConfigDir() should not return empty string")
	}

	// macOS/Linux에서는 ~/ 형태여야 함
	homeDir, _ := os.UserHomeDir()
	expectedDir := filepath.Join(homeDir, ".kosis")
	if dir != expectedDir {
		t.Errorf("expected %s, got %s", expectedDir, dir)
	}
}

func TestEnsureConfigDir(t *testing.T) {
	// 테스트용 임시 디렉토리 설정
	tmpDir := t.TempDir()
	SetConfigDirForTesting(tmpDir)
	defer SetConfigDirForTesting("")

	// EnsureConfigDir 테스트
	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir failed: %v", err)
	}

	dir := ConfigDir()
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("config directory does not exist: %v", err)
	}

	// 두 번 실행해도 에러가 없어야 함
	if err := EnsureConfigDir(); err != nil {
		t.Fatalf("EnsureConfigDir failed on second call: %v", err)
	}
}

func TestAddRemoveAPIKey(t *testing.T) {
	// 테스트용 임시 디렉토리 설정
	tmpDir := t.TempDir()
	SetConfigDirForTesting(tmpDir)
	defer SetConfigDirForTesting("")

	// 환경변수 해제 (테스트 독립성)
	os.Unsetenv("KOSIS_API_KEY")

	// 테스트: API 키 추가
	cfg := DefaultConfig()
	if err := Save(cfg); err != nil {
		t.Fatalf("설정 저장 실패: %v", err)
	}

	// 저장 후 테스트
	cfg, _ = Load()
	initialLen := len(cfg.APIKeys)

	if err := AddAPIKey("test_key_1"); err != nil {
		t.Fatalf("AddAPIKey failed: %v", err)
	}

	cfg, _ = Load()
	if len(cfg.APIKeys) != initialLen+1 {
		t.Errorf("expected %d keys, got %d", initialLen+1, len(cfg.APIKeys))
	}

	// 중복 키 추가 시도
	if err := AddAPIKey("test_key_1"); err == nil {
		t.Error("AddAPIKey should fail for duplicate key")
	}

	// 빈 키 추가 시도
	if err := AddAPIKey(""); err == nil {
		t.Error("AddAPIKey should fail for empty key")
	}
}

func TestSetDefaultKey(t *testing.T) {
	// 테스트용 임시 디렉토리 설정
	tmpDir := t.TempDir()
	SetConfigDirForTesting(tmpDir)
	defer SetConfigDirForTesting("")

	// 환경변수 해제
	os.Unsetenv("KOSIS_API_KEY")

	cfg := DefaultConfig()
	if err := Save(cfg); err != nil {
		t.Fatalf("설정 저장 실패: %v", err)
	}

	if err := SetDefaultKey("my_key"); err != nil {
		t.Fatalf("SetDefaultKey failed: %v", err)
	}

	cfg, _ = Load()
	if len(cfg.APIKeys) != 1 {
		t.Errorf("expected 1 key, got %d", len(cfg.APIKeys))
	}
	if cfg.APIKeys[0] != "my_key" {
		t.Errorf("expected 'my_key', got %s", cfg.APIKeys[0])
	}
}

func TestGetAPIKeysWithEnv(t *testing.T) {
	os.Setenv("KOSIS_API_KEY", "env_key")
	defer os.Unsetenv("KOSIS_API_KEY")

	keys, err := GetAPIKeys()
	if err != nil {
		t.Fatalf("GetAPIKeys failed: %v", err)
	}

	if len(keys) != 1 {
		t.Errorf("expected 1 key, got %d", len(keys))
	}

	if keys[0] != "env_key" {
		t.Errorf("expected 'env_key', got %s", keys[0])
	}
}

func useTempConfig(t *testing.T) {
	t.Helper()
	SetConfigDirForTesting(t.TempDir())
	t.Cleanup(func() { SetConfigDirForTesting("") })
	os.Unsetenv("KOSIS_API_KEY")
}

func TestAIProviderLifecycle(t *testing.T) {
	useTempConfig(t)

	p := AIProvider{BaseURL: "http://localhost:11434/v1", Model: "llama3"}
	if err := AddAIProvider("ollama", p); err != nil {
		t.Fatalf("AddAIProvider failed: %v", err)
	}
	if err := AddAIProvider("bad", AIProvider{BaseURL: "localhost:11434", Model: "x"}); err == nil {
		t.Error("스킴 없는 URL은 거부해야 함")
	}
	if err := AddAIProvider("bad", AIProvider{BaseURL: "http://h/v1"}); err == nil {
		t.Error("모델 없는 공급자는 거부해야 함")
	}

	if err := SetAIDefault("ollama"); err != nil {
		t.Fatalf("SetAIDefault failed: %v", err)
	}
	if err := SetAIDefault("nonexistent"); err == nil {
		t.Error("SetAIDefault should fail for nonexistent provider")
	}

	if err := SetAIProviderKey("ollama", "sk-test-1234567890"); err != nil {
		t.Fatalf("SetAIProviderKey failed: %v", err)
	}
	name, got, err := GetAIProvider("")
	if err != nil || name != "ollama" || got.APIKey != "sk-test-1234567890" {
		t.Fatalf("GetAIProvider(\"\") = %s %+v %v", name, got, err)
	}

	// 기본 공급자를 지우면 내장 기본값으로 복귀
	if err := RemoveAIProvider("ollama"); err != nil {
		t.Fatalf("RemoveAIProvider failed: %v", err)
	}
	cfg, _ := Load()
	if _, exists := cfg.AI.Providers["ollama"]; exists {
		t.Error("ollama was not removed")
	}
	if cfg.AI.Default != DefaultAIProvider {
		t.Errorf("기본값 복귀 실패: %s", cfg.AI.Default)
	}

	if err := RemoveAIProvider(DefaultAIProvider); err == nil {
		t.Error("내장 공급자는 제거할 수 없어야 함")
	}
}

func TestResolveKeyEnvWins(t *testing.T) {
	p := AIProvider{APIKey: "from-config", KeyEnv: "KOSIS_TEST_AI_KEY"}
	if got := p.ResolveKey(); got != "from-config" {
		t.Errorf("env 미설정 시 config 키여야 함, got %q", got)
	}
	t.Setenv("KOSIS_TEST_AI_KEY", "from-env")
	if got := p.ResolveKey(); got != "from-env" {
		t.Errorf("env가 우선해야 함, got %q", got)
	}
}

// 예전 CLI 도구 방식 설정(ai.tools + default: gemini)은 hermes 기본으로 이관돼야 한다.
func TestLegacyAIConfigMigrates(t *testing.T) {
	useTempConfig(t)
	legacy := "ai:\n  default: gemini\n  tools:\n    gemini:\n      cmd: gemini -p '{prompt}'\napi_keys: []\n"
	if err := os.WriteFile(ConfigFilePath(), []byte(legacy), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	if cfg.AI.Default != DefaultAIProvider {
		t.Errorf("default = %s, want %s", cfg.AI.Default, DefaultAIProvider)
	}
	if _, ok := cfg.AI.Providers[DefaultAIProvider]; !ok {
		t.Error("내장 공급자가 채워지지 않음")
	}

	// 저장 후 다시 읽어도 base_url 등 키 이름이 보존돼야 한다 (yaml/mapstructure 태그 일치)
	if err := Save(cfg); err != nil {
		t.Fatal(err)
	}
	cfg2, _ := Load()
	if cfg2.AI.Providers[DefaultAIProvider].BaseURL == "" {
		t.Error("저장 후 base_url 유실")
	}
}

func TestNoAPIKeyMessage(t *testing.T) {
	msg := NoAPIKeyMessage()
	if msg == "" {
		t.Error("NoAPIKeyMessage() should return non-empty string")
	}

	// 메시지에 주요 내용이 포함되어 있는지 확인
	if !contains(msg, "API 키") {
		t.Error("message should mention API 키")
	}
	if !contains(msg, "KOSIS_API_KEY") {
		t.Error("message should mention KOSIS_API_KEY")
	}
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func TestHasAPIKey(t *testing.T) {
	// 환경변수로 설정
	os.Setenv("KOSIS_API_KEY", "test_key")
	defer os.Unsetenv("KOSIS_API_KEY")

	if !HasAPIKey() {
		t.Error("HasAPIKey should return true when KOSIS_API_KEY is set")
	}
}

func TestGetFirstAPIKey(t *testing.T) {
	os.Setenv("KOSIS_API_KEY", "first_key")
	defer os.Unsetenv("KOSIS_API_KEY")

	key, err := GetFirstAPIKey()
	if err != nil {
		t.Fatalf("GetFirstAPIKey failed: %v", err)
	}

	if key != "first_key" {
		t.Errorf("expected 'first_key', got %s", key)
	}
}

func TestSetDefaultFormat(t *testing.T) {
	// 테스트용 임시 디렉토리 설정
	tmpDir := t.TempDir()
	SetConfigDirForTesting(tmpDir)
	defer SetConfigDirForTesting("")

	os.Unsetenv("KOSIS_API_KEY")

	cfg := DefaultConfig()
	if err := Save(cfg); err != nil {
		t.Fatalf("설정 저장 실패: %v", err)
	}

	// 유효한 형식 설정
	if err := SetDefaultFormat("json"); err != nil {
		t.Fatalf("SetDefaultFormat failed: %v", err)
	}

	cfg, _ = Load()
	if cfg.DefaultFormat != "json" {
		t.Errorf("expected json, got %s", cfg.DefaultFormat)
	}

	// 유효하지 않은 형식 설정 시도
	if err := SetDefaultFormat("invalid"); err == nil {
		t.Error("SetDefaultFormat should fail for invalid format")
	}
}

func TestSetCacheTTL(t *testing.T) {
	// 테스트용 임시 디렉토리 설정
	tmpDir := t.TempDir()
	SetConfigDirForTesting(tmpDir)
	defer SetConfigDirForTesting("")

	os.Unsetenv("KOSIS_API_KEY")

	cfg := DefaultConfig()
	if err := Save(cfg); err != nil {
		t.Fatalf("설정 저장 실패: %v", err)
	}

	// 유효한 TTL 설정
	if err := SetCacheTTL(48); err != nil {
		t.Fatalf("SetCacheTTL failed: %v", err)
	}

	cfg, _ = Load()
	if cfg.CacheTTLHours != 48 {
		t.Errorf("expected 48, got %d", cfg.CacheTTLHours)
	}

	// 음수 TTL 설정 시도
	if err := SetCacheTTL(-1); err == nil {
		t.Error("SetCacheTTL should fail for negative TTL")
	}
}
