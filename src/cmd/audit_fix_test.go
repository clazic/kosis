package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/history"
	"github.com/clazic/kosis/internal/rules"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// flagCmd 실제 명령의 플래그로 값을 정한 새 명령. 플래그 값은 공유되므로 테스트가 끝나면 기본값으로 되돌린다.
func flagCmd(t *testing.T, src *cobra.Command, set map[string]string) *cobra.Command {
	reset := func() { src.Flags().VisitAll(func(f *pflag.Flag) { _ = f.Value.Set(f.DefValue) }) }
	reset()
	t.Cleanup(reset)
	c := &cobra.Command{}
	c.Flags().AddFlagSet(src.Flags())
	for k, v := range set {
		_ = c.Flags().Set(k, v)
	}
	return c
}

// 1·2: 인자·형식 오류는 API 를 부르기 전에 오류(→ 종료 코드 1)
func TestSearchArgErrorsBeforeAPI(t *testing.T) {
	t.Setenv("KOSIS_API_KEY", "") // 키가 없어도 인자 오류가 먼저
	cases := []map[string]string{
		{"limit": "-1"}, {"limit": "0"}, {"limit": "1001"},
		{"format": "nonsense"}, {"page": "0"}, {"sort": "abc"},
	}
	for _, set := range cases {
		err := runSearch(flagCmd(t, searchCmd, set), []string{"인구"})
		if err == nil {
			t.Errorf("%v: 오류여야 함", set)
		} else if strings.Contains(err.Error(), "API 키") {
			t.Errorf("%v: API 키보다 인자 검사가 먼저여야 함: %v", set, err)
		}
	}
}

// 2·4: meta 인자 1개·잘못된 형식·선택자+다른 type·--obj 오용은 오류
func TestMetaArgErrors(t *testing.T) {
	cases := []struct {
		args []string
		set  map[string]string
	}{
		{[]string{"101"}, nil},
		{[]string{"101", "DT"}, map[string]string{"format": "csv"}},
		{[]string{"101", "DT"}, map[string]string{"type": "CMMT", "find": "생산액"}},
		{[]string{"101", "DT"}, map[string]string{"type": "PRD", "axis": "item"}},
		{[]string{"101", "DT"}, map[string]string{"obj": "ITEM"}},
		{[]string{"101", "DT"}, map[string]string{"level": "-1"}},
	}
	for _, c := range cases {
		if err := runMeta(flagCmd(t, metaCmd, c.set), c.args); err == nil {
			t.Errorf("%v %v: 오류여야 함", c.args, c.set)
		}
	}
}

func TestAxisMatches(t *testing.T) {
	item := api.CodeRow{Flag: "--item", Axis: "항목"}
	ind := api.CodeRow{Flag: "--class2", Axis: "산업별"}
	if !axisMatches(item, "item") || axisMatches(ind, "item") {
		t.Error("item")
	}
	if !axisMatches(ind, "class2") || !axisMatches(ind, "--class2") || axisMatches(ind, "class1") {
		t.Error("classN")
	}
	if !axisMatches(ind, "산업") || axisMatches(item, "항목") {
		t.Error("축 이름 일부 (항목은 item 으로만)")
	}
}

// 3: 규칙 파일이 깨졌으면 오류(→ stderr·종료 코드 1), stdout 에 '오류:' 를 쓰지 않는다
func TestRulesListBadFile(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(bad, []byte("rules: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KOSIS_RULES_FILE", bad)
	if err := rulesListCmd.RunE(flagCmd(t, rulesListCmd, map[string]string{"format": "json"}), nil); err == nil {
		t.Fatal("깨진 rules 는 오류여야 함")
	}
}

func TestRuleSelected(t *testing.T) {
	r := ruleEntry(map[string]any{"id": "meta.period-codes", "topic": "meta"}, "")
	tb := ruleEntry(map[string]any{"table": "101/DT_1FS1101"}, "wrong")
	if !ruleSelected(r, "meta.period-codes", "", "") || ruleSelected(r, "meta.period", "", "") {
		t.Error("--id 는 정확히")
	}
	if !ruleSelected(tb, "101/DT_1FS1101", "", "wrong") || ruleSelected(tb, "", "", "ok") {
		t.Error("표 ID·status")
	}
	if !ruleSelected(r, "", "meta", "ok") || ruleSelected(r, "", "data", "") {
		t.Error("topic")
	}
}

// 11: 메타 실패는 빈 항목과 구분된다
func TestSearchItemsFailure(t *testing.T) {
	rs := []api.SearchResult{{OrgID: "1", TblID: "OK"}, {OrgID: "1", TblID: "EMPTY"}, {OrgID: "1", TblID: "FAIL"}}
	items, errs := searchItems(func(org, tbl string) ([]api.MetaResult, error) {
		switch tbl {
		case "OK":
			return []api.MetaResult{{ObjID: "ITEM", ItmNM: "생산액", UnitNM: "백만원"}}, nil
		case "FAIL":
			return nil, errors.New("API 오류 [40]")
		}
		return nil, nil
	}, rs)
	if items[0] != "생산액(백만원)" || errs[0] != "" {
		t.Errorf("정상: %q %q", items[0], errs[0])
	}
	if items[1] != "" || errs[1] != "" {
		t.Errorf("항목 없음은 실패가 아님: %q %q", items[1], errs[1])
	}
	if items[2] != "" || !strings.Contains(errs[2], "[40]") {
		t.Errorf("실패: %q %q", items[2], errs[2])
	}
}

// 10: 공백 든 검색어·필터가 인자 그대로 남는다
func TestHistoryKeepsArgs(t *testing.T) {
	args := []string{"s", "광업 제조업", "-n", "1", "--org", "101", "--items", "-f", "json"}
	q := history.QuoteArgs(args)
	if q != "s '광업 제조업' -n 1 --org 101 --items -f json" {
		t.Errorf("인용: %s", q)
	}
	// 셸 메타 글자는 모두 작은따옴표 안에 (백틱·줄바꿈·역슬래시·#·$·작은따옴표)
	for in, want := range map[string]string{
		"a`b`": "'a`b`'", "a\nb": "'a\nb'", `a\b`: `'a\b'`, "#x": "'#x'", "$HOME": "'$HOME'", "it's": `'it'\''s'`,
		"인구": "인구", "DT_1FS1101": "DT_1FS1101", "": "''",
	} {
		if got := history.QuoteArgs([]string{in}); got != want {
			t.Errorf("QuoteArgs(%q)=%s want %s", in, got, want)
		}
	}
}

// 20: 요청 항목 중 결과가 없는 항목
func TestMissingItems(t *testing.T) {
	meta := []api.MetaResult{{ItmID: "T01", ItmNM: "종사자수", UnitNM: "명"}, {ItmID: "T07", ItmNM: "생산액", UnitNM: "백만원"}}
	rows := []api.DataRow{{ItmID: "T01"}}
	if got := missingItems(meta, "T01+T07", rows); len(got) != 1 || got[0] != "T07 생산액(백만원)" {
		t.Errorf("나열: %v", got)
	}
	if got := missingItems(meta, "ALL", rows); len(got) != 1 {
		t.Errorf("ALL: %v", got)
	}
	if got := missingItems(meta, "T07", nil); got != nil {
		t.Errorf("결과 0행은 따지지 않음: %v", got)
	}
}

// 19: 명령용 클라이언트는 캐시를 켠다
func TestCmdClientUsesCache(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KOSIS_API_KEY", "test-key")
	c, err := newCmdClient()
	if err != nil {
		t.Fatal(err)
	}
	if !c.CacheEnabled() {
		t.Error("검색·메타 명령 클라이언트는 캐시를 써야 함")
	}
}

func ruleEntry(body map[string]any, status string) rules.Entry {
	return rules.Entry{Kind: "rule", Status: status, Body: body}
}

// 10: replay ID 는 입력 전체가 양의 정수 — "501junk" 가 501 로 실행되면 안 된다
func TestParseHistoryID(t *testing.T) {
	for _, bad := range []string{"501junk", "1junk", "0", "-1", "", "1.5", "0x10"} {
		if _, err := parseHistoryID(bad); err == nil {
			t.Errorf("%q: 오류여야 함", bad)
		}
	}
	if id, err := parseHistoryID("501"); err != nil || id != 501 {
		t.Errorf("501: %d %v", id, err)
	}
}

// RSI R1: --find 는 표기가 다른 같은 기호(~·-, ㆍ·)를 같게 본다
func TestNormFindName(t *testing.T) {
	if !strings.Contains(normFindName("15 - 29세"), normFindName("15~29")) {
		t.Error("15~29 가 '15 - 29세' 에 안 맞음")
	}
	if normFindName("광업ㆍ제조업") != normFindName("광업 · 제조업") {
		t.Error("가운뎃점 표기 차이")
	}
}

// RSI R1: 못 찾으면 오류(종료 코드 1)
func TestFindNoMatchIsError(t *testing.T) {
	rows := []api.CodeRow{{Flag: "--class1", Code: "75", Name: "15 - 29세", Level: 1}}
	if err := printCodeRows(rows, "없는이름", "", 0, "", "json"); !errors.Is(err, errNoCodeMatch) {
		t.Errorf("err = %v", err)
	}
}

// RSI R1: -cN 으로 콕 집은 코드 중 결과에 없는 코드 (ALL·* 은 안 따짐)
func TestMissingClasses(t *testing.T) {
	rows := []api.DataRow{{C1: "11", C2: "0"}}
	got := missingClasses([]string{"11+24", "ALL", "1*"}, rows)
	if len(got) != 1 || got[0] != "-c1 24" {
		t.Errorf("got %v", got)
	}
	if got := missingClasses([]string{"24"}, nil); got != nil {
		t.Errorf("결과 0행은 따지지 않음: %v", got)
	}
}

// RSI R1: 큰 축 요약은 한도 안에서 최상위부터 단계별로 펼친다
func TestSummaryLevels(t *testing.T) {
	var g []api.MetaResult
	g = append(g, api.MetaResult{ItmID: "0"}, api.MetaResult{ItmID: "C"})
	for i := 0; i < 30; i++ {
		g = append(g, api.MetaResult{ItmID: "C" + string(rune('A'+i)), UpItmID: "C"})
	}
	for i := 0; i < 60; i++ {
		g = append(g, api.MetaResult{ItmID: "D" + string(rune('A'+i)), UpItmID: "CA"})
	}
	got := summaryLevels(g)
	if len(got) != 32 || got[0].ItmID != "0" {
		t.Errorf("len=%d", len(got))
	}
}

// RSI R2: [21]/[30] 뒤 진단 — 표에 없는 코드를 짚고, 다 있으면 조합·시점 문제라고 알린다
func TestDiagnoseCodes(t *testing.T) {
	sum := &api.MetaSummaryResult{
		Classifications: []api.MetaResult{{ObjID: "A", ObjNM: "시도", ObjIDSN: "1", ItmID: "00"}},
		Items:           []api.MetaResult{{ObjID: "ITEM", ItmID: "13103T1"}},
		Periods:         []api.MetaResult{{PrdSe: "월", StrtPrdDe: "2000.12", EndPrdDe: "2026.08"}},
	}
	got := strings.Join(diagnoseCodes(sum, []string{"00"}, "T1", "101", "X"), "\n")
	if !strings.Contains(got, "-i T1") || strings.Contains(got, "-c1") {
		t.Errorf("없는 항목만 짚어야 함: %s", got)
	}
	got = strings.Join(diagnoseCodes(sum, []string{"ALL"}, "13103T1", "101", "X"), "\n")
	if !strings.Contains(got, "모두 이 표에 있습니다") || !strings.Contains(got, "M 2000.12~2026.08") {
		t.Errorf("조합 안내: %s", got)
	}
}

// RSI R3: 콕 집은 코드의 이 표 이름을 보여 준다 (형제 표에서 같은 코드가 다른 뜻)
func TestEchoCodes(t *testing.T) {
	sum := &api.MetaSummaryResult{
		Classifications: []api.MetaResult{{ObjID: "C", ObjNM: "학교현황별", ObjIDSN: "1", ItmID: "C_1", ItmNM: "직원수"}},
		Items:           []api.MetaResult{{ObjID: "ITEM", ItmID: "T1", ItmNM: "값"}},
	}
	if got := echoCodes(sum, []string{"C_1"}, "ALL"); got != "조회한 코드: -c1 C_1=직원수" {
		t.Errorf("got %q", got)
	}
	if got := echoCodes(sum, []string{"ALL"}, "ALL"); got != "" {
		t.Errorf("ALL 은 안 보여 줌: %q", got)
	}
}

// 리뷰: 대조한 코드가 없으면(빈 메타·ALL 뿐) 진단 결론을 내지 않는다
func TestDiagnoseCodesNothingChecked(t *testing.T) {
	if got := diagnoseCodes(&api.MetaSummaryResult{}, []string{"11"}, "T1", "101", "X"); got != nil {
		t.Errorf("빈 메타: %v", got)
	}
}

// 리뷰: 부모가 축에 없는 코드도 최상위, 상한 50 보장
func TestSummaryLevelsOrphanTop(t *testing.T) {
	var g []api.MetaResult
	for i := 0; i < 10; i++ {
		g = append(g, api.MetaResult{ItmID: fmt.Sprintf("P%d", i), UpItmID: "ROOT"})
		for j := 0; j < 10; j++ {
			g = append(g, api.MetaResult{ItmID: fmt.Sprintf("P%dK%d", i, j), UpItmID: fmt.Sprintf("P%d", i)})
		}
	}
	got := summaryLevels(g)
	if len(got) != 10 || got[0].ItmID != "P0" {
		t.Errorf("최상위 10개만 기대, len=%d", len(got))
	}
	dup := make([]api.MetaResult, 60) // ItmID 가 모두 빈 값
	if got := summaryLevels(dup); len(got) > summaryAxisMax {
		t.Errorf("상한 넘음: %d", len(got))
	}
}

// RSI R4: --sort/--top — 숫자 아닌 값은 맨 뒤, 같은 값은 원래 순서
func TestSortTop(t *testing.T) {
	rows := []api.DataRow{{C1: "a", DT: "5"}, {C1: "b", DT: "-"}, {C1: "c", DT: "1,200"}, {C1: "d", DT: "5"}}
	got := sortTop(append([]api.DataRow{}, rows...), true, 3)
	if len(got) != 3 || got[0].C1 != "c" || got[1].C1 != "a" || got[2].C1 != "d" {
		t.Errorf("desc: %+v", got)
	}
	got = sortTop(append([]api.DataRow{}, rows...), false, 0)
	if got[0].C1 != "a" || got[3].C1 != "b" {
		t.Errorf("asc: %+v", got)
	}
}

// 리뷰: 축 머리 -cN 은 축 ID 기준 — 이름이 같은 축 둘도 번호가 다르다
func TestSummaryAxisFlagsByObjID(t *testing.T) {
	sum := &api.MetaSummaryResult{Classifications: []api.MetaResult{
		{ObjID: "A", ObjNM: "구분", ObjIDSN: "1", ItmID: "1"},
		{ObjID: "B", ObjNM: "구분", ObjIDSN: "2", ItmID: "2"},
	}}
	r, w, _ := os.Pipe()
	old := os.Stdout
	os.Stdout = w
	displayMetaSummary(sum, "101", "X")
	w.Close()
	os.Stdout = old
	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	out := string(buf[:n])
	if !strings.Contains(out, "-c1 ← 분류명: 구분  (분류ID A)") || !strings.Contains(out, "-c2 ← 분류명: 구분  (분류ID B)") {
		t.Errorf("축 번호:\n%s", out)
	}
}

// RSI R5: 요청 주기보다 더 최근까지 있는 주기 안내
func TestNewerCycle(t *testing.T) {
	sum := &api.MetaSummaryResult{Periods: []api.MetaResult{
		{PrdSe: "월", EndPrdDe: "2026.09"}, {PrdSe: "년", EndPrdDe: "2025"},
	}}
	if got := newerCycle(sum, "Y"); !strings.Contains(got, "-p M") {
		t.Errorf("Y: %q", got)
	}
	if got := newerCycle(sum, "M"); got != "" {
		t.Errorf("M 은 이미 최신: %q", got)
	}
}

// RSI R5: --under 가 가장 아래 코드면 '하위 없음', 없는 코드면 '없는 코드'
func TestUnderLeafMessage(t *testing.T) {
	rows := []api.CodeRow{{Flag: "--class1", Code: "11110", Name: "종로구", Level: 2, ParentCode: "11"}}
	if err := printCodeRows(rows, "", "11110", 0, "", "table"); err == nil || !strings.Contains(err.Error(), "하위 코드가 없습니다") {
		t.Errorf("leaf: %v", err)
	}
	if err := printCodeRows(rows, "", "99", 0, "", "table"); err == nil || !strings.Contains(err.Error(), "이 표에 없습니다") {
		t.Errorf("missing: %v", err)
	}
}

// RSI R5: 개편으로 중간에 끊긴 옛 코드
func TestCodesEndedEarly(t *testing.T) {
	rows := []api.DataRow{{C1: "46110", PrdDe: "202606"}, {C1: "12110", PrdDe: "202607"}, {C1: "11", PrdDe: "202607"}}
	got := codesEndedEarly([]string{"46110+11"}, rows)
	if len(got) != 1 || got[0] != "-c1 46110(~202606)" {
		t.Errorf("got %v", got)
	}
	if got := codesEndedEarly([]string{"ALL"}, rows); got != nil {
		t.Errorf("ALL: %v", got)
	}
}

// RSI R5: 여러 기관 검색 결과는 번갈아 합친다
func TestInterleave(t *testing.T) {
	a := []api.SearchResult{{TblID: "a1"}, {TblID: "a2"}, {TblID: "a3"}}
	b := []api.SearchResult{{TblID: "b1"}}
	got := interleave([][]api.SearchResult{a, b})
	ids := []string{}
	for _, r := range got {
		ids = append(ids, r.TblID)
	}
	if strings.Join(ids, ",") != "a1,b1,a2,a3" {
		t.Errorf("got %v", ids)
	}
}

// RSI R6: --change — 계열별 yoy(연도-1 같은 시점)·prev(직전 시점), 소수 자리 유지
func TestComputeChanges(t *testing.T) {
	rows := []api.DataRow{
		{C1: "0", ItmID: "T", PrdDe: "202508", DT: "28967"}, {C1: "0", ItmID: "T", PrdDe: "202607", DT: "29136.1"},
		{C1: "0", ItmID: "T", PrdDe: "202608", DT: "29151"}, {C1: "1", ItmID: "T", PrdDe: "202608", DT: "5"},
	}
	yoy := computeChanges(rows, true)
	if c := yoy[rowKey(rows[2])]; c[0] != "184" || c[1] != "0.64" {
		t.Errorf("yoy: %v", c)
	}
	if _, ok := yoy[rowKey(rows[3])]; ok {
		t.Error("다른 계열과 비교하면 안 됨")
	}
	prev := computeChanges(rows, false)
	if c := prev[rowKey(rows[2])]; c[0] != "14.9" {
		t.Errorf("prev: %v", c)
	}
}

// RSI R6: 표에 있는 주기 (연간 A = Y, 반기 H = S)
func TestCyclesOf(t *testing.T) {
	have := cyclesOf(&api.MetaSummaryResult{Periods: []api.MetaResult{{PrdSe: "월"}, {PrdSe: "반기"}, {PrdSe: "3년"}}})
	if !have["M"] || !have["S"] || !have["H"] || !have["F"] || have["Y"] {
		t.Errorf("%v", have)
	}
}

// RSI R7: --no-total — 여러 코드 축의 합계 행만 빼고, 고정 축은 건드리지 않는다
func TestDropTotals(t *testing.T) {
	rows := []api.DataRow{
		{C1: "00", C1NM: "전  국", C2: "0", C2NM: "계"}, {C1: "11", C1NM: "서울특별시", C2: "0", C2NM: "계"},
		{C1: "0", C1NM: "전체 산업", C2: "0", C2NM: "계"},
	}
	got := dropTotals(rows, []string{"ALL", "0"})
	if len(got) != 1 || got[0].C1 != "11" {
		t.Errorf("got %+v", got)
	}
}

// RSI R7: 키 표시는 뒤 4자만, 짧은 키는 전부 가림
func TestMaskKey(t *testing.T) {
	if got := maskKey("abcdefghijklmnop1234"); got != "…1234" {
		t.Errorf("got %q", got)
	}
	if got := maskKey("short"); got != "****" {
		t.Errorf("got %q", got)
	}
}

// RSI R7: ex 의 HTML 엔티티 (두 번 인코딩 포함)
func TestUnescapeText(t *testing.T) {
	// 엔티티는 글자로, 영문 HTML 태그는 지우고, 한글 꺾쇠(<참고>)는 남긴다
	if got := unescapeText("a &amp;lt;b&amp;gt;굵게&lt;/b&gt; &lt;참고&gt;"); got != "a 굵게 <참고>" {
		t.Errorf("got %q", got)
	}
}

// RSI R8: 마지막 시점 값이 모두 비면 그 시점 (X 는 값이 있는 것)
func TestEmptyLatest(t *testing.T) {
	if got := emptyLatest([]api.DataRow{{PrdDe: "2024", DT: "80.8"}, {PrdDe: "2025", DT: "-"}}); got != "2025" {
		t.Errorf("got %q", got)
	}
	if got := emptyLatest([]api.DataRow{{PrdDe: "2025", DT: "X"}, {PrdDe: "2025", DT: "-"}}); got != "" {
		t.Errorf("X: %q", got)
	}
}

// RSI R8: --level 이 없는 단계면 있는 단계를 알려 준다
func TestNoMatchLevels(t *testing.T) {
	rows := []api.CodeRow{{Flag: "--class1", Axis: "시도별", Code: "00", Level: 1}}
	if err := noMatch(rows, "", 2, ""); err == nil || !strings.Contains(err.Error(), "1단계 1개") {
		t.Errorf("%v", err)
	}
}

// RSI R9: 축 번호가 2부터인 표 — 열은 C2, 누락 검사도 번호 기준, 1번 축 없음
func TestAxisNumberFromSN(t *testing.T) {
	sum := &api.MetaSummaryResult{Classifications: []api.MetaResult{{ObjID: "B", ObjNM: "분배지표별", ObjIDSN: "2", ItmID: "10"}}}
	cm := sum.BuildColumnMeta()
	if cm.Columns[0].Key != "C2_NM" {
		t.Errorf("열 키: %s", cm.Columns[0].Key)
	}
	if hasAxis(sum, 1) || !hasAxis(sum, 2) {
		t.Error("hasAxis")
	}
	if got := missingClassFlags(sum, []string{"ALL", ""}); len(got) != 1 || got[0] != "--class2(분배지표별)" {
		t.Errorf("missing: %v", got)
	}
	if got := missingClassFlags(sum, []string{"", "10"}); len(got) != 0 {
		t.Errorf("1번 축은 없으니 요구하지 않음: %v", got)
	}
	d := strings.Join(diagnoseCodes(sum, []string{"90"}, "", "101", "X"), "\n")
	if !strings.Contains(d, "그 번호의 축이 없음") {
		t.Errorf("diagnose: %s", d)
	}
}

// RSI R10: 긴 코드의 끝만 쓴 경우 진짜 코드를 제안
func TestDiagnoseSuffixHint(t *testing.T) {
	sum := &api.MetaSummaryResult{Classifications: []api.MetaResult{{ObjID: "B", ObjNM: "월", ObjIDSN: "1", ItmID: "13102112880M00"}}}
	got := strings.Join(diagnoseCodes(sum, []string{"M00"}, "", "132", "X"), "\n")
	if !strings.Contains(got, "혹시 13102112880M00") {
		t.Errorf("%s", got)
	}
}

// RSI R10: 주석에 '누계'가 있는 표
func TestNotesSayCumulative(t *testing.T) {
	if !notesSayCumulative([]map[string]interface{}{{"CMMT_DC": "당해년도 해당 월까지의 누계"}}) {
		t.Error("누계 못 찾음")
	}
	if notesSayCumulative([]map[string]interface{}{{"CMMT_DC": "잠정치"}}) {
		t.Error("오탐")
	}
}

// RSI R11: 연간 마지막 시점이 월간 끝과 같은 해이고 12월이 아니면 부분 연도 안내
func TestPartialYear(t *testing.T) {
	sum := &api.MetaSummaryResult{Periods: []api.MetaResult{{PrdSe: "월", EndPrdDe: "2026.08"}, {PrdSe: "년", EndPrdDe: "2026"}}}
	if got := partialYear(sum, "Y", []api.DataRow{{PrdDe: "2026"}}); !strings.Contains(got, "2026") {
		t.Errorf("got %q", got)
	}
	if got := partialYear(sum, "Y", []api.DataRow{{PrdDe: "2025"}}); got != "" {
		t.Errorf("2025 는 완결: %q", got)
	}
	q := &api.MetaSummaryResult{Periods: []api.MetaResult{{PrdSe: "분기", EndPrdDe: "2026 2/4"}}}
	if got := partialYear(q, "Y", []api.DataRow{{PrdDe: "2026"}}); got == "" {
		t.Error("분기 2/4 면 부분 연도")
	}
}

// RSI R12: --ratio — 같은 축 두 코드, 다른 축·시점이 같은 행끼리
func TestComputeRatios(t *testing.T) {
	rows := []api.DataRow{
		{C1: "26", ItmID: "T2", PrdDe: "2024", DT: "568107"}, {C1: "26", ItmID: "T1", PrdDe: "2024", DT: "116775"},
		{C1: "11", ItmID: "T2", PrdDe: "2024", DT: "100"}, {C1: "11", ItmID: "T1", PrdDe: "2024", DT: "50"},
	}
	got, err := computeRatios(rows, "T2", "T1")
	if err != nil || got[rowKey(rows[0])] != "486.50" || got[rowKey(rows[2])] != "200.00" {
		t.Errorf("%v %v", got, err)
	}
	if _, err := computeRatios(rows, "T9", "T1"); err == nil {
		t.Error("없는 코드는 오류")
	}
}

// RSI R12: --top 경계 동률은 함께
func TestSortTopTies(t *testing.T) {
	rows := []api.DataRow{{C1: "a", DT: "3.4"}, {C1: "b", DT: "3.3"}, {C1: "c", DT: "3.3"}, {C1: "d", DT: "3.2"}}
	if got := sortTop(rows, true, 2); len(got) != 3 {
		t.Errorf("동률 포함 3행 기대: %d", len(got))
	}
}

// RSI R13: --ratio ALL/00 — 그 축 모든 행을 분모 대비 %
func TestComputeRatiosAll(t *testing.T) {
	rows := []api.DataRow{{C1: "00", ItmID: "T1", PrdDe: "2024", DT: "200"}, {C1: "26", ItmID: "T1", PrdDe: "2024", DT: "300"}}
	got, err := computeRatios(rows, "ALL", "00")
	if err != nil || got[rowKey(rows[1])] != "150.00" || got[rowKey(rows[0])] != "100.00" {
		t.Errorf("%v %v", got, err)
	}
}

// 리뷰2: --top 경계 값이 숫자가 아니면 늘리지 않고, 숫자는 값으로 비교
func TestSortTopBoundary(t *testing.T) {
	rows := []api.DataRow{{DT: "5"}, {DT: "-"}, {DT: "-"}, {DT: "-"}}
	if got := sortTop(rows, true, 2); len(got) != 2 {
		t.Errorf("비숫자 경계: %d", len(got))
	}
	rows = []api.DataRow{{DT: "3.30"}, {DT: "3.3"}, {DT: "1"}}
	if got := sortTop(rows, true, 1); len(got) != 2 {
		t.Errorf("3.30=3.3 동률: %d", len(got))
	}
}

// 리뷰2: SN 이 겹치거나 비면 쓰이지 않은 번호
func TestAxesCollision(t *testing.T) {
	sum := &api.MetaSummaryResult{Classifications: []api.MetaResult{
		{ObjID: "A", ObjIDSN: "2"}, {ObjID: "B"}, {ObjID: "C", ObjIDSN: "2"}, {ObjID: ""},
	}}
	ax := sum.Axes()
	got := map[int]bool{}
	for _, a := range ax {
		if got[a.N] {
			t.Errorf("번호 중복 %d", a.N)
		}
		got[a.N] = true
	}
	if len(ax) != 3 || ax[0].N != 2 {
		t.Errorf("%+v", ax)
	}
}

// 리뷰2: 쉼표·+ 섞인 코드, 공백
func TestJoinCodes(t *testing.T) {
	if got := joinCodes("T1 ,T2+T3"); got != "T1+T2+T3" {
		t.Errorf("%q", got)
	}
	if got := joinCodes("T1+T2"); got != "T1+T2" {
		t.Errorf("쉼표 없으면 그대로: %q", got)
	}
}

// 리뷰2: 반기 부분 연도, 6자리 분기 끝
func TestPartialYearHalfAndNumeric(t *testing.T) {
	h := &api.MetaSummaryResult{Periods: []api.MetaResult{{PrdSe: "반기", EndPrdDe: "2026 1/2"}}}
	if partialYear(h, "Y", []api.DataRow{{PrdDe: "2026"}}) == "" {
		t.Error("반기 1/2 면 부분 연도")
	}
	q := &api.MetaSummaryResult{Periods: []api.MetaResult{{PrdSe: "분기", EndPrdDe: "202604"}}}
	if got := partialYear(q, "Y", []api.DataRow{{PrdDe: "2026"}}); got != "" {
		t.Errorf("202604 는 4분기 — 완결: %q", got)
	}
}

// 리뷰2: 합계 이름 — 전산업 잡고, '전체가구 중 1인가구' 는 안 잡음
func TestTotalNameRegex(t *testing.T) {
	for _, n := range []string{"전산업", "전산업(A~U)", "합계(전국)", "전체", "전체가구", "계"} {
		if !totalName.MatchString(n) {
			t.Errorf("못 잡음: %s", n)
		}
	}
	for _, n := range []string{"전체가구중1인가구", "서울특별시", "제조업"} {
		if totalName.MatchString(n) {
			t.Errorf("오탐: %s", n)
		}
	}
}
