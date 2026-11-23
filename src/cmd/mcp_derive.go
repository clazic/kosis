package cmd

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/clazic/kosis/internal/api"
)

// DataDerive kosis_data 의 계산 인자 — kosis d 의 --sort/--top/--sort-by/--change/--ratio/--no-total 과 같다.
// AI 가 눈으로 정렬하거나 손으로 빼고 나누지 않게 서버가 계산한다.
type DataDerive struct {
	Sort    string // desc | asc
	Top     int    // 정렬 뒤 앞 N행 (경계 동률은 함께)
	SortBy  string // value | change | pct | ratio
	Change  string // yoy | prev
	Ratio   string // 분자코드/분모코드
	NoTotal bool   // 여러 코드를 준 축의 합계 행 빼기
}

func (d DataDerive) empty() bool {
	return d.Sort == "" && d.Top == 0 && d.SortBy == "" && d.Change == "" && d.Ratio == "" && !d.NoTotal
}

// validate 인자 조합 검사 (조회 전).
func (d DataDerive) validate() error {
	if d.Sort != "" && d.Sort != "desc" && d.Sort != "asc" {
		return fmt.Errorf("sort 는 desc 또는 asc 입니다 (받은 값: %q)", d.Sort)
	}
	if d.Top < 0 || (d.Top > 0 && d.Sort == "") {
		return fmt.Errorf("top 은 1 이상이고 sort(desc|asc)와 함께 씁니다")
	}
	if d.Change != "" && d.Change != "yoy" && d.Change != "prev" {
		return fmt.Errorf("change 는 yoy 또는 prev 입니다 (받은 값: %q)", d.Change)
	}
	if d.Ratio != "" {
		n, dn, _ := strings.Cut(d.Ratio, "/")
		if strings.Count(d.Ratio, "/") != 1 || strings.TrimSpace(n) == "" || strings.TrimSpace(dn) == "" {
			return fmt.Errorf("ratio 는 분자코드/분모코드 입니다 (예 T2/T1, ALL/00, Y70+Y75/000). 받은 값: %q", d.Ratio)
		}
	}
	if d.SortBy != "" {
		ok := map[string]bool{"value": true, "change": d.Change != "", "pct": d.Change != "", "ratio": d.Ratio != ""}[d.SortBy]
		if d.Sort == "" || !ok {
			return fmt.Errorf("sort_by 는 sort 와 함께 value|change|pct|ratio 중 하나 (change·pct 는 change, ratio 는 ratio 인자 필요). 받은 값: %q", d.SortBy)
		}
	}
	return nil
}

// derived 계산 결과: 줄어든·정렬된 행과 덧붙일 열.
type derived struct {
	rows    []api.DataRow
	changes map[string][2]string // 행 키 → {증감, 증감률}
	ratios  map[string]string
	ranks   map[string]string
}

// columns 덧붙일 열 (API 키 · 이름).
func (dv derived) columns() []DataColumn {
	var cols []DataColumn
	if dv.changes != nil {
		cols = append(cols, DataColumn{Key: "DT_CHG", Label: "증감"}, DataColumn{Key: "DT_CHG_PCT", Label: "증감률(%)"})
	}
	if dv.ratios != nil {
		cols = append(cols, DataColumn{Key: "DT_RATIO_PCT", Label: "비율(%)"})
	}
	if dv.ranks != nil {
		cols = append(cols, DataColumn{Key: "RANK", Label: "순위"})
	}
	return cols
}

func (dv derived) fill(r api.DataRow, m map[string]string) {
	k := rowKey(r)
	if dv.changes != nil {
		c := dv.changes[k]
		m["DT_CHG"], m["DT_CHG_PCT"] = c[0], c[1]
	}
	if dv.ratios != nil {
		m["DT_RATIO_PCT"] = dv.ratios[k]
	}
	if dv.ranks != nil {
		m["RANK"] = dv.ranks[k]
	}
}

// mcpDerive kosis d 와 같은 순서로 계산한다: 증감 → 비율 → 합계 빼기 → 정렬·top·순위.
// 계산 함수는 kosis d 것을 그대로 쓴다. note 로 안내·경고를 모은다 (kosis d 의 stderr 줄과 같은 내용, 인자 이름만 MCP 식).
func mcpDerive(client *api.Client, q DataQuery, results []api.DataRow, cmmt []map[string]interface{}, given []string, note func(string, ...any)) (derived, error) {
	d := q.Derive
	dv := derived{rows: results}
	if d.Change != "" {
		dv.changes = computeChanges(dv.rows, d.Change == "yoy")
		if notesSay(cmmt, "누계") {
			note("표 주석에 '누계'가 있습니다 — 값이 연초부터의 누계면 change 는 누계끼리 비교한 값입니다(그 달만의 증감이 아님). 그 달 값은 이번 달 누계 − 지난달 누계 (kosis_meta type=CMMT 로 확인)")
		}
		if len(dv.changes) == 0 && len(dv.rows) > 0 {
			note("change: 비교할 시점이 결과에 없습니다 — 비교 시점까지 받으세요 (yoy: 월 latest 13, 분기 latest 5, 년 latest 2 / prev: latest 2, 또는 start·end 로 두 시점)")
		} else {
			kept, noChange := keepChanged(dv.rows, dv.changes)
			if len(kept) < len(dv.rows) {
				note("change: 비교 기준으로만 쓴 앞 시점 행 %d개는 뺐습니다 (남은 행 %d개)", len(dv.rows)-len(kept), len(kept))
			}
			if len(noChange) > 0 {
				var ids []string
				for _, r := range noChange {
					ids = append(ids, strings.TrimSpace(strings.Join([]string{r.C1, r.C2, r.C3, r.ItmID}, " "))+"@"+r.PrdDe)
				}
				note("증감을 못 구한 최신 행 %d개 (비교 시점 값이 없거나 숫자 아님 — 신설 코드·X): %s", len(noChange), capList(ids, 10))
			}
			dv.rows = kept
		}
	}
	if d.Ratio != "" {
		num, den, _ := strings.Cut(d.Ratio, "/")
		ratios, notes, err := computeRatiosNotes(dv.rows, strings.TrimSpace(num), strings.TrimSpace(den))
		if err != nil {
			return dv, fmt.Errorf("ratio: %v", err)
		}
		for _, n := range notes {
			note("ratio%s", strings.NewReplacer("kosis ratio --scale 1", "kosis_ratio scale=1").Replace(n))
		}
		dv.ratios = ratios
	}
	missBeforeSort := missingClasses(given, dv.rows) // no_total·top 으로 줄기 전에
	if d.NoTotal {
		before := len(dv.rows)
		dv.rows = dropTotals(dv.rows, given)
		note("no_total: 합계 행 %d개 뺌 (이름이 전국·계·합계·총계·소계·전체인 행 — 이름이 다른 합계는 못 잡으니 코드를 직접 고르세요)", before-len(dv.rows))
	}
	if d.Sort == "" {
		return dv, nil
	}
	by := d.SortBy
	if by == "" && dv.changes != nil {
		by = "change" // change 와 함께면 증감으로 (수치값 순위는 sort_by=value)
	}
	if ((by == "change" || by == "pct") && len(dv.changes) == 0) || (by == "ratio" && len(dv.ratios) == 0) {
		return dv, fmt.Errorf("sort 기준 값(증감·비율)이 하나도 없습니다 — 비교 시점까지 받으세요 (yoy: 월 latest 13, 분기 latest 5, 년 latest 2 / prev: latest 2) 또는 sort_by=value")
	}
	val := func(r api.DataRow) string { return r.DT }
	switch by {
	case "change":
		val = func(r api.DataRow) string { return dv.changes[rowKey(r)][0] }
	case "pct":
		val = func(r api.DataRow) string { return dv.changes[rowKey(r)][1] }
	case "ratio":
		val = func(r api.DataRow) string { return dv.ratios[rowKey(r)] }
	}
	if by != "" {
		hint := ""
		if by == "change" && d.SortBy == "" {
			hint = " — 증감률(%) 순위는 sort_by=pct"
		}
		note("sort: %s 기준%s", map[string]string{"value": "수치값", "change": "증감", "pct": "증감률(%)", "ratio": "비율(%)"}[by], hint)
	}
	groups := map[string]bool{}
	grp := sortGrouper(dv.rows) // 자르기 전 결과로 한 번만
	mixedAxes := multiCodeAxes(dv.rows)
	for _, r := range dv.rows {
		groups[grp(r)] = true
	}
	dv.rows = sortTopBy(dv.rows, d.Sort == "desc", d.Top, val)
	dv.ranks = rankRows(dv.rows, val, grp) // 묶음은 자르기 전 결과 기준
	summary, _ := client.MetaSummary(q.OrgID, q.TblID)
	if miss := withoutCoveredParents(summary, given, missBeforeSort); len(miss) > 0 {
		hint := " — 그 코드의 값이 없어 순위에서 빠졌습니다"
		if summary != nil && len(childrenOfMissing(summary, miss)) > 0 {
			hint = " — 위 '아래 코드' 안내대로 하위 코드를 넣어야 순위가 맞습니다"
		}
		note("순위에 없는 코드: %s%s", capList(miss, 10), hint)
	}
	if len(mixedAxes) > 1 {
		note("코드가 여럿인 축이 %d개(%s) — 순위가 그 조합으로 섞입니다. 순위를 매길 축 하나만 여러 코드로 두세요", len(mixedAxes), strings.Join(mixedAxes, "×"))
	}
	if len(groups) > 1 {
		note("sort: 항목·시점 %d묶음마다 따로 정렬·순위를 매겼습니다 (한 항목·시점만 보려면 item·latest 로 좁히세요)", len(groups))
	} else if d.Top > 0 && len(dv.rows) > d.Top {
		note("top %d: %d위와 값이 같은 행까지 %d행을 보여 줍니다 (동률)", d.Top, d.Top, len(dv.rows))
	}
	note("RANK(순위) 열이 순위입니다 — 이름을 묻는 질문에는 RANK 1 행의 이름(C1_NM 등)을 답하세요")
	return dv, nil
}

// ── 두 표 비율 (kosis ratio) ──────────────────────────────────

// RatioAnswer kosis_ratio 결과.
type RatioAnswer struct {
	Rows    []map[string]string `json:"rows"`
	Notices []string            `json:"notices,omitempty"`
}

// MCPRatio 다른 두 표(또는 같은 표의 두 조회)의 값을 이름·시점으로 맞춰 나눈다 (kosis ratio).
// 각 쪽은 kosis_data 와 같은 검사를 거치고, 그 안내는 [분자]/[분모] 를 붙여 Notices 로.
func MCPRatio(client *api.Client, num, den DataQuery, scale float64, noTotal bool, sortDir string, top int) (*RatioAnswer, error) {
	if scale == 0 {
		return nil, fmt.Errorf("scale 은 0 이 아닌 수입니다 (100=%%, 1=배수, 1000=천명당)")
	}
	if sortDir != "" && sortDir != "desc" && sortDir != "asc" {
		return nil, fmt.Errorf("sort 는 desc 또는 asc 입니다")
	}
	if top < 0 || (top > 0 && sortDir == "") {
		return nil, fmt.Errorf("top 은 1 이상이고 sort(desc|asc)와 함께 씁니다")
	}
	ans := &RatioAnswer{Rows: []map[string]string{}}
	side := func(label string, q DataQuery) (ratioSide, error) {
		q.Fields = ""
		q.Derive = DataDerive{}
		a, err := MCPData(client, q)
		if err != nil {
			return ratioSide{}, fmt.Errorf("[%s] %v", label, err)
		}
		for _, n := range a.Notices {
			ans.Notices = append(ans.Notices, fmt.Sprintf("[%s] %s", label, n))
		}
		s, err := ratioSideFromRows(label, a.Rows)
		if err != nil {
			return s, fmt.Errorf("%s", strings.NewReplacer("-i 는 하나만", "item 은 하나만").Replace(err.Error()))
		}
		return s, nil
	}
	n, err := side("분자", num)
	if err != nil {
		return nil, err
	}
	d, err := side("분모", den)
	if err != nil {
		return nil, err
	}
	out, notes, err := ratioJoin(n, d, scale, noTotal, sortDir, top)
	rep := strings.NewReplacer("ℹ ", "", "⚠ ", "", "-s/-e", "start·end", "d 로 따로 조회", "kosis_data 로 따로 조회")
	for _, x := range notes {
		ans.Notices = append(ans.Notices, rep.Replace(x))
	}
	if err != nil {
		return nil, fmt.Errorf("%s", rep.Replace(err.Error()))
	}
	for i, o := range out {
		m := map[string]string{"NAME": o.Name, "PRD_DE": o.Period, "NUM": o.Num, "DEN": o.Den, "RATIO": o.Ratio}
		if sortDir != "" {
			m["RANK"] = strconv.Itoa(i + 1)
		}
		ans.Rows = append(ans.Rows, m)
	}
	return ans, nil
}
