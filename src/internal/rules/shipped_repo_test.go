package rules

import (
	"bytes"
	"os"
	"testing"
)

// 내장 사본이 저장소 원본과 같은지 — 다르면 cd src && make sync-rules.
func TestEmbeddedMatchesRepo(t *testing.T) {
	const path = "../../../references/rules.yaml"
	want, err := os.ReadFile(path)
	if err != nil {
		t.Skip("저장소 references/rules.yaml 없음")
	}
	if !bytes.Equal(want, embeddedRules) {
		t.Fatal("internal/rules/embedded_rules.yaml 이 references/rules.yaml 과 다릅니다 — cd src && make sync-rules")
	}
}

// 내장 규칙이 읽히는지 (스킬 폴더 없이도 표 규칙·gap 을 쓸 수 있어야 한다).
func TestEmbeddedLoads(t *testing.T) {
	entries, err := Embedded()
	if err != nil {
		t.Fatal(err)
	}
	kinds := map[string]int{}
	for _, e := range entries {
		kinds[e.Kind]++
		if e.File != EmbeddedName || e.Source != "shipped" {
			t.Fatalf("출처 표시: %q %q", e.Source, e.File)
		}
	}
	if kinds["table"] == 0 || kinds["rule"] == 0 || kinds["gap"] == 0 {
		t.Errorf("종류별 항목 수: %v", kinds)
	}
}

// 저장소의 배포 rules.yaml 이 읽히고 항목마다 형식이 맞는지 (src 만 따로 받은 환경이면 건너뜀).
func TestRepoShippedRulesValid(t *testing.T) {
	const path = "../../../references/rules.yaml"
	if _, err := os.Stat(path); err != nil {
		t.Skip("저장소 references/rules.yaml 없음")
	}
	entries, err := LoadShipped(path)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, e := range entries {
		if probs := Validate(e); len(probs) > 0 {
			t.Errorf("%s: %v", titleOf(e), probs)
		}
		if id, _ := e.Body["id"].(string); id != "" {
			if seen[id] {
				t.Errorf("id 중복: %s", id)
			}
			seen[id] = true
		}
	}
	if len(entries) == 0 {
		t.Error("항목 0개")
	}
}
