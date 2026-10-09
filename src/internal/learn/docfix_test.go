package learn

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSkill(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, body := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestFixDocsOnlyKeywordLines(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, map[string]string{
		"SKILL.md": "| 출생아수 | 101 | `INH_1B81A01` | 시도별 |\n" +
			"| 사망자수 | 101 | `INH_1B81A01` | 같은 표 다른 용도 — 건드리면 안 됨 |\n" +
			"| 출생아수 비슷한 ID | INH_1B81A01_X |\n",
		"references/05.md": "kosis d 101 INH_1B81A01 -c1 26  # 출생아수\n",
	})

	ch, err := FixDocs([]string{dir, dir}, "출생 아수", TableRef{"101", "INH_1B81A01"}, TableRef{"101", "DT_1B81A23"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ch) != 2 {
		t.Fatalf("키워드+표가 함께 있는 2줄만 고쳐야 함 (같은 디렉토리 중복 포함): %+v", ch)
	}
	skill, _ := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	got := string(skill)
	if !strings.Contains(got, "| 출생아수 | 101 | `DT_1B81A23` |") || !strings.Contains(got, "| 사망자수 | 101 | `INH_1B81A01` |") || !strings.Contains(got, "INH_1B81A01_X") {
		t.Errorf("수정 결과가 다름:\n%s", got)
	}
	if _, err := os.Stat(filepath.Join(dir, ".SKILL.md.bak")); err != nil {
		t.Error("백업이 없음")
	}
}

func TestFixDocsDifferentOrgNeedsPair(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, map[string]string{
		"SKILL.md": "출생아수: 101 / `DT_A`\n출생아수는 DT_A 참고\n",
	})
	ch, _ := FixDocs([]string{dir}, "출생아수", TableRef{"101", "DT_A"}, TableRef{"116", "DT_B"})
	if len(ch) != 1 || !strings.Contains(ch[0].After, "116 / `DT_B`") {
		t.Errorf("기관이 다르면 '기관 표' 쌍이 있는 줄만 고쳐야 함: %+v", ch)
	}
}

func TestAppendLearningDedup(t *testing.T) {
	dir := t.TempDir()
	writeSkill(t, dir, map[string]string{
		"LEARNINGS.md": "## 테이블별 함정\n\n| 날짜 | ORG/TBL | 함정 | 왜 | 다음엔 |\n|---|---|---|---|---|\n| a | b | c | d | e |\n\n---\n\n## 성공 쿼리 패턴\n",
	})
	for i := 0; i < 2; i++ {
		if _, err := AppendLearning([]string{dir}, "101 / `DT_B`", "'출생아수|울산' 요청에 DT_A 오선택", "사용자 교정", "DT_B 사용"); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(filepath.Join(dir, "LEARNINGS.md"))
	text := string(data)
	if strings.Count(text, "DT_A 오선택") != 1 {
		t.Errorf("중복 기록:\n%s", text)
	}
	if !strings.Contains(text, "| c | d | e |\n| ") || strings.Index(text, "DT_A 오선택") > strings.Index(text, "## 성공") {
		t.Errorf("함정 표 끝에 들어가야 함:\n%s", text)
	}
	if strings.Contains(text, "출생아수|울산") {
		t.Error("파이프 문자가 표를 깨뜨림")
	}
}
