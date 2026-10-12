package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/clazic/kosis/internal/api"
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
  --axis <축>              그 축의 코드만: item(항목), class1…class8, 또는 분류 이름 일부(예 산업)
                           넷은 함께 쓸 수 있고, 결과 끝에 d 에 넣을 "코드+코드" 가 나옵니다
                           -f json: [{flag, axis, code, name, level, parent_code, parent_name, path, unit}]
                           분류·항목(ITM)만 1번 조회합니다. --type 없이 또는 --type ITM 과 함께만 쓸 수 있고
                           다른 --type 과 함께 쓰면 오류(종료 코드 1). flag 의 축 번호는 메타 OBJ_ID_SN 기준
  --obj <OBJ_ID>           --type ITM 원본에서 이 분류(예 A) 또는 ITEM(항목)만 받기 (KOSIS objId)
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
  [분류] → 축 머리의 -c1, -c2 … 를 그대로 d 에 쓴다
    코드         분류값명           ← 코드를 -c1 등에 사용
  [항목] → -i 에 대응
    코드         항목명             ← 코드를 -i에 사용
  [수록정보] → -p 에 대응
    -p M (월)  2011.01 ~ 2026.09    ← -p 에 넣을 코드와 수록 시점 범위
  코드가 50개 넘는 축은 위 단계부터 50개 안에서만 보여 주고 끝에 안내(… N개 중 M개만 표시)가 붙는다.
  나머지는 --under <코드>, --find <이름>, --axis <축> 으로 본다. -f json 은 전부 나온다

예제:
  # 인구 통계표 메타 요약 (분류/항목/수록정보 한눈에)
  kosis m 101 DT_1IN1502

  # 요약 모드 JSON 출력
  kosis m 101 DT_1IN1502 -f json

  # 수록정보만 확인 (시점 범위 파악)
  kosis m 101 DT_1IN1502 --type PRD

  # 항목만 (코드·이름·단위) — 큰 표도 짧게
  kosis m 101 DT_1FS1101 --axis item

  # 이름으로 항목 코드 찾기
  kosis m 101 DT_1FS1101 --find 출하액 -f json

  # 대화형 모드 (검색 → 선택 → 메타 확인)
  kosis meta

메타 결과 읽는 법:
  분류가 여러 개면 순서대로 -c1, -c2, -c3 ...에 대응합니다.
  예) 분류1=시도별, 분류2=산업별 → -c1 26 -c2 C10
  항목 코드(T01, T07 등)는 -i에 그대로 사용합니다.
  prdSe 값은 -p에 그대로 사용합니다: 년→Y, 월→M, 분기→Q, 반기→H, 2년·3년·5년 등 여러 해→F

주의:
  - 분류/항목에 한글 이름을 사용하면 에러 21 발생 → 반드시 코드 사용
  - prdSe=2년·3년·5년 등 여러 해 주기 통계표에 -p Y를 쓰면 에러 30 발생 → -p F 사용
  - 인자·형식·플래그 조합이 틀리거나 조회가 실패하면 stderr 에 오류, 종료 코드 1 (stdout 비움)
  - 검색·메타 응답은 캐시에 둡니다 (config cache_ttl_hours, 기본 24시간). 새로 받으려면 kosis config cache-clear

관련 명령어:
  kosis search <키워드>            먼저 검색하여 ORG_ID, TBL_ID 확인
  kosis data <ORG_ID> <TBL_ID>    확인된 코드로 데이터 조회
  kosis explain <ORG_ID> <TBL_ID> 통계 조사 방법론 확인`,
	Args: cobra.MaximumNArgs(2),
	RunE: runMeta,
}

func runMeta(cmd *cobra.Command, args []string) error {
	metaType, _ := cmd.Flags().GetString("type")
	format, _ := cmd.Flags().GetString("format")
	findF, _ := cmd.Flags().GetString("find")
	underF, _ := cmd.Flags().GetString("under")
	levelF, _ := cmd.Flags().GetInt("level")
	axisF, _ := cmd.Flags().GetString("axis")
	objF, _ := cmd.Flags().GetString("obj")
	metaType = strings.ToUpper(metaType)
	selecting := findF != "" || underF != "" || levelF > 0 || axisF != ""

	// 인자 검사는 API 를 부르기 전에 (실패는 stderr·종료 코드 1)
	if len(args) == 1 {
		return fmt.Errorf("ORG_ID와 TBL_ID를 모두 입력하세요 (예: kosis meta 101 DT_1IN1502)")
	}
	if err := checkFormat(format, "table", "json"); err != nil {
		return err
	}
	if levelF < 0 {
		return fmt.Errorf("--level 은 1 이상입니다 (받은 값: %d)", levelF)
	}
	if selecting && metaType != "" && metaType != "ITM" {
		return fmt.Errorf("--find/--under/--level/--axis 는 분류·항목 코드에만 씁니다 — --type %s 와 함께 쓸 수 없습니다 (--type 을 빼거나 ITM)", metaType)
	}
	if objF != "" && (metaType != "ITM" || selecting) {
		return fmt.Errorf("--obj 는 --type ITM 원본 출력에만 씁니다 (코드 고르기는 --axis)")
	}
	cmd.SilenceUsage = true

	var orgID, tblID string
	if len(args) == 0 {
		keyword := interactive.Prompt("? 검색어를 입력하세요:")
		if keyword == "" {
			return fmt.Errorf("검색어가 입력되지 않았습니다")
		}
		client, err := newCmdClient()
		if err != nil {
			return err
		}
		fmt.Fprint(os.Stderr, "  📋 검색 중...\n")
		searchResults, err := client.Search(keyword, api.SearchOptions{ResultCount: 20})
		if err != nil {
			return err
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
		orgID, tblID = searchResults[selectedIdx].OrgID, searchResults[selectedIdx].TblID
	} else {
		orgID, tblID = args[0], args[1]
	}

	client, err := newCmdClient()
	if err != nil {
		return err
	}

	// 코드 고르기: type=ITM 1번만 부른다 (수록정보 PRD 는 필요 없음). --axis item 이면 항목만 받는다.
	if selecting {
		obj := ""
		if strings.EqualFold(axisF, "item") {
			obj = "ITEM"
		}
		rows, err := client.MetaCodes(orgID, tblID, obj)
		if err != nil {
			return fmt.Errorf("메타데이터 조회 실패: %w", err)
		}
		return printCodeRows(rows, findF, underF, levelF, axisF, format)
	}

	if metaType == "" {
		summary, err := client.MetaSummary(orgID, tblID)
		if err != nil {
			return fmt.Errorf("메타데이터 조회 실패: %w", err)
		}
		if format == "json" {
			data := []map[string]interface{}{{
				"ORG_ID":          orgID,
				"TBL_ID":          tblID,
				"CLASSIFICATIONS": summary.Classifications,
				"ITEMS":           summary.Items,
				"PERIODS":         summary.Periods,
			}}
			formatter, err := output.NewFormatter(format)
			if err != nil {
				return err
			}
			if err := formatter.Format(data, output.FormatOptions{}); err != nil {
				return fmt.Errorf("출력 실패: %w", err)
			}
			return nil
		}
		displayMetaSummary(summary, orgID, tblID)
		return nil
	}

	// 유형마다 필드가 달라 API 필드를 그대로 출력한다
	results, err := client.MetaRawObj(orgID, tblID, metaType, objF)
	if err != nil && strings.Contains(err.Error(), "[30]") { // 그 유형의 메타 없음
		results, err = nil, nil
	}
	if err != nil {
		return fmt.Errorf("메타데이터 조회 실패: %w", err)
	}
	if len(results) == 0 {
		fmt.Fprintf(os.Stderr, "'%s' 타입의 메타데이터가 없습니다.\n", metaType)
		if format == "json" {
			fmt.Println("[]")
		}
		return nil
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
		return err
	}
	data := make([]map[string]interface{}, len(results))
	for i, r := range results {
		data[i] = r
	}
	if err := formatter.Format(data, output.FormatOptions{Columns: columns}); err != nil {
		return fmt.Errorf("출력 실패: %w", err)
	}
	fmt.Fprintf(os.Stderr, "\n결과: %d개\n", len(results))
	return nil
}

func init() {
	rootCmd.AddCommand(metaCmd)

	metaCmd.Flags().String("type", "", "조회할 메타 유형. 미지정=요약(분류+항목+수록정보). ITM=분류/항목, PRD=수록정보(시점범위·주기), TBL=통계표명, ORG=기관명, CMMT=주석, UNIT=단위, SOURCE=출처, WGT=가중치, NCD=갱신일")
	metaCmd.Flags().StringP("format", "f", "table", "출력 형식: table=정렬표, json=파이프/jq용")
	metaCmd.Flags().String("find", "", "이름으로 코드 찾기 (쉼표로 여러 개, 공백 무시. 예: \"울산,울주\")")
	metaCmd.Flags().String("under", "", "이 코드 바로 아래 코드들 (예: --under 26 → 울산의 구·군)")
	metaCmd.Flags().Int("level", 0, "이 단계의 코드만 (1=최상위. 예: 산업 대분류)")
	metaCmd.Flags().String("axis", "", "이 축의 코드만: item(항목), class1…class8, 또는 분류 이름 일부(예 산업)")
	metaCmd.Flags().String("obj", "", "--type ITM 원본에서 이 분류 OBJ_ID(예 A) 또는 ITEM 만 받기")
}

// summaryAxisMax 요약에서 한 축을 다 보여 줄 최대 코드 수. 넘으면 최상위 코드만 보여 준다
// (AI 가 읽을 출력이 수백·수천 줄로 불어나지 않게). 전체는 --axis·--under·--find 로 본다.
const summaryAxisMax = 50

// summaryLevels 큰 축에서 최상위부터 한 단계씩, summaryAxisMax 를 넘지 않을 때까지 펼친 코드 (원래 순서 유지).
func summaryLevels(g []api.MetaResult) []api.MetaResult {
	inAxis := map[string]bool{}
	for _, c := range g {
		inAxis[c.ItmID] = true
	}
	in := map[string]bool{}
	var frontier map[string]bool // nil = 최상위 단계 (상위 코드가 없거나 이 축에 없는 코드 — Codes 와 같은 기준)
	for {
		next := map[string]bool{}
		for _, c := range g {
			top := c.UpItmID == "" || !inAxis[c.UpItmID]
			if ((frontier == nil && top) || (frontier != nil && frontier[c.UpItmID])) && !in[c.ItmID] {
				next[c.ItmID] = true
			}
		}
		if len(next) == 0 || len(in)+len(next) > summaryAxisMax {
			break
		}
		for k := range next {
			in[k] = true
		}
		frontier = next
	}
	var out []api.MetaResult
	for _, c := range g {
		if in[c.ItmID] {
			out = append(out, c)
		}
	}
	if len(in) == 0 || len(out) > summaryAxisMax { // 최상위만으로도 넘치거나 ItmID 가 겹치면 앞에서부터
		return g[:summaryAxisMax]
	}
	return out
}

// displayMetaSummary displays metadata summary in a formatted way
func displayMetaSummary(summary *api.MetaSummaryResult, orgID, tblID string) {
	fmt.Println("\n=== 메타데이터 요약 ===")
	fmt.Println()

	// Display classifications
	if len(summary.Classifications) > 0 {
		axes := map[string]bool{}
		for _, c := range summary.Classifications {
			axes[c.ObjID] = true
		}
		fmt.Printf("[분류] d 의 -c1, -c2 … 에 사용 (축 %d개, 코드 %d개)\n", len(axes), len(summary.Classifications))
		flagOf := map[string]string{} // OBJ_ID → -cN. Codes 는 Classifications 를 같은 순서로 한 행씩 만든다
		codes := summary.Codes()
		for i, c := range summary.Classifications {
			if _, ok := flagOf[c.ObjID]; !ok && i < len(codes) {
				flagOf[c.ObjID] = "-c" + strings.TrimPrefix(codes[i].Flag, "--class")
			}
		}
		var order []string
		groups := map[string][]api.MetaResult{}
		for _, c := range summary.Classifications {
			if _, ok := groups[c.ObjID]; !ok {
				order = append(order, c.ObjID)
			}
			groups[c.ObjID] = append(groups[c.ObjID], c)
		}
		for _, obj := range order {
			g := groups[obj]
			fmt.Printf("\n  %s ← 분류명: %s  (분류ID %s)\n", flagOf[obj], g[0].ObjNM, obj)
			show := g
			if len(g) > summaryAxisMax {
				show = summaryLevels(g)
			}
			kids := map[string]int{} // 상위 코드 → 하위 코드 수
			for _, c := range g {
				kids[c.UpItmID]++
			}
			shown := map[string]bool{}
			for _, c := range show {
				shown[c.ItmID] = true
			}
			for _, c := range show {
				code := c.ItmID
				name := c.ItmNM
				if code == "" {
					code = c.Code
				}
				if name == "" {
					name = c.Label
				}
				hidden := ""
				if n := kids[c.ItmID]; n > 0 && len(show) < len(g) {
					// 하위가 안 보이는 코드만 표시 — 시도 순위에서 통합시(12) 아래 1224·1236 같은 하위를 놓치지 않게
					visible := 0
					for _, k := range g {
						if k.UpItmID == c.ItmID && shown[k.ItmID] {
							visible++
						}
					}
					if visible == 0 {
						hidden = fmt.Sprintf("  (하위 %d개: --under %s)", n, code)
					} else if visible < n {
						hidden = fmt.Sprintf("  (하위 %d개 중 %d개 표시: --under %s)", n, visible, code)
					}
				}
				fmt.Printf("    %-16s %s%s\n", code, name, hidden)
			}
			if len(show) < len(g) {
				fmt.Printf("    … 이 축 코드 %d개 중 %d개만 표시. 하위: kosis m %s %s --under <코드> / 이름: --find <이름> / 전체: --axis %q\n",
					len(g), len(show), orgID, tblID, g[0].ObjNM)
			}
		}
		fmt.Println()
	}

	// Display items
	if len(summary.Items) > 0 {
		fmt.Printf("[항목] d 의 -i 에 사용 (%d개 항목)\n", len(summary.Items))
		for _, item := range summary.Items {
			code := item.ItmID
			name := item.ItmNM
			if code == "" {
				code = item.Code
			}
			if name == "" {
				name = item.Label
			}
			if u := strings.TrimSpace(item.UnitNM); u != "" {
				name += "  [" + u + "]"
			}
			fmt.Printf("    %-16s %s\n", code, name)
		}
		fmt.Println()
	}

	// Display periods
	if len(summary.Periods) > 0 {
		fmt.Printf("[수록정보] d 의 -p 에 사용\n")
		for _, p := range summary.Periods {
			code := p.PrdSe
			name := p.PrdDe
			if code == "" {
				code = p.Code
			}
			if name == "" {
				name = p.Label
			}
			if p.StrtPrdDe != "" || p.EndPrdDe != "" {
				fmt.Printf("    -p %-3s (%s)  %s ~ %s\n", api.NormalizePrdSe(code), code, p.StrtPrdDe, p.EndPrdDe)
				continue
			}
			fmt.Printf("    prdSe=%s  %s\n", code, name)
		}
		fmt.Println()
	}
}

// errNoCodeMatch 찾는 코드가 없을 때 — grep 처럼 종료 코드 1 이라 AI·스크립트가 실패를 알아챈다.
var errNoCodeMatch = errors.New("맞는 코드가 없습니다 (--find·--under·--level·--axis 조건). 이름을 더 짧게(일부만) 해서 --find 하거나 --axis <축> 으로 그 축 전체를 보세요")

// findFold 이름 비교 때 같게 볼 글자: 물결·대시류 → '-', 가운뎃점류 → '·'.
var findFold = strings.NewReplacer("~", "-", "～", "-", "〜", "-", "–", "-", "—", "-", "‐", "-", "−", "-", "－", "-", "ㆍ", "·", "・", "·", "･", "·", "∙", "·", "‧", "·")

// normFindName --find 비교용: 공백을 지우고 표기가 다른 같은 기호를 하나로 ("15~29" = "15 - 29세").
func normFindName(s string) string {
	return findFold.Replace(strings.Join(strings.Fields(s), ""))
}

// printCodeRows --find·--under·--level 로 거른 코드 목록과, 플래그별로 d 에 바로 넣을 "a+b+c".
func printCodeRows(rows []api.CodeRow, find, under string, level int, axis, format string) error {
	var names []string
	for _, n := range strings.Split(find, ",") {
		if n = normFindName(n); n != "" {
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
		if axis != "" && !axisMatches(r, axis) {
			continue
		}
		if len(names) > 0 {
			hit := false
			for _, n := range names {
				if strings.Contains(normFindName(r.Name), n) {
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
		if err := enc.Encode(out); err != nil {
			return err
		}
		if len(out) == 0 {
			return errNoCodeMatch
		}
		return nil
	}
	if len(out) == 0 {
		return errNoCodeMatch
	}
	data := make([]map[string]interface{}, len(out))
	for i, r := range out {
		data[i] = map[string]interface{}{"플래그": r.Flag, "분류": r.Axis, "코드": r.Code, "이름": r.Name,
			"단계": r.Level, "상위": strings.TrimSpace(r.ParentCode + " " + r.ParentName), "경로": r.Path}
	}
	formatter, err := output.NewFormatter("table")
	if err != nil {
		return err
	}
	if err := formatter.Format(data, output.FormatOptions{Columns: []string{"플래그", "분류", "코드", "이름", "단계", "상위", "경로"}}); err != nil {
		return fmt.Errorf("출력 실패: %w", err)
	}
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
		short := strings.Replace(strings.Replace(f, "--class", "-c", 1), "--item", "-i", 1) // SKILL 과 같은 짧은 표기
		fmt.Printf("  %s \"%s\"\n", short, strings.Join(codes[f], "+"))
	}
	return nil
}

// axisMatches --axis 값: item, class1…class8(플래그 이름), 또는 분류 이름 일부(공백 무시).
func axisMatches(r api.CodeRow, axis string) bool {
	a := strings.ToLower(strings.TrimPrefix(axis, "--"))
	if a == "item" {
		return r.Flag == "--item"
	}
	if strings.HasPrefix(a, "class") {
		return r.Flag == "--"+a
	}
	return r.Flag != "--item" && strings.Contains(squash(r.Axis), squash(axis))
}
