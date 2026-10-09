// Package config는 KOSIS CLI의 설정 관리를 담당합니다.
// 설정 파일은 ~/.kosis/config.yaml에 저장됩니다.
// 환경변수 KOSIS_API_KEY는 설정 파일의 API 키보다 우선됩니다.
package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// configDirOverride는 테스트용으로 설정 디렉토리를 오버라이드하는 변수입니다.
// 테스트에서만 사용하며, 실제 ~/.kosis 디렉토리를 오염시키지 않기 위해 존재합니다.
var configDirOverride string

// Config는 KOSIS CLI의 전체 설정을 나타냅니다.
type Config struct {
	APIKeys       []string `mapstructure:"api_keys"`
	DefaultFormat string   `mapstructure:"default_format"`
	CacheTTLHours int      `mapstructure:"cache_ttl_hours"`
	AI            AIConfig `mapstructure:"ai"`
	UpdateCheck   bool     `mapstructure:"update_check"`
}

// AIConfig는 quick --ai가 호출할 AI API 공급자 설정입니다.
type AIConfig struct {
	Default   string                `mapstructure:"default" yaml:"default"`
	Providers map[string]AIProvider `mapstructure:"providers" yaml:"providers"`
}

// AIProvider는 OpenAI 호환 Chat Completions API 하나를 나타냅니다.
// 키는 KeyEnv 환경변수 > APIKey(설정 파일) 순으로 찾습니다.
type AIProvider struct {
	BaseURL string `mapstructure:"base_url" yaml:"base_url"` // 예: http://host:8642/v1
	Model   string `mapstructure:"model" yaml:"model"`
	APIKey  string `mapstructure:"api_key" yaml:"api_key,omitempty"`
	KeyEnv  string `mapstructure:"key_env" yaml:"key_env,omitempty"`
}

// ResolveKey는 사용할 API 키를 반환합니다. 없으면 빈 문자열(인증 없는 로컬 서버용).
func (p AIProvider) ResolveKey() string {
	if p.KeyEnv != "" {
		if v := os.Getenv(p.KeyEnv); v != "" {
			return v
		}
	}
	return p.APIKey
}

// DefaultAIProvider 기본 공급자 이름. o(oc) 머신의 Hermes Agent API 서버를 Tailscale로 호출한다.
const DefaultAIProvider = "hermes"

// builtinProviders 설정 파일에 없으면 자동으로 채워지는 공급자.
// ponytail: 개인 Tailscale 주소가 기본값이다. 다른 환경에선 ai-add로 덮어쓰거나 set-ai로 바꿀 것.
func builtinProviders() map[string]AIProvider {
	return map[string]AIProvider{
		DefaultAIProvider: {
			BaseURL: "http://100.70.244.98:8642/v1",
			Model:   "hermes-agent",
			KeyEnv:  "HERMES_API_KEY",
		},
	}
}

// DefaultConfig는 기본 설정을 반환합니다.
func DefaultConfig() *Config {
	return &Config{
		APIKeys:       []string{},
		DefaultFormat: "table",
		CacheTTLHours: 24,
		UpdateCheck:   true,
		AI: AIConfig{
			Default:   DefaultAIProvider,
			Providers: builtinProviders(),
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

	normalizeAI(&cfg.AI)

	// 환경변수 KOSIS_API_KEY가 있으면 최우선으로 설정
	if envKey := os.Getenv("KOSIS_API_KEY"); envKey != "" {
		cfg.APIKeys = []string{envKey}
	}

	return cfg, nil
}

// normalizeAI 내장 공급자를 채우고, 기본값이 등록되지 않은 이름이면 내장 기본값으로 되돌린다.
// 예전 CLI 방식 설정(ai.tools, default: claude 등)은 여기서 자연스럽게 hermes로 이관된다.
func normalizeAI(ai *AIConfig) {
	if ai.Providers == nil {
		ai.Providers = map[string]AIProvider{}
	}
	for name, p := range builtinProviders() {
		if _, ok := ai.Providers[name]; !ok {
			ai.Providers[name] = p
		}
	}
	if _, ok := ai.Providers[ai.Default]; !ok {
		ai.Default = DefaultAIProvider
	}
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

// GetAIProvider는 이름으로 공급자를 찾습니다. name이 비면 기본 공급자를 씁니다.
func GetAIProvider(name string) (string, AIProvider, error) {
	cfg, err := Load()
	if err != nil {
		return "", AIProvider{}, err
	}
	if name == "" {
		name = cfg.AI.Default
	}
	p, ok := cfg.AI.Providers[name]
	if !ok {
		return name, AIProvider{}, fmt.Errorf("등록되지 않은 AI 공급자: %s", name)
	}
	return name, p, nil
}

// SetAIDefault는 기본 AI 공급자를 설정합니다.
func SetAIDefault(name string) error {
	if name == "" {
		return errors.New("AI 공급자명은 비워둘 수 없습니다")
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	if _, exists := cfg.AI.Providers[name]; !exists {
		return fmt.Errorf("등록되지 않은 AI 공급자: %s", name)
	}

	cfg.AI.Default = name
	return Save(cfg)
}

// AddAIProvider는 AI 공급자를 추가하거나 같은 이름을 덮어씁니다.
func AddAIProvider(name string, p AIProvider) error {
	if name == "" {
		return errors.New("AI 공급자명은 비워둘 수 없습니다")
	}
	u, err := url.Parse(p.BaseURL)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("base URL은 http(s)://호스트[:포트]/v1 형식이어야 합니다: %q", p.BaseURL)
	}
	if p.Model == "" {
		return errors.New("모델명은 비워둘 수 없습니다")
	}

	cfg, err := Load()
	if err != nil {
		return err
	}
	cfg.AI.Providers[name] = p
	return Save(cfg)
}

// SetAIProviderKey는 공급자의 API 키를 설정 파일에 저장합니다.
func SetAIProviderKey(name, key string) error {
	if key == "" {
		return errors.New("API 키는 비워둘 수 없습니다")
	}
	cfg, err := Load()
	if err != nil {
		return err
	}
	p, ok := cfg.AI.Providers[name]
	if !ok {
		return fmt.Errorf("등록되지 않은 AI 공급자: %s", name)
	}
	p.APIKey = key
	cfg.AI.Providers[name] = p
	return Save(cfg)
}

// RemoveAIProvider는 AI 공급자를 제거합니다. 내장 공급자는 제거할 수 없습니다.
func RemoveAIProvider(name string) error {
	if _, builtin := builtinProviders()[name]; builtin {
		return fmt.Errorf("내장 공급자 '%s'는 제거할 수 없습니다 (ai-add로 주소를 덮어쓸 수 있음)", name)
	}

	cfg, err := Load()
	if err != nil {
		return err
	}

	if _, exists := cfg.AI.Providers[name]; !exists {
		return fmt.Errorf("등록되지 않은 AI 공급자: %s", name)
	}

	delete(cfg.AI.Providers, name)
	normalizeAI(&cfg.AI) // 기본 공급자를 지웠다면 내장 기본값으로 복귀
	return Save(cfg)
}

// MaskKey 키의 앞뒤 4자만 보여준다.
func MaskKey(key string) string {
	if len(key) <= 10 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + "..." + key[len(key)-4:]
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
