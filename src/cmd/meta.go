package cmd

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/interactive"
	"github.com/clazic/kosis/internal/output"
	"github.com/spf13/cobra"
)

var metaCmd = &cobra.Command{
	Use:     "meta <ORG_ID> <TBL_ID>",
	Aliases: []string{"m"},
	Short:   "통계표 메타데이터 조회",
	Long: `KOSIS 통계표 메타데이터 조회

통계표의 분류, 항목, 수록정보 등 메타데이터를 확인합니다.
데이터 조회(kosis data) 전에 반드시 실행하여
사용 가능한 분류/항목 코드를 파악하세요.

사용법:
  kosis meta <ORG_ID> <TBL_ID> [flags]
  kosis m <ORG_ID> <TBL_ID>
  kosis meta                             대화형 모드

파라미터:
  <ORG_ID>                 기관 코드 (예: 101, 116, 301)
  <TBL_ID>                 통계표 ID (예: DT_1IN1502, DT_MLTM_2086)

플래그:
  --find <이름>            이름으로 코드 찾기 (쉼표로 여러 개, 공백 무시) — 지역·산업·항목 모두
  --under <코드>           그 코드 바로 아래 코드들 (예: --under 26 → 울산의 구·군)
  --level <N>              그 단계 코드만 (1=최상위, 예: 산업 대분류)
                           셋은 함께 쓸 수 있고, 결과 끝에 d 에 넣을 "코드+코드" 가 나옵니다
                           -f json: [{flag, axis, code, name, level, parent_code, parent_name, path, unit}]
  --type <type>            메타 유형 (기본: 요약)
                           ITM  = 분류/항목
                           PRD  = 수록정보 (시점 범위, 주기)
                           TBL  = 통계표명
                           ORG  = 기관명
                           CMMT = 주석
                           UNIT = 단위
                           SOURCE = 출처
                           WGT  = 가중치
                           NCD  = 갱신일
  -f, --format <type>      출력 형식: table(기본), json

출력 구조 (요약 모드):
  [분류] → -c1, -c2, ... 에 대응 (순서대로)
    코드         분류값명           ← 코드를 -c1 등에 사용
  [항목] → -i 에 대응
    코드         항목명             ← 코드를 -i에 사용
  [수록정보] → -p 에 대응
    prdSe=코드   수록주기           ← 코드를 -p에 사용

예제:
  # 인구 통계표 메타 요약 (분류/항목/수록정보 한눈에)
  kosis m 101 DT_1IN1502

  # 요약 모드 JSON 출력
  kosis m 101 DT_1IN1502 -f json

  # 수록정보만 확인 (시점 범위 파악)
  kosis m 101 DT_1IN1502 --type PRD

  # 대화형 모드 (검색 → 선택 → 메타 확인)
  kosis meta

메타 결과 읽는 법:
  분류가 여러 개면 순서대로 -c1, -c2, -c3 ...에 대응합니다.
  예) 분류1=시도별, 분류2=산업별 → -c1 26 -c2 C10
  항목 코드(T01, T07 등)는 -i에 그대로 사용합니다.
  prdSe 값은 -p에 그대로 사용합니다: 년→Y, 월→M, 분기→Q, 반기→H, 5년→F

주의:
  - 분류/항목에 한글 이름을 사용하면 에러 21 발생 → 반드시 코드 사용
  - prdSe=5년 인 통계표에 -p Y를 쓰면 에러 30 발생 → -p F 사용

관련 명령어:
  kosis search <키워드>            먼저 검색하여 ORG_ID, TBL_ID 확인
  kosis data <ORG_ID> <TBL_ID>    확인된 코드로 데이터 조회
  kosis explain <ORG_ID> <TBL_ID> 통계 조사 방법론 확인`,
	Args: cobra.MaximumNArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		// 대화형 모드: 인자가 0개일 때만 입력받기
		var orgID, tblID string

		if len(args) == 0 {
			// 검색어 입력
			keyword := interactive.Prompt("? 검색어를 입력하세요:")
			if keyword == "" {
				fmt.Fprintln(os.Stderr, "검색어가 입력되지 않았습니다.")
				return
			}

			// 설정 로드
			cfg, err := config.Load()
			if err != nil {
				fmt.Fprintf(os.Stderr, "오류: %v\n", err)
				return
			}

			// API 클라이언트 생성
			client, err := api.NewClient(cfg.APIKeys)
			if err != nil {
				fmt.Fprintf(os.Stderr, "오류: %v\n", err)
				return
			}

			// 검색 실행
			fmt.Fprint(os.Stderr, "  📋 검색 중...\n")
			searchOpts := api.SearchOptions{ResultCount: 20}
			searchResults, err := client.Search(keyword, searchOpts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "오류: %v\n", err)
				return
			}

			if len(searchResults) == 0 {
				fmt.Fprintln(os.Stderr, "검색 결과가 없습니다.")
				return
			}

			// 통계표 선택 옵션 구성
			var tableOptions []string
			for _, r := range searchResults {
				tableOptions = append(tableOptions, fmt.Sprintf("%s (%s/%s)", r.TblNM, r.OrgID, r.TblID))
			}

			// 통계표 선택
			selectedIdx, _ := interactive.Select("? 통계표를 선택하세요:", tableOptions)
			if selectedIdx < 0 {
				fmt.Fprintln(os.Stderr, "통계표를 선택해야 합니다.")
				return
			}

			selectedTable := searchResults[selectedIdx]
			orgID = selectedTable.OrgID
			tblID = selectedTable.TblID
		} else if len(args) == 1 {
			fmt.Fprintln(os.Stderr, "오류: ORG_ID와 TBL_ID를 모두 입력하세요. (예: kosis meta 101 DT_1IN1502)")
			fmt.Fprintln(os.Stderr)
			cmd.Help()
			return
		} else {
			orgID = args[0]
			tblID = args[1]
		}
		metaType, _ := cmd.Flags().GetString("type")
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

		// If --type is empty, show summary
		if metaType == "" {
			summary, err := client.MetaSummary(orgID, tblID)
			if err != nil {
				fmt.Fprintf(os.Stderr, "메타데이터 조회 실패: %v\n", err)
				return
			}

			findF, _ := cmd.Flags().GetString("find")
			underF, _ := cmd.Flags().GetString("under")
			levelF, _ := cmd.Flags().GetInt("level")
			if findF != "" || underF != "" || levelF > 0 {
				printCodeRows(summary.Codes(), findF, underF, levelF, format)
				return
			}

			if format == "json" {
				data := []map[string]interface{}{
					{
						"ORG_ID":          orgID,
						"TBL_ID":          tblID,
						"CLASSIFICATIONS": summary.Classifications,
						"ITEMS":           summary.Items,
						"PERIODS":         summary.Periods,
					},
				}
				formatter, err := output.NewFormatter(format)
				if err != nil {
					fmt.Fprintf(os.Stderr, "포맷터 생성 실패: %v\n", err)
					return
				}
				if err := formatter.Format(data, output.FormatOptions{}); err != nil {
					fmt.Fprintf(os.Stderr, "출력 실패: %v\n", err)
				}
				return
			}

			// Display summary in a formatted way
			displayMetaSummary(summary)
			return
		}

		// Retrieve specific meta type — 유형마다 필드가 달라 API 필드를 그대로 출력한다
		results, err := client.MetaRaw(orgID, tblID, metaType)
		if err != nil && strings.Contains(err.Error(), "[30]") { // 그 유형의 메타 없음
			results, err = nil, nil
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "메타데이터 조회 실패: %v\n", err)
			return
		}

		if len(results) == 0 {
			fmt.Fprintf(os.Stderr, "'%s' 타입의 메타데이터가 없습니다.\n", metaType)
			if format == "json" {
				fmt.Println("[]")
			}
			return
		}

		var columns []string
		seenCol := map[string]bool{}
		for _, r := range results {
			for k := range r {
				if !seenCol[k] {
					seenCol[k] = true
					columns = append(columns, k)
				}
			}
		}
		sort.Strings(columns)

		formatter, err := output.NewFormatter(format)
		if err != nil {
			fmt.Fprintf(os.Stderr, "포맷터 생성 실패: %v\n", err)
			return
		}
		data := make([]map[string]interface{}, len(results))
		for i, r := range results {
			data[i] = r
		}
		if err := formatter.Format(data, output.FormatOptions{Columns: columns}); err != nil {
			fmt.Fprintf(os.Stderr, "출력 실패: %v\n", err)
			return
		}

		fmt.Fprintf(os.Stderr, "\n결과: %d개\n", len(results))
	},
}

func init() {
	rootCmd.AddCommand(metaCmd)

	metaCmd.Flags().String("type", "", "조회할 메타 유형. 미지정=요약(분류+항목+수록정보). ITM=분류/항목, PRD=수록정보(시점범위·주기), TBL=통계표명, ORG=기관명, CMMT=주석, UNIT=단위, SOURCE=출처, WGT=가중치, NCD=갱신일")
	metaCmd.Flags().StringP("format", "f", "table", "출력 형식: table=정렬표, json=파이프/jq용")
	metaCmd.Flags().String("find", "", "이름으로 코드 찾기 (쉼표로 여러 개, 공백 무시. 예: \"울산,울주\")")
	metaCmd.Flags().String("under", "", "이 코드 바로 아래 코드들 (예: --under 26 → 울산의 구·군)")
	metaCmd.Flags().Int("level", 0, "이 단계의 코드만 (1=최상위. 예: 산업 대분류)")
}

// displayMetaSummary displays metadata summary in a formatted way
func displayMetaSummary(summary *api.MetaSummaryResult) {
	fmt.Println("\n=== 메타데이터 요약 ===")
	fmt.Println()

	// Display classifications
	if len(summary.Classifications) > 0 {
		fmt.Printf("[분류] objL 파라미터에 사용 (%d개 분류)\n", len(summary.Classifications))
		currentObj := ""
		for _, c := range summary.Classifications {
			if c.ObjID != currentObj {
				currentObj = c.ObjID
				fmt.Printf("\n  objL → 분류ID: %s  분류명: %s\n", c.ObjID, c.ObjNM)
			}
			code := c.ItmID
			name := c.ItmNM
			if code == "" {
				code = c.Code
			}
			if name == "" {
				name = c.Label
			}
			fmt.Printf("    %-16s %s\n", code, name)
		}
		fmt.Println()
	}

	// Display items
	if len(summary.Items) > 0 {
		fmt.Printf("[항목] itmId 파라미터에 사용 (%d개 항목)\n", len(summary.Items))
		for _, item := range summary.Items {
			code := item.ItmID
			name := item.ItmNM
			if code == "" {
				code = item.Code
			}
			if name == "" {
				name = item.Label
			}
			fmt.Printf("    %-16s %s\n", code, name)
		}
		fmt.Println()
	}

	// Display periods
	if len(summary.Periods) > 0 {
		fmt.Printf("[수록정보] prdSe 파라미터에 사용\n")
		for _, p := range summary.Periods {
			code := p.PrdSe
			name := p.PrdDe
			if code == "" {
				code = p.Code
			}
			if name == "" {
				name = p.Label
			}
			fmt.Printf("    prdSe=%s  %s\n", code, name)
		}
		fmt.Println()
	}
}

// printCodeRows --find·--under·--level 로 거른 코드 목록과, 플래그별로 d 에 바로 넣을 "a+b+c".
func printCodeRows(rows []api.CodeRow, find, under string, level int, format string) {
	var names []string
	for _, n := range strings.Split(find, ",") {
		if n = strings.Join(strings.Fields(n), ""); n != "" {
			names = append(names, n)
		}
	}
	var out []api.CodeRow
	for _, r := range rows {
		if under != "" && r.ParentCode != under {
			continue
		}
		if level > 0 && r.Level != level {
			continue
		}
		if len(names) > 0 {
			hit := false
			for _, n := range names {
				if strings.Contains(strings.ReplaceAll(r.Name, " ", ""), n) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		out = append(out, r)
	}
	if format == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if out == nil {
			out = []api.CodeRow{}
		}
		_ = enc.Encode(out)
		return
	}
	if len(out) == 0 {
		fmt.Fprintln(os.Stderr, "맞는 코드가 없습니다. kosis m <ORG> <TBL> 로 전체를 보세요.")
		return
	}
	data := make([]map[string]interface{}, len(out))
	for i, r := range out {
		data[i] = map[string]interface{}{"플래그": r.Flag, "분류": r.Axis, "코드": r.Code, "이름": r.Name,
			"단계": r.Level, "상위": strings.TrimSpace(r.ParentCode + " " + r.ParentName), "경로": r.Path}
	}
	formatter, err := output.NewFormatter("table")
	if err != nil {
		fmt.Fprintf(os.Stderr, "포맷터 생성 실패: %v\n", err)
		return
	}
	_ = formatter.Format(data, output.FormatOptions{Columns: []string{"플래그", "분류", "코드", "이름", "단계", "상위", "경로"}})
	var flags []string
	codes := map[string][]string{}
	for _, r := range out {
		if _, ok := codes[r.Flag]; !ok {
			flags = append(flags, r.Flag)
		}
		codes[r.Flag] = append(codes[r.Flag], r.Code)
	}
	fmt.Println("\nkosis d 에 넣기:")
	for _, f := range flags {
		fmt.Printf("  %s \"%s\"\n", f, strings.Join(codes[f], "+"))
	}
}
