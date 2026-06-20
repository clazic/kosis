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

	if cfg.AI.Default != "claude" {
		t.Errorf("expected AI default=claude, got %s", cfg.AI.Default)
	}

	if len(cfg.AI.Tools) != 3 {
		t.Errorf("expected 3 default AI tools, got %d", len(cfg.AI.Tools))
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

func TestAddRemoveAITool(t *testing.T) {
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

	// AI 도구 추가
	if err := AddAITool("custom", "custom -p '{prompt}'"); err != nil {
		t.Fatalf("AddAITool failed: %v", err)
	}

	cfg, _ = Load()
	if _, exists := cfg.AI.Tools["custom"]; !exists {
		t.Error("custom tool was not added")
	}

	// 중복 도구 추가 시도
	if err := AddAITool("custom", "cmd"); err == nil {
		t.Error("AddAITool should fail for duplicate tool")
	}

	// AI 도구 제거
	if err := RemoveAITool("custom"); err != nil {
		t.Fatalf("RemoveAITool failed: %v", err)
	}

	cfg, _ = Load()
	if _, exists := cfg.AI.Tools["custom"]; exists {
		t.Error("custom tool was not removed")
	}
}

func TestSetAIDefault(t *testing.T) {
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

	// 기본 AI 도구 변경
	if err := SetAIDefault("gemini"); err != nil {
		t.Fatalf("SetAIDefault failed: %v", err)
	}

	cfg, _ = Load()
	if cfg.AI.Default != "gemini" {
		t.Errorf("expected default=gemini, got %s", cfg.AI.Default)
	}

	// 존재하지 않는 도구로 설정 시도
	if err := SetAIDefault("nonexistent"); err == nil {
		t.Error("SetAIDefault should fail for nonexistent tool")
	}
}

func TestListAITools(t *testing.T) {
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

	tools, err := ListAITools()
	if err != nil {
		t.Fatalf("ListAITools failed: %v", err)
	}

	if len(tools) != 3 {
		t.Errorf("expected 3 tools, got %d", len(tools))
	}

	// 각 도구가 올바른 구조인지 확인
	for _, tool := range tools {
		if tool.Name == "" {
			t.Error("tool name should not be empty")
		}
		if tool.Cmd == "" {
			t.Error("tool cmd should not be empty")
		}
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
