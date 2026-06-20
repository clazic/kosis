// Package config는 KOSIS CLI의 설정 관리를 담당합니다.
// 설정 파일은 ~/.kosis/config.yaml에 저장됩니다.
// 환경변수 KOSIS_API_KEY는 설정 파일의 API 키보다 우선됩니다.
package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/spf13/viper"
)

// configDirOverride는 테스트용으로 설정 디렉토리를 오버라이드하는 변수입니다.
// 테스트에서만 사용하며, 실제 ~/.kosis 디렉토리를 오염시키지 않기 위해 존재합니다.
var configDirOverride string

// Config는 KOSIS CLI의 전체 설정을 나타냅니다.
type Config struct {
	APIKeys       []string  `mapstructure:"api_keys"`
	DefaultFormat string    `mapstructure:"default_format"`
	CacheTTLHours int       `mapstructure:"cache_ttl_hours"`
	AI            AIConfig  `mapstructure:"ai"`
	UpdateCheck   bool      `mapstructure:"update_check"`
}

// AIConfig는 AI 도구 설정을 나타냅니다.
type AIConfig struct {
	Default string             `mapstructure:"default"`
	Tools   map[string]AITool  `mapstructure:"tools"`
}

// AITool은 개별 AI 도구의 설정을 나타냅니다.
type AITool struct {
	Cmd string `mapstructure:"cmd"`
}

// AIToolInfo는 AI 도구의 정보를 나타냅니다 (설치 여부 포함).
type AIToolInfo struct {
	Name      string
	Cmd       string
	Installed bool
}

// DefaultConfig는 기본 설정을 반환합니다.
func DefaultConfig() *Config {
	return &Config{
		APIKeys:       []string{},
		DefaultFormat: "table",
		CacheTTLHours: 24,
		UpdateCheck:   true,
		AI: AIConfig{
			Default: "claude",
			Tools: map[string]AITool{
				"claude": {
					Cmd: "claude -p '{prompt}'",
				},
				"gemini": {
					Cmd: "gemini -p '{prompt}'",
				},
				"codex": {
					Cmd: "codex -p '{prompt}'",
				},
			},
		},
	}
}

// ConfigDir은 설정 디렉토리 경로를 반환합니다.
// 테스트 환경에서는 configDirOverride가 우선됩니다.
func ConfigDir() string {
	// 테스트용 오버라이드가 설정되었으면 그것을 사용
	if configDirOverride != "" {
		return configDirOverride
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".kosis")
	}
	return filepath.Join(homeDir, ".kosis")
}

// ConfigFilePath는 설정 파일의 전체 경로를 반환합니다.
func ConfigFilePath() string {
	return filepath.Join(ConfigDir(), "config.yaml")
}

// EnsureConfigDir은 설정 디렉토리를 생성합니다 (이미 존재하면 무시).
func EnsureConfigDir() error {
	dir := ConfigDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("설정 디렉토리 생성 실패 (%s): %w", dir, err)
	}
	return nil
}

// Load는 설정 파일을 로드합니다.
// 우선순위: 환경변수(KOSIS_API_KEY) > config.yaml > 기본값
func Load() (*Config, error) {
	v := viper.New()

	// 기본값 설정
	defaults := DefaultConfig()
	v.SetDefault("api_keys", defaults.APIKeys)
	v.SetDefault("default_format", defaults.DefaultFormat)
	v.SetDefault("cache_ttl_hours", defaults.CacheTTLHours)
	v.SetDefault("update_check", defaults.UpdateCheck)
	v.SetDefault("ai", defaults.AI)

	// 설정 파일 경로 설정
	configFile := ConfigFilePath()
	if _, err := os.Stat(configFile); err == nil {
		v.SetConfigFile(configFile)
		v.SetConfigType("yaml")
		if err := v.ReadInConfig(); err != nil {
			return nil, fmt.Errorf("설정 파일 읽기 실패: %w", err)
		}
	}

	// 설정을 구조체로 언마샬링
	cfg := &Config{}
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("설정 파싱 실패: %w", err)
	}

	// 환경변수 KOSIS_API_KEY가 있으면 최우선으로 설정
	if envKey := os.Getenv("KOSIS_API_KEY"); envKey != "" {
		cfg.APIKeys = []string{envKey}
	}

	return cfg, nil
}

// Save는 설정을 파일에 저장합니다.
func Save(cfg *Config) error {
	if err := EnsureConfigDir(); err != nil {
		return err
	}

	v := viper.New()
	v.SetConfigFile(ConfigFilePath())
	v.SetConfigType("yaml")

	// 설정값을 viper에 설정
	v.Set("api_keys", cfg.APIKeys)
	v.Set("default_format", cfg.DefaultFormat)
	v.Set("cache_ttl_hours", cfg.CacheTTLHours)
	v.Set("update_check", cfg.UpdateCheck)
	v.Set("ai", cfg.AI)

	if err := v.WriteConfig(); err != nil {
		// WriteConfig가 실패하면 WriteConfigAs 시도
		if err := v.SafeWriteConfigAs(ConfigFilePath()); err != nil {
			return fmt.Errorf("설정 파일 저장 실패: %w", err)
		}
	}

	return nil
}

// GetAPIKeys는 설정된 API 키 목록을 반환합니다.
// 환경변수 KOSIS_API_KEY가 있으면 최우선으로 반환합니다.
func GetAPIKeys() ([]string, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	if len(cfg.APIKeys) == 0 {
		return nil, errors.New("API 키가 설정되지 않았습니다")
	}

	return cfg.APIKeys, nil
}

// AddAPIKey는 새로운 API 키를 추가합니다.
func AddAPIKey(key string) error {
	if key == "" {
		return errors.New("API 키는 비워둘 수 없습니다")
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	// 중복 확인
	for _, k := range cfg.APIKeys {
		if k == key {
			return fmt.Errorf("이미 등록된 API 키입니다")
		}
	}

	cfg.APIKeys = append(cfg.APIKeys, key)
	return Save(cfg)
}

// RemoveAPIKey는 지정된 인덱스의 API 키를 제거합니다.
func RemoveAPIKey(index int) error {
	cfg, err := Load()
	if err != nil {
		return err
	}

	if index < 0 || index >= len(cfg.APIKeys) {
		return fmt.Errorf("유효하지 않은 인덱스: %d", index)
	}

	cfg.APIKeys = append(cfg.APIKeys[:index], cfg.APIKeys[index+1:]...)
	return Save(cfg)
}

// SetDefaultKey는 단일 키를 설정합니다 (기존 호환성).
func SetDefaultKey(key string) error {
	if key == "" {
		return errors.New("API 키는 비워둘 수 없습니다")
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	cfg.APIKeys = []string{key}
	return Save(cfg)
}

// GetAIConfig는 AI 도구 설정을 반환합니다.
func GetAIConfig() (AIConfig, error) {
	cfg, err := Load()
	if err != nil {
		return AIConfig{}, err
	}

	return cfg.AI, nil
}

// SetAIDefault는 기본 AI 도구를 설정합니다.
func SetAIDefault(name string) error {
	if name == "" {
		return errors.New("AI 도구명은 비워둘 수 없습니다")
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	// 설정된 도구인지 확인
	if _, exists := cfg.AI.Tools[name]; !exists {
		return fmt.Errorf("등록되지 않은 AI 도구: %s", name)
	}

	cfg.AI.Default = name
	return Save(cfg)
}

// AddAITool은 커스텀 AI 도구를 추가합니다.
func AddAITool(name, cmd string) error {
	if name == "" {
		return errors.New("AI 도구명은 비워둘 수 없습니다")
	}
	if cmd == "" {
		return errors.New("명령어는 비워둘 수 없습니다")
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	// 이미 존재하는 도구인지 확인
	if _, exists := cfg.AI.Tools[name]; exists {
		return fmt.Errorf("이미 등록된 AI 도구: %s", name)
	}

	if cfg.AI.Tools == nil {
		cfg.AI.Tools = make(map[string]AITool)
	}

	cfg.AI.Tools[name] = AITool{Cmd: cmd}
	return Save(cfg)
}

// RemoveAITool은 AI 도구를 제거합니다.
func RemoveAITool(name string) error {
	if name == "" {
		return errors.New("AI 도구명은 비워둘 수 없습니다")
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	if _, exists := cfg.AI.Tools[name]; !exists {
		return fmt.Errorf("등록되지 않은 AI 도구: %s", name)
	}

	delete(cfg.AI.Tools, name)

	// 기본 도구가 제거된 경우 처리
	if cfg.AI.Default == name {
		// 남은 도구 중 첫 번째를 기본으로 설정
		for toolName := range cfg.AI.Tools {
			cfg.AI.Default = toolName
			break
		}
		// 도구가 없으면 기본값 설정
		if cfg.AI.Default == name {
			cfg.AI.Default = ""
		}
	}

	return Save(cfg)
}

// ListAITools는 모든 AI 도구의 목록을 반환합니다 (설치 여부 포함).
func ListAITools() ([]AIToolInfo, error) {
	cfg, err := Load()
	if err != nil {
		return nil, err
	}

	var tools []AIToolInfo
	for name, tool := range cfg.AI.Tools {
		info := AIToolInfo{
			Name:      name,
			Cmd:       tool.Cmd,
			Installed: isCommandAvailable(name),
		}
		tools = append(tools, info)
	}

	return tools, nil
}

// isCommandAvailable은 커맨드가 시스템에 설치되어 있는지 확인합니다.
// exec.LookPath를 사용하여 크로스플랫폼 호환성을 제공합니다.
func isCommandAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// NoAPIKeyMessage는 API 키가 없을 때 표시할 안내 메시지를 반환합니다.
func NoAPIKeyMessage() string {
	return `API 키가 설정되지 않았습니다.

설정 방법:

1. 환경변수 사용 (우선):
   export KOSIS_API_KEY="your_api_key"

2. 명령어로 설정:
   kosis config set-key <YOUR_API_KEY>

3. 여러 개 등록 (병렬 조회용):
   kosis config add-key <API_KEY_2>
   kosis config add-key <API_KEY_3>

API 키는 KOSIS 공식 웹사이트에서 발급받을 수 있습니다.
https://kosis.kr/
`
}

// HasAPIKey는 API 키가 설정되어 있는지 확인합니다.
func HasAPIKey() bool {
	if os.Getenv("KOSIS_API_KEY") != "" {
		return true
	}

	cfg, err := Load()
	if err != nil {
		return false
	}

	return len(cfg.APIKeys) > 0
}

// GetFirstAPIKey는 첫 번째 API 키를 반환합니다.
// 환경변수 KOSIS_API_KEY가 설정되어 있으면 우선적으로 반환됩니다.
func GetFirstAPIKey() (string, error) {
	keys, err := GetAPIKeys()
	if err != nil {
		return "", err
	}

	if len(keys) == 0 {
		return "", errors.New("API 키가 설정되지 않았습니다")
	}

	return keys[0], nil
}

// GetDefaultFormat은 기본 출력 형식을 반환합니다.
func GetDefaultFormat() (string, error) {
	cfg, err := Load()
	if err != nil {
		return "", err
	}

	if cfg.DefaultFormat == "" {
		return "table", nil
	}

	return cfg.DefaultFormat, nil
}

// SetDefaultFormat은 기본 출력 형식을 설정합니다.
func SetDefaultFormat(format string) error {
	validFormats := []string{"table", "json", "csv"}
	valid := false
	for _, f := range validFormats {
		if f == format {
			valid = true
			break
		}
	}

	if !valid {
		return fmt.Errorf("유효하지 않은 형식: %s (지원: table, json, csv)", format)
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	cfg.DefaultFormat = format
	return Save(cfg)
}

// SetCacheTTL은 캐시 TTL을 설정합니다 (단위: 시간).
func SetCacheTTL(hours int) error {
	if hours < 0 {
		return errors.New("캐시 TTL은 음수일 수 없습니다")
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	cfg.CacheTTLHours = hours
	return Save(cfg)
}

// SetConfigDirForTesting은 테스트 실행 시 설정 디렉토리를 오버라이드합니다.
// 테스트에서 실제 ~/.kosis를 오염시키지 않기 위해 사용합니다.
// 테스트 완료 후 빈 문자열을 전달하여 원상복구할 수 있습니다.
func SetConfigDirForTesting(dir string) {
	configDirOverride = dir
}
