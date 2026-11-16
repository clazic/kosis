package cmd

// MCP 서버(cmd/kosis-mcp)용 함수. CLI 명령(s·m·d·ls·ex)과 같은 검사·안내 도우미를 부르되,
// stderr·stdout 에 쓰지 않고 결과와 안내 줄(Notices)로 돌려준다. 파일(이력 등)은 쓰지 않는다.
// CLI 명령의 동작은 이 파일과 무관하다.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/clazic/kosis/internal/api"
)

// ── 검색 (kosis s) ────────────────────────────────────────────

// SearchQuery kosis_search 입력 (kosis s 플래그와 같은 뜻).
type SearchQuery struct {
	Keyword string
	Limit   int    // 1~1000
	Page    int    // 1부터
	Sort    string // RANK | DATE
	Org     string // 기관 코드 또는 기관명 일부
	Stat    string // 조사명 일부 (받은 결과 안에서 거름)
	Region  string // 지역 이름 (받은 결과 안에서 거름)
}

// SearchAnswer 검색 결과 행(-f json 과 같은 키)과 범위·다음 단계 안내.
type SearchAnswer struct {
	Results []map[string]string `json:"results"`
	Total   int                 `json:"total"`
	Notices []string            `json:"notices,omitempty"`
}

// MCPSearch kosis s 와 같은 순서로 검색한다: 기관 이름 → 코드, 기관마다 검색, 번갈아 합치기, 조사명·지역 거르기.
func MCPSearch(client *api.Client, q SearchQuery) (*SearchAnswer, error) {
	ans := &SearchAnswer{Results: []map[string]string{}}
	orgIDs, orgNote, err := resolveOrgs(client, q.Org)
	if err != nil {
		return nil, err
	}
	if orgNote != "" {
		ans.Notices = append(ans.Notices, orgNote)
	}
	clientFilter := q.Stat != "" || q.Region != ""
	fetch := q.Limit
	if clientFilter {
		fetch = min(max(q.Limit*10, 300), 1000)
	}
	var perOrg [][]api.SearchResult
	for _, org := range orgIDs {
		rs, err := client.Search(q.Keyword, api.SearchOptions{ResultCount: fetch, StartCount: q.Page, Sort: q.Sort, OrgID: org})
		if err != nil && strings.Contains(err.Error(), "[30]") { // 결과 없음
			rs, err = nil, nil
		}
		if err != nil {
			return nil, fmt.Errorf("검색 실패: %w", err)
		}
		if len(rs) > 0 {
			n, _ := strconv.Atoi(rs[0].StatDBCnt)
			ans.Total += n
		}
		perOrg = append(perOrg, rs)
	}
	results := interleave(perOrg)
	fetched := len(results)
	if clientFilter {
		results = filterSearch(results, "", q.Stat, q.Region)
	}
	filtered := len(results)
	if len(results) > q.Limit {
		results = results[:q.Limit]
	}
	scope := fmt.Sprintf("검색 범위: 전체 %d개 중 %d쪽에서 %d개 받음", ans.Total, q.Page, fetched)
	if clientFilter {
		scope += fmt.Sprintf(" → stat/region 으로 거른 뒤 %d개", filtered)
	}
	ans.Notices = append(ans.Notices, scope)
	if len(results) == 0 {
		ans.Notices = append(ans.Notices, fmt.Sprintf("'%s' 검색 결과가 없습니다", q.Keyword))
		if clientFilter && ans.Total > q.Page*fetched {
			ans.Notices = append(ans.Notices, fmt.Sprintf("stat/region 은 받은 %d개 안에서만 거릅니다. 다음 쪽도 보세요: page %d", fetched, q.Page+1))
		}
		if q.Region != "" {
			ans.Notices = append(ans.Notices, "region 은 표 이름·기관·조사명·경로 글자만 봅니다. 전국 시도별·시군구별 표 안의 지역은 빠지니 region 없이 찾은 뒤 kosis_meta find 로 고르세요")
		}
		if q.Org != "" {
			ans.Notices = append(ans.Notices, "org 는 표를 만든 기관입니다. 다른 기관(예 국가데이터처)이 만든 표에 있을 수 있으니 org 없이도 찾아보세요")
		}
		ans.Notices = append(ans.Notices, "검색에서 못 찾은 것이 KOSIS 에 없다는 뜻은 아닙니다 — 다른 말(조사명·항목명)로, 또는 kosis_list 로 찾아보세요")
		return ans, nil
	}
	if ans.Total > q.Page*fetch && !clientFilter && len(results) == q.Limit {
		ans.Notices = append(ans.Notices, fmt.Sprintf("더 보기: page %d", q.Page+1))
	}
	for _, r := range results {
		ans.Results = append(ans.Results, map[string]string{
			"ORG_ID": r.OrgID, "ORG_NM": r.OrgNM, "TBL_ID": r.TblID, "TBL_NM": r.TblNM,
			"STAT_ID": r.StatID, "STAT_NM": r.StatNM, "PATH": r.MTAtitle,
			"STRT_PRD_DE": r.StrtPrdDe, "END_PRD_DE": r.EndPrdDe,
		})
	}
	return ans, nil
}

// ── 메타 (kosis m) ────────────────────────────────────────────

// MCPFilterCodes kosis m --find/--under/--level/--axis 와 같은 거르기와, 결과를 kosis_data 인자로 이은 값
// (class1: "a+b", item: "T1"). 맞는 코드가 없으면 kosis m 과 같은 안내를 담은 오류.
func MCPFilterCodes(rows []api.CodeRow, find, under string, level int, axis string) ([]api.CodeRow, map[string]string, error) {
	out := filterCodeRows(rows, find, under, level, axis)
	if len(out) == 0 {
		return nil, nil, noMatch(rows, under, level, axis)
	}
	params := map[string]string{}
	for _, r := range out {
		k := strings.TrimPrefix(r.Flag, "--")
		if params[k] != "" {
			params[k] += "+"
		}
		params[k] += r.Code
	}
	return out, params, nil
}

// MCPMetaSummary kosis m 요약을 구조로: 축마다 플래그(class1…)·코드(큰 축은 m 과 같이 최대 50개까지 위 단계부터),
// 항목(코드·이름·단위), 수록정보(period 에 넣을 주기 코드와 시점 범위).
func MCPMetaSummary(summary *api.MetaSummaryResult, orgID, tblID string) map[string]any {
	type code struct {
		Code   string `json:"code"`
		Name   string `json:"name"`
		Parent string `json:"parent,omitempty"`
	}
	axes := []map[string]any{}
	groups := map[string][]api.MetaResult{}
	for _, c := range summary.Classifications {
		groups[c.ObjID] = append(groups[c.ObjID], c)
	}
	for _, a := range summary.Axes() {
		g := groups[a.ObjID]
		show := g
		if len(g) > summaryAxisMax {
			show = summaryLevels(g)
		}
		codes := make([]code, 0, len(show))
		for _, c := range show {
			codes = append(codes, code{Code: c.ItmID, Name: cleanSpaces(c.ItmNM), Parent: c.UpItmID})
		}
		ax := map[string]any{
			"param": fmt.Sprintf("class%d", a.N), "obj_id": a.ObjID, "name": a.Name,
			"code_count": len(g), "codes": codes,
		}
		if len(show) < len(g) {
			ax["note"] = fmt.Sprintf("코드 %d개 중 %d개(위 단계부터)만 보임. 나머지: kosis_meta 의 under=<코드>, find=<이름>, axis=%q", len(g), len(show), a.Name)
		}
		axes = append(axes, ax)
	}
	items := []map[string]string{}
	for _, it := range summary.Items {
		m := map[string]string{"code": it.ItmID, "name": cleanSpaces(it.ItmNM)}
		if it.UnitNM != "" {
			m["unit"] = it.UnitNM
		}
		items = append(items, m)
	}
	periods := []map[string]string{}
	for _, p := range summary.Periods {
		periods = append(periods, map[string]string{
			"period": api.NormalizePrdSe(p.PrdSe), "prd_se": p.PrdSe, "start": p.StrtPrdDe, "end": p.EndPrdDe,
		})
	}
	out := map[string]any{"org_id": orgID, "tbl_id": tblID, "axes": axes, "items": items, "periods": periods}
	if len(summary.Axes()) > 0 && !summary.HasAxis(1) {
		out["note"] = "1번 축이 없는 표 — kosis_data 는 class1 을 ALL 로 채운다"
	}
	return out
}

func cleanSpaces(s string) string { return strings.Join(strings.Fields(s), " ") }

// ── 목록 (kosis ls) ───────────────────────────────────────────

// MCPList kosis ls 와 같은 목록. 표 단계면 ORG_ID·TBL_ID·TBL_NM, 아니면 LIST_ID·LIST_NM.
func MCPList(client *api.Client, view, parent string) ([]map[string]string, []string, error) {
	var notices []string
	if strings.EqualFold(view, "MT_OTITLE") && parent != "" {
		notices = append(notices, fmt.Sprintf("기관별 뷰(MT_OTITLE)는 기관 목록만 줍니다 — 그 기관의 표는 kosis_search 의 org=%s", parent))
	}
	results, err := client.List(api.ListOptions{VwCd: view, ParentID: parent})
	if err != nil {
		if strings.Contains(err.Error(), "[30]") {
			return nil, notices, fmt.Errorf("parent %s 아래 목록이 없습니다 (뷰 %s). parent 에는 앞 단계 결과의 LIST_ID 를 넣습니다 — 최상위는 parent 없이", parent, view)
		}
		return nil, notices, fmt.Errorf("통계목록 조회 실패: %w", err)
	}
	rows := []map[string]string{}
	for _, it := range results {
		if it.TblID != "" {
			rows = append(rows, map[string]string{"ORG_ID": it.OrgID, "TBL_ID": it.TblID, "TBL_NM": it.TblNM})
		} else {
			rows = append(rows, map[string]string{"LIST_ID": it.ListID, "LIST_NM": it.ListNM})
		}
	}
	return rows, notices, nil
}

// ── 설명 (kosis ex) ───────────────────────────────────────────

// MCPExplain kosis ex -f json 과 같은 내용: 통계설명 필드, 표 주석, 원표·조사설명 링크.
func MCPExplain(client *api.Client, orgID, tblID string) (map[string]any, error) {
	expl, err := client.Explain(orgID, tblID)
	if err != nil && strings.Contains(err.Error(), "[30]") {
		err = nil // 통계설명이 없는 표 — 주석·링크는 있다
	}
	var warn string
	if err != nil {
		warn = fmt.Sprintf("통계설명 조회 실패: %v", err)
	}
	notes := []string{}
	if cmmt, cerr := client.MetaRaw(orgID, tblID, "CMMT"); cerr == nil {
		for _, c := range cmmt {
			if dc := unescapeText(strings.TrimSpace(fmt.Sprint(c["CMMT_DC"]))); dc != "" && c["CMMT_DC"] != nil {
				notes = append(notes, dc)
			}
		}
	}
	if expl == nil {
		expl = map[string]string{}
	}
	for k, v := range expl {
		expl[k] = unescapeText(v)
	}
	links := map[string]string{"table": api.TableURL(orgID, tblID)}
	if u := api.ExplainURL(expl["confmNo"]); u != "" {
		links["survey"] = u
	}
	out := map[string]any{"org_id": orgID, "tbl_id": tblID, "links": links, "notes": notes, "explain": expl}
	if warn != "" {
		out["warning"] = warn
	}
	if len(expl) == 0 {
		out["notice"] = "이 표의 통계설명이 없습니다. 원표 페이지(links.table)에서 확인하세요"
	}
	return out, nil
}

// ── 데이터 (kosis d) ──────────────────────────────────────────

// DataQuery kosis_data 입력 (kosis d 의 같은 이름 플래그).
type DataQuery struct {
	OrgID, TblID string
	Classes      [8]string // class1 … class8
	Item         string
	Period       string // 수록주기 (Y, M, Q, H, F 또는 년·월 …)
	Start, End   string
	Latest       int
	Periods      string // 비연속 시점 "2020,2022"
	Fields       string // 출력 열 (API 키 또는 한국어 열 이름, 쉼표)
}

// DataColumn 결과 열: Key 는 행의 키(API 키 C1, C1_NM, ITM_NM, PRD_DE, DT …), Label 은 이 표의 이름.
type DataColumn struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// DataAnswer 조회 결과와 kosis d 가 stderr 로 내는 안내·경고 줄.
type DataAnswer struct {
	OrgID   string              `json:"org_id"`
	TblID   string              `json:"tbl_id"`
	Columns []DataColumn        `json:"columns"`
	Rows    []map[string]string `json:"rows"`
	Notices []string            `json:"notices,omitempty"`
}

// MCPData kosis d 와 같은 검사(빠진 축·없는 축·없는 주기·level: 펼치기·수록 범위)를 거쳐 조회하고,
// 조회 뒤 '조용한 오답' 안내(결과에 없는 코드·일찍 끊긴 코드·잠정·전망 시점·모두 0·단위 …)를 Notices 로 모은다.
// 조회 전 검사에 걸리거나 조회가 실패하면 원인 안내까지 담은 오류.
func MCPData(client *api.Client, q DataQuery) (*DataAnswer, error) {
	ans := &DataAnswer{OrgID: q.OrgID, TblID: q.TblID, Rows: []map[string]string{}}
	note := func(format string, a ...any) { ans.Notices = append(ans.Notices, fmt.Sprintf(format, a...)) }

	classes := q.Classes
	ptrs := []*string{}
	for i := range classes {
		ptrs = append(ptrs, &classes[i])
	}
	item := q.Item
	ptrs = append(ptrs, &item)
	for _, f := range ptrs {
		if fixed := joinCodes(*f); fixed != *f && strings.Contains(*f, ",") {
			note("여러 코드는 '+' 로 잇습니다 — %q 를 %q 로 바꿔 조회합니다", *f, fixed)
			*f = fixed
		}
	}
	given := func() []string { return classes[:] }
	periodsValue := strings.TrimSpace(q.Periods)
	hasRange := q.Start != "" || q.End != ""
	hasLatest := q.Latest != 0
	hasPeriods := periodsValue != ""

	if strings.Join(given(), "") == "" || item == "" || q.Period == "" {
		var need []string
		if strings.Join(given(), "") == "" {
			need = append(need, "분류(class1 …, kosis_meta 의 axes)")
		}
		if item == "" {
			need = append(need, "항목(item, kosis_meta 의 items)")
		}
		if q.Period == "" {
			need = append(need, "주기(period, kosis_meta 의 periods)")
		}
		return nil, fmt.Errorf("빠진 인자: %s. 확인: kosis_meta org_id=%s tbl_id=%s", strings.Join(need, ", "), q.OrgID, q.TblID)
	}
	if hasLatest && q.Latest <= 0 {
		return nil, fmt.Errorf("latest 는 1 이상의 정수여야 합니다")
	}
	if (q.Start == "") != (q.End == "") {
		return nil, fmt.Errorf("start 와 end 는 함께 써야 합니다")
	}
	if hasPeriods && (hasLatest || hasRange) {
		return nil, fmt.Errorf("periods 는 latest 또는 start/end 와 함께 쓸 수 없습니다")
	}
	if hasLatest && hasRange {
		return nil, fmt.Errorf("latest 와 start/end 는 함께 쓸 수 없습니다")
	}

	// 조회 전 검사 (kosis d 와 같음)
	if summary, merr := client.MetaSummary(q.OrgID, q.TblID); merr == nil {
		if classes[0] == "" && len(summary.Axes()) > 0 && !hasAxis(summary, 1) {
			classes[0] = "ALL"
			note("이 표는 축 번호가 2부터라 1번 축이 없습니다 — class1 을 ALL 로 채웠습니다")
		}
		if extra := extraClassFlags(summary, given()); len(extra) > 0 {
			return nil, fmt.Errorf("이 표는 분류 축이 %d개라 %s 는 없는 축입니다 — 빼고 조회하세요 (축 확인: kosis_meta)", axisCount(summary), flagsToParams(extra))
		}
		if missing := missingClassFlags(summary, given()); len(missing) > 0 {
			return nil, fmt.Errorf("이 표는 분류 축이 %d개라 다음 인자도 필요합니다: %s — 코드는 kosis_meta(find·under·level), 축 전체는 ALL", axisCount(summary), flagsToParams(missing))
		}
		if out := periodsOutOfRange(summary, q.Period, append([]string{q.Start, q.End}, strings.Split(periodsValue, ",")...)); out != "" {
			note("%s", out)
		}
		for i := range classes {
			f := &classes[i]
			if !strings.Contains(strings.ToLower(*f), "level:") {
				continue
			}
			expanded, lerr := expandLevels(summary, i+1, *f)
			if lerr != nil {
				return nil, fmt.Errorf("class%d %s: %v", i+1, *f, lerr)
			}
			note("class%d %s → 코드 %d개: %s (뺄 코드는 !코드 — 예 \"level:1+!03\")", i+1, *f, strings.Count(expanded, "+")+1, capList(codeNames(summary, i+1, expanded), 30))
			*f = expanded
		}
		if have := cyclesOf(summary); len(have) > 0 && !have["?"] && !have[api.NormalizePrdSe(strings.ToUpper(q.Period))] {
			var list []string
			for _, p := range summary.Periods {
				list = append(list, fmt.Sprintf("%s(%s %s~%s)", api.NormalizePrdSe(p.PrdSe), p.PrdSe, p.StrtPrdDe, p.EndPrdDe))
			}
			return nil, fmt.Errorf("이 표에는 period %s 주기가 없습니다. 있는 주기: %s — 다른 주기 값이 필요하면 그 주기가 있는 표를 찾으세요", api.NormalizePrdSe(q.Period), strings.Join(list, ", "))
		}
	}

	opts := api.DataOptions{
		Class1: classes[0], Class2: classes[1], Class3: classes[2], Class4: classes[3],
		Class5: classes[4], Class6: classes[5], Class7: classes[6], Class8: classes[7],
		Item: item, PrdSe: q.Period, StartPrdDe: q.Start, EndPrdDe: q.End,
	}
	if hasLatest {
		opts.NewEstPrdCnt = strconv.Itoa(q.Latest)
	}
	var results []api.DataRow
	var err error
	if hasPeriods {
		periods := strings.Split(periodsValue, ",")
		for i := range periods {
			periods[i] = strings.TrimSpace(periods[i])
		}
		results, err = client.DataWithPeriods(q.OrgID, q.TblID, periods, opts)
	} else {
		results, err = client.DataWithAutoSplit(q.OrgID, q.TblID, opts, api.SplitOptions{MaxCells: 40000}, nil)
	}
	if err != nil {
		lines := []string{err.Error()}
		if strings.Contains(err.Error(), "[21]") || strings.Contains(err.Error(), "[30]") {
			if summary, merr := client.MetaSummary(q.OrgID, q.TblID); merr == nil {
				lines = append(lines, diagnoseCodes(summary, given(), item, q.OrgID, q.TblID)...)
			}
		}
		if strings.Contains(err.Error(), "[50]") {
			lines = append(lines, "[50] 은 KOSIS 서버 오류입니다 — 잠시 뒤 다시 해 보고, 같은 조합만 계속 실패하면 ALL 대신 코드를 좁히거나 같은 주제의 다른 표를 쓰세요")
		}
		return nil, fmt.Errorf("%s", strings.Join(lines, "\n"))
	}

	// 조회 뒤 안내 (kosis d 가 stderr 로 내는 것과 같음)
	var colMeta, colMetaFull *api.ColumnMeta
	if summary, serr := client.MetaSummary(q.OrgID, q.TblID); serr == nil {
		colMetaFull = summary.BuildColumnMetaWithCode()
		colMeta = colMetaFull
		if len(results) > 0 {
			colMeta = colMetaFull.FilterByData(results)
		}
		if line := partialYear(summary, q.Period, results); line != "" {
			note("%s", line)
		}
		if line := newerCycle(summary, q.Period); line != "" && !hasRange && !hasPeriods {
			note("%s", line)
		}
		if line := echoCodes(summary, given(), item); line != "" {
			note("%s", line)
		}
		if p := emptyLatest(results); p != "" {
			note("마지막 시점 %s 의 값이 모두 비어 있습니다(- 등) — 아직 공표 전일 수 있습니다. '최근 값'은 그 전 시점을 쓰고 그렇게 밝히세요", p)
		}
		if ended := codesEndedEarly(given(), results, ""); len(ended) > 0 {
			note("다른 코드보다 일찍 값이 끊긴 코드: %s — 폐지·개편된 코드(이름이 같은 새 코드로 이어짐)이거나 그 시점부터 수록되지 않은 행입니다. 확인: kosis_meta find=<이름>", strings.Join(ended, ", "))
			for _, h := range sameNameSuccessors(summary, ended) {
				note("%s", h)
			}
		} else if exp, last := expectedLast(summary, q.Period, q.End, periodsValue), lastPeriod(results); len(exp) == len(last) && exp > last {
			note("이 조회의 값은 %s 까지입니다 (표의 period %s 끝은 %s) — 항목·조합에 따라 일찍 끝났거나 아직 공표 전입니다", last, api.NormalizePrdSe(strings.ToUpper(q.Period)), exp)
			for _, h := range sameNameSuccessors(summary, codesEndedEarly(given(), results, exp)) {
				note("%s", h)
			}
		}
		if miss := missingClasses(given(), results); len(miss) > 0 {
			note("요청한 분류 코드 중 결과에 한 줄도 없는 코드: %s", capList(miss, 10))
			inMeta, notInMeta := splitByMeta(summary, miss)
			if len(notInMeta) > 0 {
				note("이 표 메타에 없는 코드(오타·짐작한 코드·개편된 지역 코드): %s — 코드 확인: kosis_meta find=<이름>", capList(notInMeta, 10))
			}
			if len(inMeta) > 0 {
				note("메타에는 있지만 이 시점·조합에 행이 없는 코드(통합·폐지된 코드, 그 시점 미수록, 공표 범위 밖): %s", capList(inMeta, 10))
			}
			for _, h := range childrenOfMissing(summary, miss) {
				note("%s", h)
			}
		}
		if miss := missingItems(summary.Items, item, results); len(miss) > 0 {
			span := ""
			if len(summary.Periods) > 0 {
				span = fmt.Sprintf(" (수록 기간 start=%s end=%s)", summary.Periods[0].StrtPrdDe, summary.Periods[0].EndPrdDe)
			}
			note("요청한 항목 중 이 시점 범위에 값이 하나도 없는 항목: %s — 항목마다 수록 시점이 다를 수 있습니다(표의 최신 시점 ≠ 그 항목의 최신 시점)%s", strings.Join(miss, ", "), span)
		}
	}
	if len(results) > 0 {
		cmmt, _ := client.MetaRaw(q.OrgID, q.TblID, "CMMT")
		if notesSay(cmmt, "잠정") {
			note("표 주석에 '잠정'이 있습니다 — 최근 값은 잠정치일 수 있으니 답에 밝히세요 (kosis_meta type=CMMT)")
		}
	}
	if f := futurePeriod(results, time.Now()); f != "" {
		note("결과에 아직 오지 않은 시점(%s)이 있습니다 — 추계·전망치일 수 있습니다(학년도 표는 아님 — 표 이름·주석 확인). 실적이 필요하면 start/end 로 지난 시점까지 고르세요", f)
	}
	if z := allZeroItems(results); len(z) > 0 {
		note("값이 모두 0 인 항목: %s — 작성하지 않은 항목일 수 있습니다. 같은 뜻의 다른 항목·kosis_explain 으로 확인하고, 실제로 0 이면 0 이 답입니다", strings.Join(z, ", "))
	}
	if u := joinedUnit(results); u != "" {
		note("단위 칸 %q 는 여러 단위를 이은 표 전체 단위일 수 있습니다 — 행마다 분류·항목 이름 괄호의 단위(예 '(천명당)')를 따르세요", u)
	}
	if len(results) > 0 && unitsEmpty(results) && !namesCarryUnit(results) {
		note("단위 칸이 비어 있습니다 — 항목 이름 괄호, kosis_meta 의 items unit, kosis_explain 에서 확인하고 짐작한 단위는 짐작이라고 밝히세요")
	}
	if len(sameNameCodes(results)) > 0 {
		sort.SliceStable(results, func(i, j int) bool { return results[i].PrdDe < results[j].PrdDe })
	}

	// 열: 메타가 있으면 이 표의 열(코드 열 포함), 없으면 값이 있는 API 키
	if colMeta != nil {
		for _, c := range colMeta.Columns {
			ans.Columns = append(ans.Columns, DataColumn{Key: c.Key, Label: c.Label})
		}
	} else {
		maps := make([]map[string]interface{}, len(results))
		for i, r := range results {
			maps[i] = apiKeyRow(r)
		}
		for _, k := range apiKeyColumns(maps) {
			ans.Columns = append(ans.Columns, DataColumn{Key: k, Label: k})
		}
	}
	if q.Fields != "" {
		var picked []DataColumn
		for _, f := range strings.Split(q.Fields, ",") {
			f = strings.TrimSpace(f)
			if f == "" {
				continue
			}
			if f == "비고" { // v0.5.24 까지의 LST_CHN_DE 열 이름
				f = "LST_CHN_DE"
			}
			col, ok := findColumn(colMetaFull, ans.Columns, f)
			if !ok {
				var keys []string
				for _, c := range ans.Columns {
					keys = append(keys, c.Key)
				}
				return nil, fmt.Errorf("fields 의 %q 는 이 결과에 없는 열입니다. 있는 열: %s", f, strings.Join(keys, ", "))
			}
			picked = append(picked, col)
		}
		ans.Columns = picked
	}
	for _, r := range results {
		m := make(map[string]string, len(ans.Columns))
		for _, c := range ans.Columns {
			m[c.Key] = r.GetField(c.Key)
		}
		ans.Rows = append(ans.Rows, m)
	}
	if len(results) == 0 {
		note("결과 행이 없습니다 — 코드·시점 조합을 kosis_meta 로 확인하세요")
	}
	return ans, nil
}

// findColumn fields 한 칸(API 키 또는 이 표의 열 이름) → 열.
func findColumn(full *api.ColumnMeta, cols []DataColumn, f string) (DataColumn, bool) {
	for _, c := range cols {
		if c.Key == f || c.Label == f {
			return c, true
		}
	}
	if full != nil { // 값이 없어 빠진 열도 고를 수 있다 (kosis d --fields 와 같음)
		for _, c := range full.Columns {
			if c.Key == f || c.Label == f {
				return DataColumn{Key: c.Key, Label: c.Label}, true
			}
		}
	}
	for _, k := range fallbackAPIKeys {
		if k == f {
			return DataColumn{Key: k, Label: k}, true
		}
	}
	return DataColumn{}, false
}

// flagsToParams "--class2(성별)"·"-c3" → "class2(성별)"·"class3" (MCP 인자 이름).
func flagsToParams(flags []string) string {
	out := make([]string, len(flags))
	for i, f := range flags {
		f = strings.TrimPrefix(f, "--")
		if strings.HasPrefix(f, "-c") {
			f = "class" + strings.TrimPrefix(f, "-c")
		}
		out[i] = f
	}
	return strings.Join(out, ", ")
}
