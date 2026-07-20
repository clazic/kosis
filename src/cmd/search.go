package cmd

import (
	"fmt"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/interactive"
	"github.com/clazic/kosis/internal/output"
	"github.com/clazic/kosis/internal/history"
	"github.com/spf13/cobra"
)

var searchCmd = &cobra.Command{
	Use:     "search <키워드>",
	Aliases: []string{"s"},
	Short:   "통계표 키워드 검색",
	Long: `KOSIS 통계표 검색

키워드로 KOSIS 통계표를 검색합니다.
검색 결과에서 ORG_ID와 TBL_ID를 확인하여 meta, data 명령어에 사용합니다.

사용법:
  kosis search <키워드> [flags]
  kosis s <키워드>
  kosis search                         대화형 모드

파라미터:
  <키워드>                 검색할 통계 키워드 (필수, 없으면 대화형)

플래그:
  -n, --limit <N>          결과 수 (기본: 20, 최대: 1000)
  -f, --format <type>      출력 형식: table(기본), json, csv, md

예제:
  # 인구 관련 통계표 검색
  kosis s "인구"

  # 미분양 검색 (결과 50개)
  kosis s "미분양" -n 50

  # JSON 형식으로 출력 (파이프용)
  kosis s "GDP" -f json

  # Markdown 테이블 출력
  kosis s "소비자물가" -f md

  # 대화형 모드
  kosis search

주의:
  - 검색어에 연도(2023 등)를 포함하면 자동 제거 후 재검색됩니다
  - 숫자 코드(11501 등)로는 검색할 수 없습니다 → kosis ls 사용
  - 검색은 통계표 이름(키워드)만 지원합니다

출력 컬럼:
  ORG_ID       기관 코드 (meta/data에 사용)
  TBL_ID       통계표 ID (meta/data에 사용)
  TBL_NM       통계표 이름
  ORG_NM       기관명
  STRT_PRD_DE  수록 시작 시점
  END_PRD_DE   수록 종료 시점

다음 단계:
  검색 결과에서 ORG_ID와 TBL_ID를 확인한 후:
  kosis meta <ORG_ID> <TBL_ID>     메타데이터 확인 (필수!)
  kosis data <ORG_ID> <TBL_ID>     데이터 조회`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		// 대화형 모드: 인자가 없으면 입력받기
		var keyword string
		if len(args) == 0 {
			keyword = interactive.Prompt("? 검색어를 입력하세요:")
			if keyword == "" {
				fmt.Println("검색어가 입력되지 않았습니다.")
				return
			}
		} else {
			keyword = args[0]
		}
		limit, _ := cmd.Flags().GetInt("limit")
		format, _ := cmd.Flags().GetString("format")

		// Get API keys from config
		keys, err := config.GetAPIKeys()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		if len(keys) == 0 {
			fmt.Printf("오류: API 키가 설정되지 않았습니다.\n")
			fmt.Println()
			fmt.Println(config.NoAPIKeyMessage())
			return
		}

		// Create API client
		client, err := api.NewClient(keys)
		if err != nil {
			fmt.Printf("클라이언트 생성 실패: %v\n", err)
			return
		}

		// Execute search
		results, err := client.Search(keyword, api.SearchOptions{
			ResultCount: limit,
		})
		if err != nil {
			fmt.Printf("검색 실패: %v\n", err)
			return
		}

		if len(results) == 0 {
			fmt.Printf("'%s' 검색 결과가 없습니다.\n", keyword)
			return
		}

		// Convert results to []map[string]interface{}
		data := make([]map[string]interface{}, len(results))
		for i, result := range results {
			data[i] = map[string]interface{}{
				"ORG_ID":      result.OrgID,
				"ORG_NM":      result.OrgNM,
				"TBL_ID":      result.TblID,
				"TBL_NM":      result.TblNM,
				"STRT_PRD_DE": result.StrtPrdDe,
				"END_PRD_DE":  result.EndPrdDe,
			}
		}

		// Format and output
		formatter, err := output.NewFormatter(format)
		if err != nil {
			fmt.Printf("포맷터 생성 실패: %v\n", err)
			return
		}

		opts := output.FormatOptions{
			Columns: []string{"ORG_ID", "ORG_NM", "TBL_ID", "TBL_NM", "STRT_PRD_DE", "END_PRD_DE"},
		}

		if err := formatter.Format(data, opts); err != nil {
			fmt.Printf("출력 실패: %v\n", err)
			return
		}

		fmt.Printf("\n검색 결과: %d개\n", len(results))

		// Record history
		fullCmd := "search " + keyword
		if cmd.Flags().Changed("limit") {
			fullCmd += fmt.Sprintf(" -n %d", limit)
		}
		if cmd.Flags().Changed("format") {
			fullCmd += fmt.Sprintf(" -f %s", format)
		}
		history.Add(fullCmd, len(results))
	},
}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().IntP("limit", "n", 20, "가져올 검색 결과 개수 (최대 1000). 결과가 너무 많으면 키워드를 더 구체적으로")
	searchCmd.Flags().StringP("format", "f", "table", "출력 형식: table=정렬표, json=파이프/jq용, csv=쉼표구분, md=Markdown 표")
}
