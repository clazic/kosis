package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
)

// NewAPIClient는 API 키를 로드하고 클라이언트를 생성한 후 캐시를 초기화합니다.
// 검색·메타·목록·설명 응답은 캐시(TTL: config cache_ttl_hours)를 쓰고, 데이터 조회(d)는 쓰지 않는다.
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

// newCmdClient 명령용 클라이언트(캐시 포함). 키가 없으면 발급 안내까지 담은 오류를 돌려준다.
func newCmdClient() (*api.Client, error) {
	if keys, _ := config.GetAPIKeys(); len(keys) == 0 {
		return nil, fmt.Errorf("API 키가 설정되지 않았습니다\n\n%s", config.NoAPIKeyMessage())
	}
	return NewAPIClient()
}

// checkFormat 이 명령이 stdout 에 쓸 수 있는 형식인지 API 를 부르기 전에 확인한다.
func checkFormat(format string, allowed ...string) error {
	for _, a := range allowed {
		if format == a {
			return nil
		}
	}
	return fmt.Errorf("알 수 없는 출력 형식: %s (가능: %s)", format, strings.Join(allowed, ", "))
}
