package cmd

import (
	"fmt"
	"os"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/interactive"
	"github.com/clazic/kosis/internal/output"
	"github.com/spf13/cobra"
)

var explainCmd = &cobra.Command{
	Use:     "explain <ORG_ID> <TBL_ID>",
	Aliases: []string{"ex"},
	Short:   "통계 조사 설명",
	Long: `KOSIS 통계 조사 설명

통계 조사의 작성목적, 조사대상, 조사방법, 조사기간 등
방법론 정보를 확인합니다.

주의: KOSIS 통계설명 API는 많은 통계표에서 빈 결과를 반환합니다.
      설명이 없는 경우 kosis m <ORG_ID> <TBL_ID>로 메타데이터를 대신 확인하세요.

사용법:
  kosis explain <ORG_ID> <TBL_ID> [flags]
  kosis ex <ORG_ID> <TBL_ID>
  kosis explain                        대화형 모드

파라미터:
  <ORG_ID>                 기관 코드 (없으면 대화형)
  <TBL_ID>                 통계표 ID (없으면 대화형)

플래그:
  -f, --format <type>      출력 형식: table(기본), json

예제:
  # 인구총조사 설명
  kosis ex 101 DT_1IN1502

  # JSON 형식으로 출력
  kosis ex 101 DT_1IN1502 -f json

  # 대화형 모드
  kosis explain

출력 항목:
  작성목적, 작성대상, 작성항목, 작성방법, 조사기간,
  공표시기, 공표범위, 작성기관, 주요연혁

빈 결과 시 대안:
  kosis m <ORG_ID> <TBL_ID>     메타데이터로 분류/항목/수록정보 확인

관련 명령어:
  kosis search <키워드>      통계표 검색
  kosis meta <ORG> <TBL>     분류/항목 코드 확인
  kosis data <ORG> <TBL>     데이터 조회`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runExplain,
}

var explainFormat string

func init() {
	rootCmd.AddCommand(explainCmd)

	explainCmd.Flags().StringVarP(&explainFormat, "format", "f", "table", "출력 형식: table=정렬표, json=파이프/jq용")
}

func runExplain(cmd *cobra.Command, args []string) error {
	var orgID, tblID string
	if len(args) == 1 {
		return fmt.Errorf("인자가 1개만 제공되었습니다. ORG_ID와 TBL_ID를 모두 입력하세요: kosis explain <ORG_ID> <TBL_ID>")
	}
	if len(args) == 2 {
		orgID = args[0]
		tblID = args[1]
	}

	// API 키 가져오기
	keys, err := config.GetAPIKeys()
	if err != nil {
		return fmt.Errorf("설정 로드 실패: %w", err)
	}

	if len(keys) == 0 {
		fmt.Println(config.NoAPIKeyMessage())
		return fmt.Errorf("API 키가 설정되지 않았습니다")
	}

	// API 클라이언트 생성
	client, err := api.NewClient(keys)
	if err != nil {
		return fmt.Errorf("클라이언트 생성 실패: %w", err)
	}

	if len(args) == 0 {
		stat, statErr := os.Stdin.Stat()
		if statErr != nil {
			return fmt.Errorf("stdin 상태 확인 실패: %w", statErr)
		}
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			return fmt.Errorf("인자 없이 실행하려면 대화형 터미널에서 실행하세요: kosis explain <ORG_ID> <TBL_ID>")
		}

		keyword := interactive.Prompt("? 검색어를 입력하세요:")
		if keyword == "" {
			return fmt.Errorf("검색어가 입력되지 않았습니다")
		}

		fmt.Print("  검색 중...\n")
		searchResults, err := client.Search(keyword, api.SearchOptions{ResultCount: 20})
		if err != nil {
			return fmt.Errorf("검색 실패: %w", err)
		}
		if len(searchResults) == 0 {
			return fmt.Errorf("검색 결과가 없습니다")
		}

		var tableOptions []string
		for _, r := range searchResults {
			tableOptions = append(tableOptions, fmt.Sprintf("%s (%s/%s)", r.TblNM, r.OrgID, r.TblID))
		}

		selectedIdx, _ := interactive.Select("? 통계표를 선택하세요:", tableOptions)
		if selectedIdx < 0 {
			return fmt.Errorf("통계표를 선택해야 합니다")
		}

		selected := searchResults[selectedIdx]
		orgID = selected.OrgID
		tblID = selected.TblID
	}

	// 통계설명 조회
	results, err := client.Explain(orgID, tblID)
	if err != nil {
		return fmt.Errorf("통계설명 조회 실패: %w", err)
	}

	if len(results) == 0 || (len(results) == 1 && results[0].StatsNM == "" && results[0].WritingPurps == "") {
		fmt.Printf("해당 통계표(%s/%s)의 설명이 없습니다.\n", orgID, tblID)
		fmt.Println("참고: KOSIS 통계설명 API는 일부 통계표에서만 데이터를 제공합니다.")
		fmt.Printf("대안: kosis m %s %s 로 메타데이터를 확인하세요.\n", orgID, tblID)
		return nil
	}

	// 결과를 맵 형식으로 변환
	data := make([]map[string]interface{}, len(results))
	for i, item := range results {
		data[i] = map[string]interface{}{
			"ORG_ID":        item.OrgID,
			"ORG_NM":        item.OrgNM,
			"TBL_ID":        "",
			"TBL_NM":        "",
			"EXPL_NM":       item.StatsNM,
			"EXPL_CONT":     item.WritingPurps,
			"SURVEY_TYPE":   item.DataCollect,
			"SURVEY_PURP":   item.WritingPurps,
			"SURVEY_METHOD": item.DataCollect,
		}
	}

	// 포맷터 생성
	formatter, err := output.NewFormatter(explainFormat)
	if err != nil {
		return fmt.Errorf("포맷터 생성 실패: %w", err)
	}

	// 포맷팅된 출력
	if err := formatter.Format(data, output.FormatOptions{
		Korean: true,
	}); err != nil {
		return fmt.Errorf("출력 생성 실패: %w", err)
	}

	return nil
}
