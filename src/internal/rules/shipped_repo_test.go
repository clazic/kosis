package rules

import (
	"os"
	"testing"
)

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
