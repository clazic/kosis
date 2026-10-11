package cmd

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/history"
	"github.com/clazic/kosis/internal/interactive"
	"github.com/clazic/kosis/internal/output"
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
  --org <코드|이름>        기관으로 거르기 (101, 울산, 국가데이터처 — 이름은 일부만)
  --stat <이름>            통계조사명으로 거르기 (예: 광업제조업조사)
  --region <지역>          표 이름·기관·조사명·분류 경로에 지역 이름이 든 것만
                           (전국 "시도별" 표는 빠집니다 — 그런 표에서 지역 고르기는 m --find)
  --items                  결과마다 항목명(단위) 표시 — 표마다 메타 조회 1번이라 -n 을 작게

예제:
  # 인구 관련 통계표 검색
  kosis s "인구"

  # 미분양 검색 (결과 50개)
  kosis s "미분양" -n 50

  # JSON 형식으로 출력 (파이프용)
  kosis s "GDP" -f json

  # Markdown 테이블 출력
  kosis s "소비자물가" -f md

  # 울산 기관이 만든 표만, 항목·단위까지
  kosis s "생산액" --org 울산 --items -n 10

  # 특정 조사의 표만
  kosis s "출하액" --stat 광업제조업조사

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
  STAT_NM      통계조사명 (같은 조사의 표끼리 묶어 보기)
  STRT_PRD_DE  수록 시작 시점
  END_PRD_DE   수록 종료 시점
  (json/csv/md 는 STAT_ID, PATH(분류 경로)도 포함, --items 면 ITEMS)
  안내문(검색 결과 수 등)은 stderr 로 나가므로 stdout 은 데이터만 담깁니다.

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
				fmt.Fprintln(os.Stderr, "검색어가 입력되지 않았습니다.")
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
			fmt.Fprintf(os.Stderr, "오류: %v\n", err)
			return
		}

		if len(keys) == 0 {
			fmt.Fprintf(os.Stderr, "오류: API 키가 설정되지 않았습니다.\n")
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, config.NoAPIKeyMessage())
			return
		}

		// Create API client
		client, err := api.NewClient(keys)
		if err != nil {
			fmt.Fprintf(os.Stderr, "클라이언트 생성 실패: %v\n", err)
			return
		}

		orgF, _ := cmd.Flags().GetString("org")
		statF, _ := cmd.Flags().GetString("stat")
		regionF, _ := cmd.Flags().GetString("region")
		withItems, _ := cmd.Flags().GetBool("items")
		filtering := orgF != "" || statF != "" || regionF != ""

		// 거르기는 API 가 지원하지 않아 결과를 넉넉히 받아 거른다
		fetch := limit
		if filtering {
			fetch = min(max(limit*10, 300), 1000)
		}
		results, err := client.Search(keyword, api.SearchOptions{ResultCount: fetch})
		if err != nil && strings.Contains(err.Error(), "[30]") { // 결과 없음
			results, err = nil, nil
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "검색 실패: %v\n", err)
			os.Exit(1)
		}
		if filtering {
			results = filterSearch(results, orgF, statF, regionF)
			if len(results) > limit {
				results = results[:limit]
			}
		}

		if len(results) == 0 {
			fmt.Fprintf(os.Stderr, "'%s' 검색 결과가 없습니다.\n", keyword)
			if format == "json" {
				fmt.Println("[]")
			}
			return
		}

		var items []string
		if withItems {
			items = searchItems(client, results)
		}

		data := make([]map[string]interface{}, len(results))
		for i, result := range results {
			data[i] = map[string]interface{}{
				"ORG_ID":      result.OrgID,
				"ORG_NM":      result.OrgNM,
				"TBL_ID":      result.TblID,
				"TBL_NM":      result.TblNM,
				"STAT_ID":     result.StatID,
				"STAT_NM":     result.StatNM,
				"PATH":        result.MTAtitle,
				"STRT_PRD_DE": result.StrtPrdDe,
				"END_PRD_DE":  result.EndPrdDe,
			}
			if withItems {
				data[i]["ITEMS"] = items[i]
			}
		}

		// Format and output
		formatter, err := output.NewFormatter(format)
		if err != nil {
			fmt.Fprintf(os.Stderr, "포맷터 생성 실패: %v\n", err)
			return
		}

		columns := []string{"ORG_ID", "ORG_NM", "TBL_ID", "TBL_NM", "STAT_NM", "STRT_PRD_DE", "END_PRD_DE"}
		if format != "table" {
			columns = []string{"ORG_ID", "ORG_NM", "TBL_ID", "TBL_NM", "STAT_ID", "STAT_NM", "PATH", "STRT_PRD_DE", "END_PRD_DE"}
		}
		if withItems {
			columns = append(columns, "ITEMS")
		}
		if err := formatter.Format(data, output.FormatOptions{Columns: columns}); err != nil {
			fmt.Fprintf(os.Stderr, "출력 실패: %v\n", err)
			return
		}

		fmt.Fprintf(os.Stderr, "\n검색 결과: %d개\n", len(results))

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
	searchCmd.Flags().String("org", "", "기관 코드(101) 또는 기관명 일부(울산, 국가데이터처)로 거르기")
	searchCmd.Flags().String("stat", "", "통계조사명 일부로 거르기 (예: 광업제조업조사)")
	searchCmd.Flags().String("region", "", "표 이름·기관·조사명·분류 경로에 지역 이름이 든 것만 (전국 시도별 표는 빠짐)")
	searchCmd.Flags().Bool("items", false, "결과마다 항목명·단위 표시 (표마다 메타 조회 1번 — -n 을 작게)")
}

// filterSearch 기관(코드 또는 이름 일부)·조사명·지역 이름으로 거른다 (공백 무시).
func filterSearch(rs []api.SearchResult, org, stat, region string) []api.SearchResult {
	has := func(s, sub string) bool { return strings.Contains(squash(s), squash(sub)) }
	var out []api.SearchResult
	for _, r := range rs {
		if org != "" && r.OrgID != org && !has(r.OrgNM, org) {
			continue
		}
		if stat != "" && !has(r.StatNM, stat) {
			continue
		}
		if region != "" && !has(r.TblNM+" "+r.OrgNM+" "+r.StatNM+" "+r.MTAtitle, region) {
			continue
		}
		out = append(out, r)
	}
	return out
}

func squash(s string) string { return strings.Join(strings.Fields(s), "") }

// searchItems 결과마다 항목명(단위) 목록. 메타 조회는 동시에 4개까지, 실패한 표는 빈 값.
func searchItems(client *api.Client, rs []api.SearchResult) []string {
	out := make([]string, len(rs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, r := range rs {
		wg.Add(1)
		go func(i int, r api.SearchResult) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			meta, err := client.Meta(r.OrgID, r.TblID, api.MetaOptions{Type: "ITM"})
			if err != nil {
				return
			}
			var names []string
			for _, m := range meta {
				if m.ObjID != "ITEM" {
					continue
				}
				n := m.ItmNM
				if m.UnitNM != "" {
					n += "(" + m.UnitNM + ")"
				}
				names = append(names, n)
			}
			out[i] = strings.Join(names, ", ")
		}(i, r)
	}
	wg.Wait()
	return out
}
