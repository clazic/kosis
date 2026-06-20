package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
)

// NewAPIClient는 API 키를 로드하고 클라이언트를 생성한 후 캐시를 초기화합니다.
// 모든 CLI 명령어에서 사용되는 헬퍼 함수입니다.
func NewAPIClient() (*api.Client, error) {
	// Get API keys from config
	keys, err := config.GetAPIKeys()
	if err != nil {
		return nil, fmt.Errorf("API 키 로드 실패: %w", err)
	}

	if len(keys) == 0 {
		return nil, fmt.Errorf("API 키가 설정되지 않았습니다")
	}

	// Create API client
	client, err := api.NewClient(keys)
	if err != nil {
		return nil, fmt.Errorf("클라이언트 생성 실패: %w", err)
	}

	// 캐시 초기화
	cfg, err := config.Load()
	if err != nil {
		return nil, fmt.Errorf("설정 로드 실패: %w", err)
	}

	cacheDir := filepath.Join(config.ConfigDir(), "cache")
	if err := client.InitCache(cacheDir, cfg.CacheTTLHours); err != nil {
		// 캐시 초기화 실패는 경고만 하고 계속 진행
		fmt.Fprintf(os.Stderr, "캐시 초기화 경고: %v\n", err)
	}

	return client, nil
}
