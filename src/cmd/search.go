package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/clazic/kosis/internal/api"
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
  kosis s <키워드>              (여러 단어는 따옴표 없이도 한 검색어: kosis s 1인당 지역내총생산)
  kosis search                         대화형 모드

파라미터:
  <키워드>                 검색할 통계 키워드 (필수, 없으면 대화형)

플래그:
  -n, --limit <N>          결과 수 (기본: 20, 1~1000)
  --page <N>               쪽 번호 (기본 1). 다음 결과는 --page 2 …
  --sort <RANK|DATE>       정렬: RANK=정확도(기본), DATE=최신
  -f, --format <type>      출력 형식: table(기본), json, csv, md
  --org <코드|이름>        표를 만든 기관 (101, 울산, 국가데이터처 — 이름은 일부만).
                           KOSIS 서버에서 거른다. 이름은 기관별 목록(kosis ls --view MT_OTITLE)에서
                           코드로 바꾼다 (맞는 기관이 여럿이면 각각 검색, 5개 넘으면 오류)
  --stat <이름>            통계조사명으로 거르기 (예: 광업제조업조사) — 받은 쪽 안에서만
  --region <지역>          표 이름·기관·조사명·분류 경로에 지역 이름이 든 것만 — 받은 쪽 안에서만
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
  - 검색은 표 이름뿐 아니라 표 내용(분류·항목 이름)과 주석에도 맞춘다 — 이름에 검색어가 없는 표도 나온다
  - 검색어에 연도(2023 등)가 있고 결과가 없을 때([30])만 연도를 빼고 한 번 더 검색합니다
  - 숫자 코드(11501 등)로는 검색할 수 없습니다 → kosis ls 사용
  - stderr '검색 범위' = 전체 결과 수, 받은 수(이 쪽), --stat/--region 으로 거른 뒤 수.
    --stat/--region 은 받은 쪽 안에서만 거르므로 빈 결과면 --page 로 다음 쪽도 보세요
  - 검색에서 못 찾은 것이 KOSIS 에 없다는 뜻은 아닙니다
  - 인자·형식이 틀리거나 조회가 실패하면 stderr 에 오류, 종료 코드 1 (stdout 비움)

출력 컬럼:
  ORG_ID       기관 코드 (meta/data에 사용)
  TBL_ID       통계표 ID (meta/data에 사용)
  TBL_NM       통계표 이름
  ORG_NM       기관명
  STAT_NM      통계조사명 (같은 조사의 표끼리 묶어 보기)
  STRT_PRD_DE  수록 시작 시점
  END_PRD_DE   수록 종료 시점
  (json/csv/md 는 STAT_ID, PATH(분류 경로)도 포함, --items 면 ITEMS.
   --items 메타 조회에 실패한 표가 있으면 ITEMS_ERROR 열이 추가되고 stderr 에 표 목록)
  안내문(검색 결과 수 등)은 stderr 로 나가므로 stdout 은 데이터만 담깁니다.

다음 단계:
  검색 결과에서 ORG_ID와 TBL_ID를 확인한 후:
  kosis meta <ORG_ID> <TBL_ID>     메타데이터 확인 (필수!)
  kosis data <ORG_ID> <TBL_ID>     데이터 조회`,
	Args: cobra.ArbitraryArgs, // 따옴표 없이 여러 단어를 줘도 한 검색어로 (AI 가 자주 빠뜨림)
	RunE: runSearch,
}

func runSearch(cmd *cobra.Command, args []string) error {
	limit, _ := cmd.Flags().GetInt("limit")
	page, _ := cmd.Flags().GetInt("page")
	sortF, _ := cmd.Flags().GetString("sort")
	format, _ := cmd.Flags().GetString("format")
	orgF, _ := cmd.Flags().GetString("org")
	statF, _ := cmd.Flags().GetString("stat")
	regionF, _ := cmd.Flags().GetString("region")
	withItems, _ := cmd.Flags().GetBool("items")

	// 인자 검사는 API 를 부르기 전에 — 실패는 stderr·종료 코드 1, stdout 은 비운다
	if limit < 1 || limit > 1000 {
		return fmt.Errorf("-n 은 1~1000 이어야 합니다 (받은 값: %d)", limit)
	}
	if page < 1 {
		return fmt.Errorf("--page 는 1 이상이어야 합니다 (받은 값: %d)", page)
	}
	sortF = strings.ToUpper(sortF)
	if sortF != "RANK" && sortF != "DATE" {
		return fmt.Errorf("--sort 는 RANK(정확도) 또는 DATE(최신) 입니다 (받은 값: %s)", sortF)
	}
	if err := checkFormat(format, "table", "json", "csv", "md", "markdown"); err != nil {
		return err
	}
	cmd.SilenceUsage = true // 여기부터는 사용법이 아니라 실행 오류

	var keyword string
	if len(args) == 0 {
		keyword = interactive.Prompt("? 검색어를 입력하세요:")
		if keyword == "" {
			return fmt.Errorf("검색어가 입력되지 않았습니다")
		}
	} else {
		keyword = strings.Join(args, " ")
	}

	client, err := newCmdClient()
	if err != nil {
		return err
	}

	// 기관: 숫자는 그 코드, 이름은 기관별 목록(MT_OTITLE)에서 찾은 코드로 서버에서 거른다
	orgIDs, orgNote, err := resolveOrgs(client, orgF)
	if err != nil {
		return err
	}
	if orgNote != "" {
		fmt.Fprintln(os.Stderr, orgNote)
	}
	// 조사명·지역은 API 가 거르지 못해 넉넉히 받아 거른다 (한 페이지, 최대 1000개)
	clientFilter := statF != "" || regionF != ""
	fetch := limit
	if clientFilter {
		fetch = min(max(limit*10, 300), 1000)
	}

	var perOrg [][]api.SearchResult
	total := 0
	for _, org := range orgIDs {
		rs, err := client.Search(keyword, api.SearchOptions{ResultCount: fetch, StartCount: page, Sort: sortF, OrgID: org})
		if err != nil && strings.Contains(err.Error(), "[30]") { // 결과 없음
			rs, err = nil, nil
		}
		if err != nil {
			return fmt.Errorf("검색 실패: %w", err)
		}
		if len(rs) > 0 {
			n, _ := strconv.Atoi(rs[0].StatDBCnt)
			total += n
		}
		perOrg = append(perOrg, rs)
	}
	results := interleave(perOrg) // 기관이 여럿이면 번갈아 — -n 으로 자를 때 첫 기관이 다 차지하지 않게
	fetched := len(results)
	if clientFilter {
		results = filterSearch(results, "", statF, regionF)
	}
	filtered := len(results)
	if len(results) > limit {
		results = results[:limit]
	}

	// 범위 진단 (stderr): 전체 결과 수 ≠ 받은 수 ≠ 거른 뒤 수
	scope := fmt.Sprintf("검색 범위: 전체 %d개 중 %d쪽에서 %d개 받음", total, page, fetched)
	if clientFilter {
		scope += fmt.Sprintf(" → --stat/--region 으로 거른 뒤 %d개", filtered)
	}
	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "'%s' 검색 결과가 없습니다. %s\n", keyword, scope)
		printSearchNextSteps(keyword, total, fetched, page, clientFilter, orgF, regionF)
		if format == "json" {
			fmt.Println("[]")
		}
		history.AddArgs(historyArgs(args, keyword), 0)
		return nil
	}

	var items, itemErrs []string
	if withItems {
		items, itemErrs = searchItems(func(org, tbl string) ([]api.MetaResult, error) {
			return client.Meta(org, tbl, api.MetaOptions{Type: "ITM", ObjID: "ITEM"})
		}, results)
	}
	failed := 0
	for _, e := range itemErrs {
		if e != "" {
			failed++
		}
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
			if failed > 0 {
				data[i]["ITEMS_ERROR"] = itemErrs[i]
			}
		}
	}

	formatter, err := output.NewFormatter(format)
	if err != nil {
		return err
	}
	columns := []string{"ORG_ID", "ORG_NM", "TBL_ID", "TBL_NM", "STAT_NM", "STRT_PRD_DE", "END_PRD_DE"}
	if format != "table" {
		columns = []string{"ORG_ID", "ORG_NM", "TBL_ID", "TBL_NM", "STAT_ID", "STAT_NM", "PATH", "STRT_PRD_DE", "END_PRD_DE"}
	}
	if withItems {
		columns = append(columns, "ITEMS")
		if failed > 0 {
			columns = append(columns, "ITEMS_ERROR")
		}
	}
	if err := formatter.Format(data, output.FormatOptions{Columns: columns}); err != nil {
		return fmt.Errorf("출력 실패: %w", err)
	}

	fmt.Fprintf(os.Stderr, "\n검색 결과: %d개 (%s)\n", len(results), scope)
	if total > page*fetch && !clientFilter && len(results) == limit {
		fmt.Fprintf(os.Stderr, "  더 보기: --page %d\n", page+1)
	}
	if failed > 0 {
		var ids []string
		for i, e := range itemErrs {
			if e != "" {
				ids = append(ids, results[i].OrgID+"/"+results[i].TblID)
			}
		}
		fmt.Fprintf(os.Stderr, "⚠ --items: %d개 표는 메타 조회에 실패해 항목이 비어 있습니다 (부분 결과, ITEMS_ERROR 열): %s\n", failed, strings.Join(ids, ", "))
	}
	history.AddArgs(historyArgs(args, keyword), len(results))
	return nil
}

// historyArgs 실행 인자 그대로 (대화형으로 검색어를 받았으면 그 검색어를 붙인다).
func historyArgs(args []string, keyword string) []string {
	argv := append([]string{}, os.Args[1:]...)
	if len(args) == 0 {
		argv = append(argv, keyword)
	}
	return argv
}

// resolveOrgs --org 값을 서버 필터용 기관 코드로. 숫자면 그대로, 이름이면 기관별 목록에서 이름 일부로 찾는다.
// 반환 [""] 은 기관 필터 없음.
func resolveOrgs(client *api.Client, org string) ([]string, string, error) {
	if org == "" {
		return []string{""}, "", nil
	}
	if isDigits(org) {
		return []string{org}, "", nil
	}
	list, err := client.List(api.ListOptions{VwCd: "MT_OTITLE"})
	if err != nil {
		return nil, "", fmt.Errorf("기관 이름을 코드로 바꾸지 못했습니다 (기관별 목록 조회 실패: %v) — 기관 코드로 넣어 보세요: kosis ls --view MT_OTITLE", err)
	}
	var ids, names []string
	for _, l := range list {
		if strings.Contains(squash(l.ListNM), squash(org)) {
			ids = append(ids, l.ListID)
			names = append(names, l.ListID+" "+l.ListNM)
		}
	}
	switch {
	case len(ids) == 0:
		return nil, "", fmt.Errorf("기관 '%s' 을(를) 기관별 목록에서 찾지 못했습니다 — kosis ls --view MT_OTITLE 로 기관 이름·코드를 확인하세요", org)
	case len(ids) > maxOrgMatches:
		return nil, "", fmt.Errorf("기관 '%s' 에 맞는 기관이 %d개라 너무 많습니다 — 더 정확한 이름이나 코드로: %s", org, len(ids), strings.Join(names, ", "))
	}
	return ids, "기관 '" + org + "' → " + strings.Join(names, ", "), nil
}

// maxOrgMatches 기관 이름 하나가 가리킬 수 있는 기관 수 (기관마다 검색을 한 번씩 부른다).
const maxOrgMatches = 5

func isDigits(s string) bool {
	_, err := strconv.Atoi(s)
	return err == nil && !strings.HasPrefix(s, "-")
}

// printSearchNextSteps 빈 결과일 때 다음에 해 볼 것 (stderr).
func printSearchNextSteps(keyword string, total, fetched, page int, clientFilter bool, org, region string) {
	if clientFilter && total > page*fetched {
		fmt.Fprintf(os.Stderr, "  · --stat/--region 은 받은 %d개 안에서만 거릅니다. 다음 쪽도 보세요: --page %d\n", fetched, page+1)
	}
	if region != "" {
		fmt.Fprintln(os.Stderr, "  · --region 은 표 이름·기관·조사명·경로 글자만 봅니다. 전국 시도별·시군구별 표 안의 지역은 빠지니 --region 없이 찾은 뒤 kosis m <ORG> <TBL> --find <지역> 으로 고르세요")
	}
	if org != "" {
		fmt.Fprintln(os.Stderr, "  · --org 는 표를 만든 기관입니다. 다른 기관(예 국가데이터처)의 전국 표에도 그 지역 값이 있을 수 있습니다")
	}
	fmt.Fprintln(os.Stderr, "  · 검색에서 못 찾은 것이 KOSIS 에 없다는 뜻은 아닙니다 — 다른 말(조사명·항목명)로, 또는 kosis ls 로 찾아보세요")
}

func init() {
	rootCmd.AddCommand(searchCmd)

	searchCmd.Flags().IntP("limit", "n", 20, "가져올 검색 결과 개수 (1~1000). 결과가 너무 많으면 키워드를 더 구체적으로")
	searchCmd.Flags().Int("page", 1, "결과 쪽 번호 (1부터, -n 개씩). 전체 수는 stderr '검색 범위' 에")
	searchCmd.Flags().String("sort", "RANK", "정렬: RANK=정확도, DATE=최신")
	searchCmd.Flags().StringP("format", "f", "table", "출력 형식: table=정렬표, json=파이프/jq용, csv=쉼표구분, md=Markdown 표")
	searchCmd.Flags().String("org", "", "표를 만든 기관 코드(101) 또는 기관명 일부(울산, 국가데이터처) — KOSIS 서버에서 거른다")
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

// searchItems 결과마다 항목명(단위) 목록과 실패 사유. 메타 조회는 동시에 4개까지.
// 실패한 표는 항목이 "" 이고 errs 에 사유가 든다 (항목이 정말 없는 표와 구분).
func searchItems(fetch func(org, tbl string) ([]api.MetaResult, error), rs []api.SearchResult) (out, errs []string) {
	out = make([]string, len(rs))
	errs = make([]string, len(rs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4)
	for i, r := range rs {
		wg.Add(1)
		go func(i int, r api.SearchResult) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			meta, err := fetch(r.OrgID, r.TblID)
			if err != nil {
				errs[i] = err.Error()
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
	return out, errs
}

// interleave 기관별 결과를 한 개씩 번갈아 합친다 (기관 하나면 그대로).
func interleave(groups [][]api.SearchResult) []api.SearchResult {
	if len(groups) == 1 {
		return groups[0]
	}
	var out []api.SearchResult
	for i := 0; ; i++ {
		added := false
		for _, g := range groups {
			if i < len(g) {
				out = append(out, g[i])
				added = true
			}
		}
		if !added {
			return out
		}
	}
}
