package rules

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var now = time.Date(2026, 10, 12, 11, 30, 0, 0, time.FixedZone("KST", 9*3600))

func sampleTable() Entry {
	return Entry{
		Kind: "table", Title: "울산 울주군 코드가 표마다 다름", Host: "testhost",
		Verified: "2026-10-12", Evidence: "m 101 DT_1FS1101 → 26510",
		Body: map[string]any{"table": "101/DT_1FS1101", "axes": []any{map[string]any{"flag": "--class1", "codes": map[string]any{"울산 울주군": "26510"}}}},
	}
}

func TestWriteLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path, err := Write(dir, sampleTable(), now)
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^20261012-[0-9a-f]{8}\.yaml$`).MatchString(filepath.Base(path)) {
		t.Errorf("파일 이름은 ASCII YYYYMMDD-<8hex>.yaml: %s", filepath.Base(path))
	}
	if info, _ := os.Stat(path); info.Mode().Perm() != 0o600 {
		t.Errorf("권한 0600: %v", info.Mode().Perm())
	}
	got, probs := LoadDir(dir)
	if len(probs) != 0 || len(got) != 1 {
		t.Fatalf("got %d, problems %+v", len(got), probs)
	}
	e := got[0]
	if e.Title != "울산 울주군 코드가 표마다 다름" || e.Body["table"] != "101/DT_1FS1101" || e.Source != "local" || e.Created == "" {
		t.Errorf("읽은 값: %+v", e)
	}
	if _, err := Write(dir, sampleTable(), now); err == nil {
		t.Error("같은 내용·시각은 같은 이름 — 덮어쓰지 않아야 함")
	}
}

func TestDifferentHostsNeverCollide(t *testing.T) {
	a, b := sampleTable(), sampleTable()
	a.Host, b.Host = "d", "m"
	if FileName(a, now) == FileName(b, now) {
		t.Error("컴퓨터가 다르면 파일 이름이 달라야 함 (git 합치기 = 합집합)")
	}
}

func TestValidate(t *testing.T) {
	cases := []struct {
		name string
		edit func(*Entry)
		want string
	}{
		{"kind", func(e *Entry) { e.Kind = "memo" }, "kind 는"},
		{"evidence", func(e *Entry) { e.Evidence = "" }, "evidence"},
		{"verified", func(e *Entry) { e.Verified = "어제" }, "verified 는 YYYY-MM-DD"},
		{"table 형식", func(e *Entry) { e.Body["table"] = "DT_1FS1101" }, "ORG/TBL"},
		{"rule 필드", func(e *Entry) { e.Kind = "rule"; e.Body = map[string]any{"when": "x"} }, "do 가 필요"},
		{"status", func(e *Entry) { e.Status = "deleted" }, "status 는 wrong"},
	}
	for _, c := range cases {
		e := sampleTable()
		c.edit(&e)
		if got := strings.Join(Validate(e), "; "); !strings.Contains(got, c.want) {
			t.Errorf("%s: %q 에 %q 없음", c.name, got, c.want)
		}
	}
	e := sampleTable()
	e.Verified, e.Confidence = "", "guess"
	if p := Validate(e); len(p) != 0 {
		t.Errorf("확인 전(guess)은 verified 없어도 됨: %v", p)
	}
}

func TestLoadDirReportsBadFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "울산메모.yaml"), []byte("kind: table\ntitle: x\n"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, "20261012-00000000.yaml"), []byte("kind: [깨짐"), 0o600)
	_ = os.WriteFile(filepath.Join(dir, ".rule-123.tmp"), []byte("x"), 0o600)
	_, probs := LoadDir(dir)
	if len(probs) != 2 {
		t.Fatalf("한글 파일명·깨진 YAML 두 개를 잡아야 함 (임시 파일은 무시): %+v", probs)
	}
	joined := probs[0].Problems[0] + probs[1].Problems[0]
	if !strings.Contains(joined, "YAML 오류") {
		t.Errorf("깨진 YAML 표시: %+v", probs)
	}
}

// 실제 배포 파일이 형식을 지키는지 — 규칙을 고칠 때 깨지면 여기서 잡힌다.
func TestShippedRulesValid(t *testing.T) {
	path := filepath.Join("..", "..", "..", "references", "rules.yaml")
	entries, err := LoadShipped(path)
	if err != nil {
		t.Fatal(err)
	}
	counts := map[string]int{}
	ids := map[string]bool{}
	for _, e := range entries {
		counts[e.Kind]++
		if p := Validate(e); len(p) > 0 {
			t.Errorf("%s %q: %v", e.Kind, e.Title, p)
		}
		key := e.Kind + ":" + e.Title
		if ids[key] {
			t.Errorf("중복: %s", key)
		}
		ids[key] = true
	}
	if counts["rule"] < 30 || counts["table"] < 15 {
		t.Errorf("배포 규칙 수가 줄었음: %v", counts)
	}
}

func TestMigrateLegacy(t *testing.T) {
	dir := t.TempDir()
	legacy := filepath.Join(dir, "rules.local.yaml")
	_ = os.WriteFile(legacy, []byte(`version: 1
rules: []
tables:
  - id: "207/DT_2071M26"
    name: (이름 미확인)
    question: "울산 구군별 업종별 종사자수"
    confidence: guess
    evidence: "이전 kosis q 학습 기록"
    verified: ""
`), 0o600)
	rulesDir := filepath.Join(dir, "rules.d")
	n, err := MigrateLegacy(legacy, rulesDir, now)
	if err != nil || n != 1 {
		t.Fatalf("migrate = %d %v", n, err)
	}
	got, probs := LoadDir(rulesDir)
	if len(probs) != 0 || len(got) != 1 || got[0].Body["table"] != "207/DT_2071M26" || got[0].Confidence != "guess" {
		t.Errorf("옮긴 결과: %+v %+v", got, probs)
	}
	if _, err := os.Stat(legacy + ".migrated"); err != nil {
		t.Error("원본은 .migrated 로 남아야 함")
	}
	if n, _ := MigrateLegacy(legacy, rulesDir, now); n != 0 {
		t.Error("두 번 옮기지 않아야 함")
	}
}

func TestMatches(t *testing.T) {
	e := sampleTable()
	if !Matches(e, "울주 군") || !Matches(e, "dt_1fs1101") || Matches(e, "부산") {
		t.Error("공백·대소문자 무시 검색")
	}
}

func TestParseAndSimilar(t *testing.T) {
	e, err := Parse([]byte("kind: table\ntitle: 울산 코드\nverified: \"2026-10-12\"\nevidence: m 확인\ntable: 101/DT_1FS1101\nsource: local\n"))
	if err != nil || e.Body["table"] != "101/DT_1FS1101" || e.Body["source"] != nil {
		t.Fatalf("parse: %+v %v", e, err)
	}
	if _, err := Parse([]byte("")); err == nil {
		t.Error("빈 입력은 오류")
	}
	if len(Similar(e, []Entry{sampleTable()})) != 1 {
		t.Error("같은 표를 가리키면 비슷한 항목")
	}
	r := sampleTable()
	r.Kind = "rule"
	if len(Similar(e, []Entry{r})) != 0 {
		t.Error("종류가 다르면 비슷하지 않음")
	}
}
