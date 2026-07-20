package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
	"github.com/spf13/cobra"
)

var bulkCmd = &cobra.Command{
	Use:   "bulk <userStatsId>",
	Short: "대용량 통계자료 다운로드",
	Long: `KOSIS 대용량 통계자료 다운로드

4만 셀 초과 대용량 데이터를 SDMX 또는 XLS 형식으로 다운로드합니다.
KOSIS 웹에서 사전 등록된 userStatsId가 필요합니다.

사용법:
  kosis bulk <userStatsId> [flags]

파라미터:
  <userStatsId>            사전 등록된 통계표 ID
                           (https://kosis.kr/openapi/ 에서 등록)

플래그:
  --type <DSD>             SDMX 유형 (기본: DSD)
  -o, --output <파일>      출력 파일 (.xls/.sdmx)

예제:
  # SDMX 형식으로 다운로드
  kosis bulk "myid/101/DT_1IN1502/2/1/20191106094026_1" --type DSD -o 통계.sdmx

  # XLS 형식으로 다운로드
  kosis bulk "myid/101/DT_1IN1502/..." -o 통계.xls

용량 제한:
  4만 셀 이하    kosis data 명령어로 직접 조회 (자동 분할 지원)
  4만~20만 셀    bulk 다운로드 필요 (XLS만 가능)
  20만 셀 초과   불가 (범위 축소 필요)

주의:
  - userStatsId는 KOSIS 웹(https://kosis.kr/openapi/)에서 사전 등록해야 합니다
  - -o/--output 플래그는 필수입니다

관련 명령어:
  kosis data ...             파라미터 직접 지정 조회 (4만 셀 이하 또는 자동 분할)`,

	Example: `  # SDMX 형식으로 다운로드
  kosis bulk "myid/101/DT_1IN1502/2/1/20191106094026_1" --type DSD -o 통계.sdmx

  # XLS 형식 (기본)
  kosis bulk "myid/101/DT_1IN1502/..." -o 통계.xls`,

	Run: func(cmd *cobra.Command, args []string) {
		// 인자 확인
		if len(args) == 0 {
			fmt.Println("오류: userStatsId는 필수입니다")
			fmt.Println()
			cmd.Help()
			return
		}

		userStatsID := args[0]

		// --output 플래그 확인 (필수)
		if bulkOutputFlag == "" {
			fmt.Println("오류: -o/--output 플래그는 필수입니다")
			fmt.Println()
			cmd.Help()
			return
		}

		// 설정 로드
		cfg, err := config.Load()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		// API 클라이언트 생성
		client, err := api.NewClient(cfg.APIKeys)
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		// BigDataOptions 생성
		opts := api.BigDataOptions{
			Type: bulkTypeFlag,
		}

		// 기본값 설정 (type이 비어있으면 DSD 사용)
		if opts.Type == "" {
			opts.Type = "DSD"
		}

		fmt.Printf("대용량 통계자료 다운로드 중: %s\n", userStatsID)
		fmt.Printf("형식: %s\n", opts.Type)

		// BigData 호출
		data, err := client.BigData(userStatsID, opts)
		if err != nil {
			fmt.Printf("조회 오류: %v\n", err)
			return
		}

		// 경로 검증: 절대 경로로 변환 후 현재 작업 디렉토리를 벗어나는지 확인
		absOutput, err := filepath.Abs(bulkOutputFlag)
		if err != nil {
			fmt.Printf("경로 변환 오류: %v\n", err)
			return
		}
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("작업 디렉토리 확인 오류: %v\n", err)
			return
		}
		// ".." 컴포넌트가 포함되어 있거나 작업 디렉토리를 벗어나면 거부
		if strings.Contains(bulkOutputFlag, "..") || !strings.HasPrefix(absOutput, cwd+string(filepath.Separator)) && absOutput != cwd {
			fmt.Printf("오류: 출력 경로가 현재 작업 디렉토리를 벗어납니다: %s\n", bulkOutputFlag)
			return
		}

		// 출력 디렉토리 확인
		outputDir := filepath.Dir(absOutput)
		if outputDir != "." && outputDir != "" {
			if err := os.MkdirAll(outputDir, 0755); err != nil {
				fmt.Printf("디렉토리 생성 오류: %v\n", err)
				return
			}
		}

		// 파일에 저장
		if err := os.WriteFile(absOutput, data, 0644); err != nil {
			fmt.Printf("파일 저장 오류: %v\n", err)
			return
		}

		// 파일 크기 계산
		fileInfo, _ := os.Stat(bulkOutputFlag)
		sizeKB := fileInfo.Size() / 1024
		sizeMB := sizeKB / 1024

		// 성공 메시지
		fmt.Printf("✓ 데이터가 %s로 저장되었습니다.\n", bulkOutputFlag)
		if sizeMB > 0 {
			fmt.Printf("  파일 크기: %d MB\n", sizeMB)
		} else if sizeKB > 0 {
			fmt.Printf("  파일 크기: %d KB\n", sizeKB)
		} else {
			fmt.Printf("  파일 크기: %d bytes\n", fileInfo.Size())
		}
	},
}

var (
	bulkTypeFlag   string
	bulkOutputFlag string
)

func init() {
	rootCmd.AddCommand(bulkCmd)

	// 플래그 정의
	bulkCmd.Flags().StringVar(&bulkTypeFlag, "type", "DSD", "SDMX 유형. DSD=구조정의(Data Structure Definition). .xls 저장 시에는 무시됨")
	bulkCmd.Flags().StringVarP(&bulkOutputFlag, "output", "o", "", "저장할 파일 경로 (필수). 확장자로 형식 결정: .sdmx=SDMX, .xls=엑셀(4만~20만 셀)")
}
