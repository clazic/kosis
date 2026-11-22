package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const attachShipped = `
rules:
  - id: search.terms
    topic: search
    when: 검색할 때
    do: 짧게
    verified: "2026-06-15"
    evidence: 실측
tables:
  - id: "116/DT_X_2086"
    name: 미분양주택현황
    periods: [Y]
    facts:
      - 연간만 있다. 월 단위 요청은 불가
      - 월간 전국은 116/DT_X_2080
    verified: "2026-07-20"
  - id: "142/DT_X_A008"
    name: 시군구별 기업수 (중소기업기본통계)
    periods: [Y]
    facts:
      - 소상공인 기업수 2023 7,906,861. 표 이름에 '소상공인'이 없어 검색에 안 걸린다
    verified: "2026-10-13"
  - id: "142/DT_X_SURVEY"
    name: 소상공인 실태조사 기업체 수
    facts:
      - 표본조사
    verified: "2026-10-13"
  - id: "101/DT_X_MANY"
    name: 사실이 많은 표
    facts: [f1, f2, f3, f4, f5, f6, f7, f8, f9, f10]
    verified: "2026-10-13"
concepts:
  - id: own
    title: 자가보유율 · 자가점유율
    terms: ["자가보유율", "자가 점유율"]
    sources:
      - name: 보유율
        table: "101/DT_X_OWN"
        code: "-i T1"
        value: "2024 56.9%"
      - name: 점유율
        table: "116/DT_X_2086"
        value: "2024 58.4%"
    tell: 정의를 밝힌다
    evidence: 실측
    verified: "2026-11-23"
gaps:
  - id: ev-reg
    title: 시도별 전기차 등록 대수
    missing: 연료별 자동차 등록 대수 표를 찾지 못함
    evidence: s 전기차
    verified: "2026-10-13"
`

// 배포 규칙 파일 + 빈 HOME(로컬 규칙·스킬 폴더 없음)을 차린다. 로컬 규칙 디렉토리를 돌려준다.
func setupAttach(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	f := filepath.Join(home, "rules.yaml")
	if err := os.WriteFile(f, []byte(attachShipped), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KOSIS_RULES_FILE", f)
	dir := filepath.Join(home, ".kosis", "rules.d")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeLocal(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestTableRuleNotes(t *testing.T) {
	dir := setupAttach(t)
	n := tableRuleNotes("116", "DT_X_2086")
	if len(n.Lines) != 3 || n.More != 0 || !strings.Contains(n.Lines[1], "규칙(배포 2026-07-20): 연간만 있다") {
		t.Fatalf("배포 facts: %+v", n)
	}
	if got := tableRuleNotes("999", "NOPE"); len(got.Lines) != 0 {
		t.Errorf("규칙 없는 표: %+v", got)
	}
	if got := tableRuleNotes("", ""); len(got.Lines) != 0 {
		t.Errorf("빈 표 ID: %+v", got)
	}

	// 로컬 규칙이 앞에 오고, 같은 fact 는 한 번만, 틀림 표시 항목은 facts 대신 정정 안내
	writeLocal(t, dir, "20261101-aaaaaaaa.yaml", `kind: table
title: 116/DT_X_2086 로컬
verified: "2026-11-01"
evidence: 실측
table: "116/DT_X_2086"
facts:
  - 2026.07 부터 광주·전남 코드가 바뀜
  - 연간만 있다. 월 단위 요청은 불가
`)
	writeLocal(t, dir, "20261102-bbbbbbbb.yaml", `kind: table
title: 116/DT_X_2086 옛 설명
verified: "2026-06-01"
evidence: 실측
status: wrong
table: "116/DT_X_2086"
facts:
  - 월간도 있다
`)
	n = tableRuleNotes("116", "DT_X_2086")
	joined := strings.Join(n.Lines, "\n")
	if !strings.HasPrefix(n.Lines[1], "규칙(로컬 2026-11-01): 2026.07 부터") {
		t.Errorf("로컬 먼저: %q", n.Lines[1])
	}
	if strings.Count(joined, "연간만 있다") != 1 {
		t.Errorf("같은 fact 중복: %s", joined)
	}
	if strings.Contains(joined, "월간도 있다") || !strings.Contains(joined, "틀린 것으로 표시") {
		t.Errorf("틀림 항목: %s", joined)
	}

	// facts 가 많으면 8줄까지, 나머지는 More
	n = tableRuleNotes("101", "DT_X_MANY")
	if countFacts(n.Lines) != maxTableFacts || n.More != 2 {
		t.Errorf("상한: %d줄, More %d", countFacts(n.Lines), n.More)
	}
	if lines := MCPTableRules("101", "DT_X_MANY"); !strings.Contains(lines[len(lines)-1], "kosis_rules id=101/DT_X_MANY") {
		t.Errorf("MCP 더 보기: %q", lines[len(lines)-1])
	}
}

func TestSearchRuleNotes(t *testing.T) {
	setupAttach(t)
	// 표 이름에 검색어가 없어도 facts 에 있으면 잡고, 이름에 있는 표가 앞에 온다
	lines := searchRuleNotes("소상공인")
	if len(lines) != 2 || !strings.Contains(lines[0], "142/DT_X_SURVEY") || !strings.Contains(lines[1], "142/DT_X_A008") {
		t.Fatalf("소상공인: %q", lines)
	}
	if !strings.Contains(lines[1], "7,906,861") || !strings.Contains(lines[1], "[주기 Y]") {
		t.Errorf("fact·주기: %q", lines[1])
	}
	// 못 찾은 기록: 낱말이 모두 있어야 한다
	lines = searchRuleNotes("전기차 등록")
	if len(lines) != 1 || !strings.Contains(lines[0], "못 찾은 기록") || !strings.Contains(lines[0], "--id ev-reg") {
		t.Errorf("gap: %q", lines)
	}
	if lines := searchRuleNotes("전기차 수출"); len(lines) != 0 {
		t.Errorf("낱말 하나만 맞음: %q", lines)
	}
	for _, kw := range []string{"", "가", "없는말없는말"} {
		if lines := searchRuleNotes(kw); len(lines) != 0 {
			t.Errorf("%q: %q", kw, lines)
		}
	}
	if lines := MCPSearchRules("전기차 등록"); !strings.Contains(lines[0], "kosis_rules id=ev-reg") {
		t.Errorf("MCP 표기: %q", lines)
	}
}

// 검증된 표가 많으면 5개까지 싣고 나머지는 수로 알린다.
func TestSearchRuleNotesCap(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	var b strings.Builder
	b.WriteString("tables:\n")
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&b, "  - id: \"101/DT_C%d\"\n    name: 인구 표 %d\n    verified: \"2026-10-13\"\n", i, i)
	}
	f := filepath.Join(home, "rules.yaml")
	if err := os.WriteFile(f, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KOSIS_RULES_FILE", f)
	lines := searchRuleNotes("인구")
	if len(lines) != maxSearchTables+1 || !strings.Contains(lines[maxSearchTables], "3개 더") {
		t.Errorf("상한: %q", lines)
	}
}

// 스킬 폴더도 KOSIS_RULES_FILE 도 없으면 바이너리에 내장된 규칙을 쓴다 (kosis-mcp 서버 환경).
func TestRulesFallBackToEmbedded(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("KOSIS_RULES_FILE", "")
	t.Chdir(t.TempDir()) // cwd 아래 프로젝트 스킬 폴더도 없게
	if n := tableRuleNotes("116", "DT_MLTM_2086"); len(n.Lines) == 0 || !strings.Contains(strings.Join(n.Lines, " "), "연간만 있다") {
		t.Errorf("내장 규칙: %+v", n)
	}
	rows, err := MCPRules("", "116/DT_MLTM_2086", "", "", "")
	if err != nil || len(rows) != 1 || rows[0]["source"] != "shipped" {
		t.Errorf("MCPRules: %v %v", rows, err)
	}
}

func TestConceptNotes(t *testing.T) {
	setupAttach(t)
	// 검색어에 헷갈리는 지표 이름이 들면(공백 무시) 맨 앞에 혼동 주의
	for _, kw := range []string{"자가보유율", "전국 자가점유율 추이"} {
		lines := searchRuleNotes(kw)
		if len(lines) == 0 || !strings.HasPrefix(lines[0], "규칙 혼동 주의(2026-11-23): 자가보유율 · 자가점유율 — ① 보유율: 101/DT_X_OWN -i T1 = 2024 56.9% / ② 점유율: 116/DT_X_2086 = 2024 58.4% — 정의를 밝힌다") {
			t.Errorf("%q: %q", kw, lines)
		}
	}
	if lines := searchRuleNotes("자가"); len(lines) != 0 {
		t.Errorf("용어 일부만: %q", lines)
	}
	// 그 표를 조회하면 이 표의 지표에 ▶, facts 상한과는 따로 센다
	n := tableRuleNotes("116", "DT_X_2086")
	if len(n.Lines) != 3 || !strings.Contains(n.Lines[0], "▶② 점유율: 116/DT_X_2086") || strings.Contains(n.Lines[0], "▶①") {
		t.Errorf("표 조회: %q", n.Lines)
	}
	if countFacts(n.Lines) != 2 {
		t.Errorf("혼동 주의는 facts 로 세지 않는다: %d", countFacts(n.Lines))
	}
	if n := tableRuleNotes("101", "DT_X_OWN"); len(n.Lines) != 1 || !strings.Contains(n.Lines[0], "▶① 보유율") {
		t.Errorf("규칙 없는 표도 혼동 주의는 나온다: %q", n.Lines)
	}
}
