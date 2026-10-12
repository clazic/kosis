package cmd

import (
	"errors"
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
