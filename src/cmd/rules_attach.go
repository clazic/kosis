package cmd

import (
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/clazic/kosis/internal/rules"
	"gopkg.in/yaml.v3"
)

// 조회 결과에 그 표·검색어의 규칙을 실어 보낸다 (kosis m·d·s 와 kosis-mcp 의 kosis_meta·kosis_data·kosis_search).
// AI 가 rules.yaml 을 따로 읽지 않아도, 스킬 폴더가 없는 MCP 서버에서도 검증된 지식이 결과와 함께 간다.

const (
	maxTableFacts   = 8 // 표 하나에 붙이는 facts 줄 수
	maxSearchTables = 5 // 검색어에 붙이는 검증된 표 수
	maxSearchGaps   = 3
	maxConcepts     = 2   // 검색어·표 하나에 붙이는 혼동 주의 수
	factCut         = 320 // 검색 결과에 붙이는 fact 한 줄 길이(글자)
)

// rulesForLookup 로컬(더 최신) + 배포 규칙. 배포는 KOSIS_RULES_FILE → 설치된 스킬 폴더 → 바이너리 내장 순.
// 읽지 못하면 빈 목록 (조회 자체를 막지 않는다).
func rulesForLookup() (local, shipped []rules.Entry) {
	shipped, local, _, _, err := loadAllRules()
	if err != nil {
		return nil, nil
	}
	return local, shipped
}

// TableNotes 한 표의 규칙에서 뽑은 줄.
type TableNotes struct {
	Lines []string // "규칙(배포 2026-10-13): 연간만 있다 …" — 로컬 먼저
	More  int      // maxTableFacts 를 넘어 싣지 못한 facts 수
}

// tableRuleNotes 표(ORG/TBL)에 해당하는 tables 항목의 facts. 틀림(status: wrong) 항목은 facts 대신 정정 안내 한 줄.
func tableRuleNotes(orgID, tblID string) TableNotes {
	var out TableNotes
	if orgID == "" || tblID == "" {
		return out
	}
	id := orgID + "/" + tblID
	local, shipped := rulesForLookup()
	seen := map[string]bool{}
	nc := 0
	for _, e := range append(local, shipped...) { // 같은 말로 불리는 다른 지표가 있는 표면 먼저 알린다
		if e.Kind != "concept" || e.Status == "wrong" || nc >= maxConcepts {
			continue
		}
		if line := conceptLineForTable(e, id); line != "" {
			out.Lines = append(out.Lines, line)
			nc++
		}
	}
	for _, e := range append(local, shipped...) {
		if e.Kind != "table" || !strings.EqualFold(bodyString(e, "table"), id) {
			continue
		}
		src := "배포"
		if e.Source == "local" {
			src = "로컬"
		}
		if e.Status == "wrong" {
			out.Lines = append(out.Lines, fmt.Sprintf("규칙 정정(%s): 예전 규칙 %q 은 틀린 것으로 표시됐습니다 — 쓰지 마세요", src, e.Title))
			continue
		}
		tag := src
		if e.Verified != "" {
			tag += " " + e.Verified
		}
		if e.Confidence == "guess" {
			tag += " 추정"
		}
		for _, f := range bodyStrings(e, "facts") {
			key := strings.Join(strings.Fields(f), "")
			if seen[key] {
				continue
			}
			seen[key] = true
			if countFacts(out.Lines) >= maxTableFacts {
				out.More++
				continue
			}
			out.Lines = append(out.Lines, fmt.Sprintf("규칙(%s): %s", tag, f))
		}
	}
	return out
}

func countFacts(lines []string) int {
	n := 0
	for _, l := range lines {
		if strings.HasPrefix(l, "규칙(") {
			n++
		}
	}
	return n
}

// searchRuleNotes 검색어에 맞는 규칙: 예전에 못 찾은 것(gaps)과 검증된 표(tables).
// 검색어의 낱말(2글자 이상)이 모두 들어 있는 항목만. 표 이름에 검색어가 없어도 facts·코드 이름에 있으면 잡는다
// (표 이름에 '소상공인'이 없는 소상공인 기업 수 표처럼 검색으로 안 걸리는 표).
func searchRuleNotes(keyword string) []string {
	var tokens []string
	for _, w := range strings.Fields(keyword) {
		if utf8.RuneCountInString(w) >= 2 {
			tokens = append(tokens, strings.ToLower(w))
		}
	}
	if len(tokens) == 0 {
		return nil
	}
	has := func(text string) bool {
		t := strings.ToLower(strings.Join(strings.Fields(text), ""))
		for _, w := range tokens {
			if !strings.Contains(t, w) {
				return false
			}
		}
		return true
	}
	local, shipped := rulesForLookup()
	var concepts, gaps, tables []string
	squashed := strings.ToLower(strings.Join(strings.Fields(keyword), ""))
	for _, e := range append(local, shipped...) { // 검색어에 헷갈리는 지표 이름이 들었으면 맨 앞에
		if e.Kind != "concept" || e.Status == "wrong" || len(concepts) >= maxConcepts {
			continue
		}
		for _, t := range bodyStrings(e, "terms") {
			if strings.Contains(squashed, strings.ToLower(strings.Join(strings.Fields(t), ""))) {
				concepts = append(concepts, conceptLine(e, ""))
				break
			}
		}
	}
	type hit struct {
		line   string
		inName bool
	}
	var hits []hit
	seenTbl := map[string]bool{}
	for _, e := range append(local, shipped...) {
		if e.Status == "wrong" {
			continue
		}
		switch e.Kind {
		case "gap":
			title := bodyString(e, "title")
			if title == "" {
				title = e.Title
			}
			missing := bodyString(e, "missing")
			if has(title+" "+missing) && len(gaps) < maxSearchGaps {
				gaps = append(gaps, fmt.Sprintf("규칙(못 찾은 기록 %s): %s — %s (kosis rules list --id %s)", e.Verified, title, cutRunes(missing, factCut), bodyString(e, "id")))
			}
		case "table":
			id := bodyString(e, "table")
			if id == "" || seenTbl[id] {
				continue
			}
			name := bodyString(e, "name")
			if name == "" {
				name = e.Title
			}
			body, _ := yaml.Marshal(e.Body)
			if !has(name + " " + string(body)) {
				continue
			}
			seenTbl[id] = true
			fact := ""
			for _, f := range bodyStrings(e, "facts") { // 검색어가 든 fact 를 먼저
				if fact == "" || containsAny(strings.ToLower(f), tokens) {
					fact = f
					if containsAny(strings.ToLower(f), tokens) {
						break
					}
				}
			}
			line := fmt.Sprintf("규칙(검증된 표 %s): %s %s", e.Verified, id, name)
			if p := bodyStrings(e, "periods"); len(p) > 0 {
				line += " [주기 " + strings.Join(p, "·") + "]"
			}
			if fact != "" {
				line += " — " + cutRunes(fact, factCut)
			}
			hits = append(hits, hit{line, has(name)})
		}
	}
	sort.SliceStable(hits, func(i, j int) bool { return hits[i].inName && !hits[j].inName })
	for i, h := range hits {
		if i == maxSearchTables {
			tables = append(tables, fmt.Sprintf("규칙: 검증된 표 %d개 더 — kosis rules list %q --kind table", len(hits)-maxSearchTables, keyword))
			break
		}
		tables = append(tables, h.line)
	}
	return append(concepts, append(gaps, tables...)...)
}

// conceptSource 혼동 쌍의 한 지표.
type conceptSource struct{ name, table, code, value string }

func conceptSources(e rules.Entry) []conceptSource {
	items, _ := e.Body["sources"].([]any)
	var out []conceptSource
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		str := func(k string) string { v, _ := m[k].(string); return strings.Join(strings.Fields(v), " ") }
		out = append(out, conceptSource{str("name"), str("table"), str("code"), str("value")})
	}
	return out
}

// conceptLine "규칙 혼동 주의(날짜): 제목 — ① 무엇: 표 코드 = 값 / ② … — 어떻게 답할지". mark 는 조회 중인 표(그 지표에 ▶).
func conceptLine(e rules.Entry, mark string) string {
	title := bodyString(e, "title")
	if title == "" {
		title = e.Title
	}
	var parts []string
	for i, s := range conceptSources(e) {
		p := fmt.Sprintf("%s %s: %s", string(rune('①'+i)), s.name, s.table)
		if s.code != "" {
			p += " " + s.code
		}
		if s.value != "" {
			p += " = " + s.value
		}
		if mark != "" && strings.EqualFold(s.table, mark) {
			p = "▶" + p
		}
		parts = append(parts, p)
	}
	line := fmt.Sprintf("규칙 혼동 주의(%s): %s — %s", e.Verified, title, strings.Join(parts, " / "))
	if tell := bodyString(e, "tell"); tell != "" {
		line += " — " + strings.Join(strings.Fields(tell), " ")
	}
	return line
}

// conceptLineForTable 이 표가 혼동 쌍의 지표 가운데 하나면 그 줄 (▶ 가 이 표의 지표).
func conceptLineForTable(e rules.Entry, id string) string {
	for _, s := range conceptSources(e) {
		if strings.EqualFold(s.table, id) {
			return conceptLine(e, id)
		}
	}
	return ""
}

func containsAny(text string, words []string) bool {
	for _, w := range words {
		if strings.Contains(text, w) {
			return true
		}
	}
	return false
}

func cutRunes(s string, n int) string {
	s = strings.Join(strings.Fields(s), " ")
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func bodyString(e rules.Entry, k string) string {
	v, _ := e.Body[k].(string)
	return v
}

func bodyStrings(e rules.Entry, k string) []string {
	items, _ := e.Body[k].([]any)
	out := make([]string, 0, len(items))
	for _, it := range items {
		if it != nil {
			out = append(out, strings.Join(strings.Fields(fmt.Sprint(it)), " "))
		}
	}
	return out
}

// ── kosis-mcp 용 ──────────────────────────────────────────────

// MCPTableRules kosis_meta·kosis_data 에 싣는 그 표의 규칙 줄 (더 있으면 마지막 줄에 kosis_rules 안내).
func MCPTableRules(orgID, tblID string) []string {
	n := tableRuleNotes(orgID, tblID)
	lines := n.Lines
	if n.More > 0 {
		lines = append(lines, fmt.Sprintf("규칙 %d줄 더: kosis_rules id=%s/%s", n.More, orgID, tblID))
	}
	return lines
}

// MCPSearchRules kosis_search 에 싣는 검색어 관련 규칙 줄.
func MCPSearchRules(keyword string) []string {
	lines := searchRuleNotes(keyword)
	for i, l := range lines {
		lines[i] = strings.NewReplacer("kosis rules list --id ", "kosis_rules id=", " --kind table", " (kosis_rules kind=table)").Replace(l)
	}
	return lines
}

// MCPRules 규칙 목록 (kosis rules list -f json 과 같은 항목). 선택자는 kosis rules list 와 같다.
func MCPRules(query, id, kind, topic, status string) ([]map[string]any, error) {
	shipped, local, _, _, err := loadAllRules()
	if err != nil {
		return nil, fmt.Errorf("규칙을 읽지 못했습니다: %w", err)
	}
	out := []map[string]any{}
	for _, e := range append(local, shipped...) { // 로컬 먼저 (더 최신)
		if (kind != "" && e.Kind != kind) || !rules.Matches(e, query) || !ruleSelected(e, id, topic, status) {
			continue
		}
		m := map[string]any{"kind": e.Kind, "title": e.Title, "source": e.Source}
		for k, v := range map[string]string{"verified": e.Verified, "evidence": e.Evidence, "confidence": e.Confidence, "status": e.Status} {
			if v != "" {
				m[k] = v
			}
		}
		for k, v := range e.Body {
			m[k] = v
		}
		out = append(out, m)
	}
	return out, nil
}
