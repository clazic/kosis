package cmd

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/cache"
	"github.com/clazic/kosis/internal/config"
	"github.com/spf13/cobra"
	"golang.org/x/term"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "설정 관리",
	Long: `설정 관리

API 키, 출력 형식, 캐시 등의 설정을 관리합니다.
인자 없이 실행하면 현재 설정을 표시합니다.
설정은 ~/.kosis/config.yaml에 저장됩니다.

사용법:
  kosis config [subcommand]

하위 명령어:
  set-key <KEY>            API 키 설정 (단일, 기존 키 교체)
  add-key <KEY>            API 키 추가 (기존 키 유지, 병렬 조회용)
  remove-key <INDEX>       API 키 제거 (인덱스)
  key-list                 등록된 API 키 목록
  show                     현재 설정 표시


  cache-clear              캐시 전체 삭제
  cache-size               캐시 크기 확인
  cache-clean              만료된 캐시 정리

예제:
  # API 키 설정
  kosis config set-key "your_api_key"

  # API 키 추가 (병렬 조회 속도 향상)
  kosis config add-key "api_key_2"

  # 현재 설정 확인
  kosis config show


  # 캐시 정리
  kosis config cache-clean

설정 파일 경로:
  macOS/Linux: ~/.kosis/config.yaml
  Windows:     %USERPROFILE%\.kosis\config.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		// 현재 설정 표시
		fmt.Println("=== KOSIS 설정 ===")
		fmt.Printf("API 키 개수: %d개\n", len(cfg.APIKeys))
		if len(cfg.APIKeys) > 0 {
			for i, key := range cfg.APIKeys {
				shortKey := maskKey(key)
				fmt.Printf("  [%d] %s\n", i, shortKey)
			}
		}
		fmt.Printf("기본 출력 형식: %s\n", cfg.DefaultFormat)
		fmt.Printf("캐시 TTL: %d시간\n", cfg.CacheTTLHours)
	},
}

var setKeyCmd = &cobra.Command{
	Use:   "set-key <API_KEY>",
	Short: "API 키 설정 (단일)",
	Long: `API 키를 설정합니다. 기존 키는 모두 제거됩니다.

사용법:
  kosis config set-key <API_KEY>

예제:
  kosis config set-key "your_api_key"

주의:
  기존에 등록된 모든 키가 제거되고 새 키 하나만 남습니다.
  키를 추가하려면 kosis config add-key를 사용하세요.

다음 단계:
  kosis config add-key <KEY>    병렬 조회용 키 추가
  kosis config show              설정 확인`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		if err := config.SetDefaultKey(key); err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}
		fmt.Println("✓ API 키가 설정되었습니다.")
	},
}

var addKeyCmd = &cobra.Command{
	Use:   "add-key <API_KEY>",
	Short: "API 키 추가",
	Long: `새로운 API 키를 추가합니다. 기존 키는 유지됩니다.

여러 키를 등록하면 대용량 조회 시 병렬로 API를 호출하여
속도가 향상됩니다.

사용법:
  kosis config add-key <API_KEY>

예제:
  kosis config add-key "api_key_2"
  kosis config add-key "api_key_3"

다음 단계:
  kosis config key-list     등록된 키 목록 확인
  kosis config show          전체 설정 확인`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		key := args[0]
		if err := config.AddAPIKey(key); err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}
		fmt.Println("✓ API 키가 추가되었습니다.")
	},
}

var removeKeyCmd = &cobra.Command{
	Use:   "remove-key <INDEX>",
	Short: "API 키 제거",
	Long: `지정된 인덱스의 API 키를 제거합니다.

사용법:
  kosis config remove-key <INDEX>

파라미터:
  <INDEX>   제거할 키의 번호. key-list 출력의 맨 앞 번호를 그대로 사용합니다.

예시:
  kosis config key-list    # 먼저 인덱스 확인
  kosis config remove-key 1

주의:
  제거 후 나머지 키의 인덱스가 다시 매겨집니다.
  여러 개를 지울 때는 매번 key-list로 인덱스를 다시 확인하세요.

다음 단계:
  kosis config key-list     남은 키 확인`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		index, err := strconv.Atoi(args[0])
		if err != nil {
			fmt.Printf("오류: 인덱스는 숫자여야 합니다 (%v)\n", err)
			return
		}

		if err := config.RemoveAPIKey(index); err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}
		fmt.Println("✓ API 키가 제거되었습니다.")
	},
}

var keyListCmd = &cobra.Command{
	Use:   "key-list",
	Short: "등록된 API 키 목록",
	Long: `등록된 API 키 목록을 표시합니다.

보안을 위해 키 전체가 아니라 앞뒤 일부만 마스킹하여 보여줍니다.
맨 앞 번호(인덱스)는 remove-key에 그대로 사용합니다.

사용법:
  kosis config key-list

예시:
  kosis config key-list

관련 명령어:
  kosis config add-key <KEY>       키 추가 (병렬 조회 속도 향상)
  kosis config remove-key <INDEX>  키 제거`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		if len(cfg.APIKeys) == 0 {
			fmt.Println("등록된 API 키가 없습니다.")
			fmt.Println()
			fmt.Println(config.NoAPIKeyMessage())
			return
		}

		fmt.Println("=== 등록된 API 키 ===")
		for i, key := range cfg.APIKeys {
			shortKey := maskKey(key)
			fmt.Printf("[%d] %s\n", i, shortKey)
		}
	},
}

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "전체 설정 표시",
	Long: `현재 설정을 YAML 형식으로 표시합니다.

표시 항목: API 키, 기본 출력 형식, 캐시 TTL

사용법:
  kosis config show

예제:
  kosis config show`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		fmt.Println("=== KOSIS 설정 (YAML) ===")
		fmt.Printf("api_keys:\n")
		for i, key := range cfg.APIKeys {
			shortKey := maskKey(key)
			fmt.Printf("  - \"%s\"  # [%d]\n", shortKey, i)
		}
		fmt.Printf("default_format: %s\n", cfg.DefaultFormat)
		fmt.Printf("cache_ttl_hours: %d\n", cfg.CacheTTLHours)

	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "cache-clear",
	Short: "캐시 전체 삭제",
	Long: `만료 여부와 관계없이 저장된 모든 API 응답 캐시를 삭제합니다.

캐시는 ~/.kosis/cache 에 저장되며, 같은 조회를 반복할 때 API 호출 없이
즉시 응답하는 데 사용됩니다. KOSIS 원본 데이터가 갱신되었는데도 예전 값이
계속 나온다면 이 명령으로 비우세요.

사용법:
  kosis config cache-clear

예시:
  kosis config cache-clear

관련 명령어:
  kosis config cache-clean   만료된 항목만 정리 (평소 권장)
  kosis config cache-size    현재 캐시 용량 확인`,
	Run: func(cmd *cobra.Command, args []string) {
		cacheDir := filepath.Join(config.ConfigDir(), "cache")
		c, err := cache.New(cacheDir, 24)
		if err != nil {
			fmt.Printf("오류: 캐시 디렉토리 접근 실패: %v\n", err)
			return
		}

		if err := c.Clear(); err != nil {
			fmt.Printf("오류: 캐시 삭제 실패: %v\n", err)
			return
		}

		fmt.Println("✓ 모든 캐시가 삭제되었습니다.")
	},
}

var cacheSizeCmd = &cobra.Command{
	Use:   "cache-size",
	Short: "캐시 크기 확인",
	Long: `캐시 디렉토리(~/.kosis/cache)가 차지하는 디스크 크기를 사람이 읽기 좋은
단위(KB/MB/GB)로 표시합니다.

사용법:
  kosis config cache-size

예시:
  kosis config cache-size

관련 명령어:
  kosis config cache-clean   만료 항목 정리
  kosis config cache-clear   전체 삭제`,
	Run: func(cmd *cobra.Command, args []string) {
		cacheDir := filepath.Join(config.ConfigDir(), "cache")
		c, err := cache.New(cacheDir, 24)
		if err != nil {
			fmt.Printf("오류: 캐시 디렉토리 접근 실패: %v\n", err)
			return
		}

		size, err := c.Size()
		if err != nil {
			fmt.Printf("오류: 캐시 크기 조회 실패: %v\n", err)
			return
		}

		// 바이트를 읽기 좋은 형식으로 변환
		sizeStr := formatBytes(size)
		fmt.Printf("캐시 크기: %s\n", sizeStr)
	},
}

var cacheCleanCmd = &cobra.Command{
	Use:   "cache-clean",
	Short: "만료된 캐시 정리",
	Long: `TTL이 지난 캐시 항목만 골라 삭제합니다. 유효한 캐시는 그대로 두므로
평소 정리에는 cache-clear 대신 이 명령을 사용하세요.

TTL은 설정의 cache_ttl_hours 값을 따릅니다 (kosis config show로 확인).

사용법:
  kosis config cache-clean

예시:
  kosis config cache-clean

관련 명령어:
  kosis config cache-size    정리 전후 용량 비교
  kosis config cache-clear   전체 삭제`,
	Run: func(cmd *cobra.Command, args []string) {
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("오류: 설정 로드 실패: %v\n", err)
			return
		}

		cacheDir := filepath.Join(config.ConfigDir(), "cache")
		c, err := cache.New(cacheDir, cfg.CacheTTLHours)
		if err != nil {
			fmt.Printf("오류: 캐시 디렉토리 접근 실패: %v\n", err)
			return
		}

		expiredCount, err := c.GetExpiredCount()
		if err != nil {
			fmt.Printf("오류: 만료된 캐시 확인 실패: %v\n", err)
			return
		}

		if expiredCount == 0 {
			fmt.Println("만료된 캐시가 없습니다.")
			return
		}

		if err := c.CleanExpired(); err != nil {
			fmt.Printf("오류: 캐시 정리 실패: %v\n", err)
			return
		}

		fmt.Printf("✓ %d개의 만료된 캐시가 삭제되었습니다.\n", expiredCount)
	},
}

// formatBytes는 바이트 크기를 읽기 좋은 형식으로 변환합니다.
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes < KB:
		return fmt.Sprintf("%d B", bytes)
	case bytes < MB:
		return fmt.Sprintf("%.2f KB", float64(bytes)/KB)
	case bytes < GB:
		return fmt.Sprintf("%.2f MB", float64(bytes)/MB)
	default:
		return fmt.Sprintf("%.2f GB", float64(bytes)/GB)
	}
}

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "대화형 초기 설정 마법사",
	Long: `처음 설치했을 때 실행하는 대화형 초기 설정 마법사입니다.

진행 단계:
  1) KOSIS API 키 입력 프롬프트 표시
  2) 입력한 키로 실제 API를 한 번 호출하여 유효성 검증
  3) 검증에 성공하면 ~/.kosis/config.yaml 에 저장

키 발급: https://kosis.kr/openapi/ (회원가입 후 즉시 발급)

사용법:
  kosis config setup

예시:
  kosis config setup

비대화형 환경(스크립트/CI)에서는:
  kosis config set-key "<API_KEY>"

다음 단계:
  kosis s "인구"            통계표 검색으로 동작 확인
  kosis config add-key ...  키를 더 추가해 병렬 조회 속도 향상`,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("KOSIS API 키 설정")
		fmt.Println("─────────────────────────────────────")
		fmt.Println("키 발급: https://kosis.kr/openapi/")
		fmt.Println()

		reader := bufio.NewReader(os.Stdin)

		for {
			var key string
			if term.IsTerminal(int(os.Stdin.Fd())) {
				fmt.Print("API Key 입력: ")
				b, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Println()
				if err != nil {
					fmt.Fprintf(os.Stderr, "입력 오류: %v\n", err)
					return
				}
				key = strings.TrimSpace(string(b))
			} else {
				fmt.Print("API Key 입력: ")
				line, _ := reader.ReadString('\n')
				key = strings.TrimSpace(line)
			}

			if key == "" {
				fmt.Println("키를 입력해주세요.")
				continue
			}

			fmt.Print("키 검증 중... ")
			client, err := api.NewClient([]string{key})
			if err == nil {
				_, err = client.Search("인구", api.SearchOptions{ResultCount: 1})
			}
			if err != nil {
				fmt.Println("✗ 검증 실패")
				fmt.Printf("오류: %v\n", err)
				fmt.Println("키를 다시 확인하거나 https://kosis.kr/openapi/ 에서 재발급하세요.")
				fmt.Println()
				continue
			}

			if err := config.SetDefaultKey(key); err != nil {
				fmt.Fprintf(os.Stderr, "저장 실패: %v\n", err)
				return
			}

			fmt.Println("✓ 검증 성공. ~/.kosis/config.yaml 저장 완료.")
			fmt.Println()
			fmt.Println("이제 다음 명령으로 시작하세요:")
			fmt.Println("  kosis s \"인구\"          # 통계표 검색")
			fmt.Println("  kosis m 101 DT_1IN1502  # 메타 확인 (분류·항목 코드)")
			return
		}
	},
}

func init() {
	rootCmd.AddCommand(configCmd)

	configCmd.AddCommand(setupCmd)
	configCmd.AddCommand(setKeyCmd)
	configCmd.AddCommand(addKeyCmd)
	configCmd.AddCommand(removeKeyCmd)
	configCmd.AddCommand(keyListCmd)
	configCmd.AddCommand(showCmd)
	configCmd.AddCommand(cacheClearCmd)
	configCmd.AddCommand(cacheSizeCmd)
	configCmd.AddCommand(cacheCleanCmd)
}

// maskKey 키 표시용: 뒤 4자만 (구분용). 8자 이하이면 전부 가린다 — AI 대화·로그에 키 조각이 남지 않게.
func maskKey(key string) string {
	if len(key) <= 8 {
		return "****"
	}
	return "…" + key[len(key)-4:]
}
