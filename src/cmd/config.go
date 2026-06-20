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

API 키, AI 도구, 출력 형식 등의 설정을 관리합니다.
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

  set-ai <도구>            기본 AI 도구 설정
  ai-add <이름> <명령어>   커스텀 AI 도구 추가
  ai-remove <이름>         AI 도구 제거
  ai-list                  등록된 AI 도구 목록

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

  # AI 도구 설정
  kosis config set-ai claude
  kosis config ai-add ollama "ollama run llama3 '{prompt}'"

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
				// 키의 일부만 표시 (보안)
				shortKey := key
				if len(key) > 10 {
					shortKey = key[:4] + "..." + key[len(key)-4:]
				}
				fmt.Printf("  [%d] %s\n", i, shortKey)
			}
		}
		fmt.Printf("기본 출력 형식: %s\n", cfg.DefaultFormat)
		fmt.Printf("캐시 TTL: %d시간\n", cfg.CacheTTLHours)
		fmt.Printf("기본 AI 도구: %s\n", cfg.AI.Default)
		fmt.Printf("등록된 AI 도구: %d개\n", len(cfg.AI.Tools))
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

예시:
  kosis config key-list    # 먼저 인덱스 확인
  kosis config remove-key 1`,
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

예시:
  kosis config key-list`,
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
			// 키의 일부만 표시 (보안)
			shortKey := key
			if len(key) > 15 {
				shortKey = key[:4] + "..." + key[len(key)-4:]
			}
			fmt.Printf("[%d] %s\n", i, shortKey)
		}
	},
}

var showCmd = &cobra.Command{
	Use:   "show",
	Short: "전체 설정 표시",
	Long: `현재 설정을 YAML 형식으로 표시합니다.

표시 항목: API 키, 기본 출력 형식, 캐시 TTL, AI 도구 목록

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
			shortKey := key
			if len(key) > 15 {
				shortKey = key[:4] + "..." + key[len(key)-4:]
			}
			fmt.Printf("  - \"%s\"  # [%d]\n", shortKey, i)
		}
		fmt.Printf("default_format: %s\n", cfg.DefaultFormat)
		fmt.Printf("cache_ttl_hours: %d\n", cfg.CacheTTLHours)
		fmt.Printf("ai:\n")
		fmt.Printf("  default: %s\n", cfg.AI.Default)
		fmt.Printf("  tools:\n")
		for name, tool := range cfg.AI.Tools {
			fmt.Printf("    %s:\n", name)
			fmt.Printf("      cmd: \"%s\"\n", tool.Cmd)
		}
	},
}

var setAICmd = &cobra.Command{
	Use:   "set-ai <TOOL_NAME>",
	Short: "기본 AI 도구 설정",
	Long: `기본 AI 도구를 설정합니다.

예시:
  kosis config set-ai claude
  kosis config set-ai gemini`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		toolName := args[0]
		if err := config.SetAIDefault(toolName); err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}
		fmt.Printf("✓ 기본 AI 도구가 '%s'로 설정되었습니다.\n", toolName)
	},
}

var aiListCmd = &cobra.Command{
	Use:   "ai-list",
	Short: "AI 도구 목록",
	Long: `등록된 AI 도구 목록을 표시합니다.

예시:
  kosis config ai-list`,
	Run: func(cmd *cobra.Command, args []string) {
		tools, err := config.ListAITools()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		fmt.Println("=== 등록된 AI 도구 ===")
		for _, tool := range tools {
			status := "설치됨"
			if !tool.Installed {
				status = "미설치"
			}

			marker := " "
			if tool.Name == cfg.AI.Default {
				marker = "*"
			}

			fmt.Printf("%s %s [%s]\n", marker, tool.Name, status)
			fmt.Printf("  명령어: %s\n", tool.Cmd)
		}
		fmt.Printf("\n* = 현재 기본 AI 도구\n")
	},
}

var aiAddCmd = &cobra.Command{
	Use:   "ai-add <NAME> <COMMAND>",
	Short: "커스텀 AI 도구 추가",
	Long: `커스텀 AI 도구를 추가합니다.

명령어는 '{prompt}'를 포함해야 합니다.

예시:
  kosis config ai-add ollama "ollama run llama3 '{prompt}'"
  kosis config ai-add local "python ai.py '{prompt}'"`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		command := args[1]

		if !strings.Contains(command, "{prompt}") {
			fmt.Println("오류: 명령어는 '{prompt}'를 포함해야 합니다.")
			return
		}

		if err := config.AddAITool(name, command); err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}
		fmt.Printf("✓ AI 도구 '%s'이(가) 추가되었습니다.\n", name)
	},
}

var aiRemoveCmd = &cobra.Command{
	Use:   "ai-remove <NAME>",
	Short: "AI 도구 제거",
	Long: `등록된 AI 도구를 제거합니다.

예시:
  kosis config ai-list     # 먼저 도구명 확인
  kosis config ai-remove ollama`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		name := args[0]
		if err := config.RemoveAITool(name); err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}
		fmt.Printf("✓ AI 도구 '%s'이(가) 제거되었습니다.\n", name)
	},
}

var cacheClearCmd = &cobra.Command{
	Use:   "cache-clear",
	Short: "캐시 전체 삭제",
	Long: `저장된 모든 캐시를 삭제합니다.

예시:
  kosis config cache-clear`,
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
	Long: `현재 캐시가 차지하는 디스크 크기를 표시합니다.

예시:
  kosis config cache-size`,
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
	Long: `만료된 캐시 항목을 삭제합니다.

예시:
  kosis config cache-clean`,
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
	Long: `KOSIS API 키를 대화형으로 입력하고 검증합니다.

키 발급: https://kosis.kr/openapi/

사용법:
  kosis config setup`,
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
			fmt.Println("  kosis q \"GDP 최근 5년\"  # 자연어 조회")
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
	configCmd.AddCommand(setAICmd)
	configCmd.AddCommand(aiListCmd)
	configCmd.AddCommand(aiAddCmd)
	configCmd.AddCommand(aiRemoveCmd)
	configCmd.AddCommand(cacheClearCmd)
	configCmd.AddCommand(cacheSizeCmd)
	configCmd.AddCommand(cacheCleanCmd)
}
