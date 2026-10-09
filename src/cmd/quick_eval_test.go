package cmd

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/learn"
	"github.com/clazic/kosis/internal/nlp"
)

func TestExpandYears(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	if got := expandYears("--start {Y-2} --end {Y}|{Y+1}", now); got != "--start 2024 --end 2026|2027" {
		t.Errorf("got %q", got)
	}
}

func TestValueMatchesAndScore(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	plan := &aiPlan{
		Table:  "101/A",
		Intent: &nlp.Intent{Period: nlp.PeriodSpec{Kind: "latest", Latest: 3, Unit: "Y"}},
		Picks: []*aiPick{
			{Axis: "A", Code: "21040", Name: "북구"}, // 부산 북구
			{Axis: periodAxis, Code: "Y", Name: "년"},
		},
	}
	if valueMatches("북구@26", plan.Picks) {
		t.Error("코드 접두가 다르면(부산 21) 울산 북구로 인정하면 안 됨")
	}
	if !valueMatches("남구|북구@21", plan.Picks) {
		t.Error("대안 중 하나가 맞으면 인정")
	}
	c := evalCase{Tables: []string{"101/A"}, Values: []string{"북구@26"}, Unit: "Y", Period: "--latest 5|--latest 3"}
	r := scoreEval(c, plan, now)
	if r.Table != 40 || r.Values != 0 || r.Unit != 10 || r.Period != 10 || r.Total != 60 {
		t.Errorf("score = %+v", r)
	}
}

// 다중 항목 질문에서 이미 다른 정답을 만족하는 축을 덮어쓰면 라운드마다 뒤집힌다.
func TestSimulateCorrectionNoFlipFlop(t *testing.T) {
	config.SetConfigDirForTesting(t.TempDir())
	defer config.SetConfigDirForTesting("")
	store := learn.LoadFrom(t.TempDir() + "/l.yaml")
	pk := &aiPick{Axis: "A", Cond: "", Code: "11", Name: "출생아수(명)", Source: srcJev,
		Options: map[string]string{"11": "출생아수(명)", "12": "사망자수(명)", "31": "출생성비(명)"}}
	plan := &aiPlan{Table: "101/T", Topic: "t|", Intent: &nlp.Intent{}, Picks: []*aiPick{pk}}
	c := evalCase{Tables: []string{"101/T"}, Values: []string{"출생", "사망"}}
	r := scoreEval(c, plan, time.Now())
	simulateCorrection(store, c, plan, &r)
	if code, ok := store.AxisPick("101/T", "A", ""); !ok || code != "11" {
		t.Errorf("이미 '출생'을 만족하는 축은 '사망'으로 덮어쓰지 않아야 함: %q %v (learned %v)", code, ok, r.Learned)
	}
}

func TestTruncWidth(t *testing.T) {
	if got := truncWidth("가나다라마", 6); got != "가나…" {
		t.Errorf("got %q", got)
	}
	if got := truncWidth("가나다", 6); got != "가나다" {
		t.Errorf("폭 안이면 그대로: %q", got)
	}
	if w := len([]rune(padWidth("가a", 6))); w != 5 { // 가(2)+a(1)+공백3
		t.Errorf("pad = %q", padWidth("가a", 6))
	}
	_ = strings.TrimSpace
}

func TestRequestedFreq(t *testing.T) {
	if requestedFreq(nlp.PeriodSpec{Unit: "Y", Freq: "Q"}) != "Q" || requestedFreq(nlp.PeriodSpec{Unit: "M"}) != "M" {
		t.Error("freq 가 있으면 freq, 없으면 unit")
	}
}

func TestLearnedNameAll(t *testing.T) {
	opts := map[string]string{"26": "울산광역시"}
	if n, ok := learnedName(opts, allCode); !ok || n != "전체" {
		t.Error("ALL 은 메타에 없어도 유효해야 함")
	}
	if _, ok := learnedName(opts, "99"); ok {
		t.Error("메타에 없는 코드는 무효")
	}
}

func TestChildrenCodeAndLearnedName(t *testing.T) {
	kids := []string{"Z_2601", "Z_2602", "Z_2631"}
	code, ok := childrenCode("Z_26", kids)
	if !ok || code != "Z_2601+Z_2602+Z_2631" {
		t.Errorf("10개 이하는 + 나열: %q", code)
	}
	var many []string
	for i := 0; i < 12; i++ {
		many = append(many, fmt.Sprintf("Z_26%02d", i))
	}
	if code, ok := childrenCode("Z_26", many); !ok || code != "Z_26*" {
		t.Errorf("10개 초과·공통 접두면 접두*: %q", code)
	}
	if _, ok := childrenCode("P", append(many, "Q1")); ok {
		t.Error("접두사로 묶을 수 없으면 제공하지 않아야 함")
	}

	opts := map[string]string{"Z_26": "울산광역시", "Z_2601": "중구", "Z_2602": "남구", "Z_2631": "울주군"}
	if n, ok := learnedName(opts, "Z_2601+Z_2631"); !ok || n != "중구·울주군" {
		t.Errorf("+ 묶음: %q %v", n, ok)
	}
	if _, ok := learnedName(opts, "Z_2601+Z_9999"); ok {
		t.Error("없는 코드가 섞이면 무효")
	}
	if n, ok := learnedName(opts, "Z_26*"); !ok || n != "울산광역시 하위 전체" {
		t.Errorf("접두*: %q", n)
	}
}

func TestKeywordCandidates(t *testing.T) {
	got := keywordCandidates("구군별 업종별 종사자수 와 생산액", "")
	want := []string{"구군별 업종별 종사자수 와 생산액", "종사자수 생산액", "종사자수", "생산액"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("got %q", got)
	}
}

func TestMergeTermAnswers(t *testing.T) {
	opts := map[string]string{"21": "부산광역시", "22": "대구광역시", "00": "계"}
	terms := []string{"부산", "대구"}

	code, name, conf, ok := mergeTermAnswers(opts, terms, []termAnswer{
		{term: "대구", code: "22", conf: 0.9}, {term: "부산", code: "21", conf: 0.8},
	})
	if !ok || code != "21+22" || name != "부산광역시·대구광역시" || conf != 0.8 {
		t.Errorf("요청 순서로 묶고 최소 신뢰도: %q %q %v %v", code, name, conf, ok)
	}
	// 같은 값으로 수렴하거나, 해당 없음·낮은 신뢰도는 묶지 않는다
	for _, tas := range [][]termAnswer{
		{{term: "부산", code: "21", conf: 0.9}, {term: "대구", code: "21", conf: 0.9}},
		{{term: "부산", code: "21", conf: 0.9}, {term: "대구", code: noneOption, conf: 0.9}},
		{{term: "부산", code: "21", conf: 0.9}, {term: "대구", code: "22", conf: 0.3}},
	} {
		if _, _, _, ok := mergeTermAnswers(opts, terms, tas); ok {
			t.Errorf("묶으면 안 됨: %+v", tas)
		}
	}
}

func TestMultiTerms(t *testing.T) {
	if got := multiTerms(&nlp.Intent{Conditions: []string{"울산"}, Measures: []string{"종사자수"}}); len(got) != 0 {
		t.Errorf("대상이 하나씩이면 대상별 질문 없음: %v", got)
	}
	got := multiTerms(&nlp.Intent{Conditions: []string{"부산", "대구"}, Measures: []string{"종사자수", "생산액"}})
	if strings.Join(got, ",") != "부산,대구,종사자수,생산액" {
		t.Errorf("got %v", got)
	}
}

func TestTableNameRemembered(t *testing.T) {
	path := t.TempDir() + "/l.yaml"
	s := learn.LoadFrom(path)
	s.TableOK("출생아수|", "101/A")
	s.SetTableName("101/A", "출생아수(시도/시/군/구)")
	s.SetTableName("101/B", "101/B") // ID와 같으면 저장 안 함
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}
	s = learn.LoadFrom(path)
	if s.TableName("101/A") != "출생아수(시도/시/군/구)" || s.TableName("101/B") != "" {
		t.Errorf("이름 저장 실패: %q %q", s.TableName("101/A"), s.TableName("101/B"))
	}
}

// 해석(hermes)이 실행마다 달라도 같은 질문이면 학습을 찾아야 한다.
func TestRequestKeySurvivesInterpretationDrift(t *testing.T) {
	s := learn.LoadFrom(t.TempDir() + "/l.yaml")
	first := &aiPlan{Topic: learn.TopicKey("종사자수 생산액", []string{"울산"}), ReqTopic: requestKey("울산 종사자수와 생산액 2023년")}
	tableOK(s, first, "101/DT_1FS1101")
	pk := &aiPick{Axis: itemAxis, Cond: "울산#종사자수생산액", ReqCond: requestKey("울산 종사자수와 생산액 2023년")}
	axisOK(s, "101/DT_1FS1101", pk, "T02+T04")

	second := &aiPlan{Topic: learn.TopicKey("종사자수와 생산액", []string{"울산광역시"}), ReqTopic: requestKey("울산  종사자수와 생산액 2023년")}
	if got := preferTable(s, second); got != "101/DT_1FS1101" {
		t.Errorf("원문 키로 표를 찾아야 함: %q", got)
	}
	pk2 := &aiPick{Axis: itemAxis, Cond: "울산광역시#종사자수와생산액", ReqCond: second.ReqTopic}
	found := false
	for _, c := range pk2.conds() {
		if code, ok := s.AxisPick("101/DT_1FS1101", itemAxis, c); ok && code == "T02+T04" {
			found = true
		}
	}
	if !found {
		t.Error("원문 키로 항목 학습을 찾아야 함")
	}
}

func TestContainsCode(t *testing.T) {
	cases := []struct {
		combo, code string
		want        bool
	}{
		{"T02+T04", "T02", true}, {"T02+T04", "T01", false}, {"ALL", "x", true},
		{"Z_26*", "Z_2601", true}, {"Z_26*", "Z_21", false}, {"T02", "T02", true},
	}
	for _, c := range cases {
		if got := containsCode(c.combo, c.code); got != c.want {
			t.Errorf("containsCode(%q,%q)=%v", c.combo, c.code, got)
		}
	}
}
