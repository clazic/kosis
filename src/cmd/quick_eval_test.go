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
