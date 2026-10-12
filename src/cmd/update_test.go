package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clazic/kosis/internal/api"
)

func TestNewerVer(t *testing.T) {
	cases := []struct {
		remote, local string
		want          bool
	}{
		{"v0.1.0", "v0.5.2", false}, // 다운그레이드 방지
		{"v0.5.3", "v0.5.2", true},
		{"v0.5.2", "v0.5.2", false},
		{"v1.0.0", "v0.9.9", true},
		{"v0.10.0", "v0.9.0", true}, // 숫자 비교 (문자열 비교면 실패)
		{"v0.5", "v0.5.0", false},
		{"v0.5.2", "dev", true}, // dev 빌드에는 모든 릴리스가 새 버전
		{"v0.5.2", "v0.5.2-dirty", false},
	}
	for _, c := range cases {
		if got := newerVer(c.remote, c.local); got != c.want {
			t.Errorf("newerVer(%q, %q) = %v, want %v", c.remote, c.local, got, c.want)
		}
	}
}

func TestParseSHA256SumsAndVerify(t *testing.T) {
	dir := t.TempDir()

	// 검증 대상 파일 — 내용이 정해져 있으므로 해시도 고정값
	payload := filepath.Join(dir, "kosis-darwin-arm64")
	if err := os.WriteFile(payload, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	const helloSum = "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"

	// sha256sum 출력 형식: 두 스페이스 구분 + 바이너리 모드(*) 접두사 혼재
	sums := filepath.Join(dir, "SHA256SUMS")
	content := helloSum + "  kosis-darwin-arm64\n" +
		helloSum + " *kosis-skill-v0.5.2.tar.gz\n" +
		"\n" + // 빈 줄은 무시되어야 함
		"garbage\n" // 필드가 모자란 줄도 무시되어야 함
	if err := os.WriteFile(sums, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := parseSHA256Sums(sums)
	if err != nil {
		t.Fatalf("parseSHA256Sums: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("항목 수 = %d, want 2 (%v)", len(got), got)
	}
	if got["kosis-darwin-arm64"] != helloSum {
		t.Errorf("바이너리 해시 = %q, want %q", got["kosis-darwin-arm64"], helloSum)
	}
	// * 접두사가 벗겨진 이름으로 조회돼야 함
	if got["kosis-skill-v0.5.2.tar.gz"] != helloSum {
		t.Errorf("스킬 해시 조회 실패: %v", got)
	}

	if err := verifySHA256(payload, helloSum); err != nil {
		t.Errorf("일치하는 해시인데 실패: %v", err)
	}
	if err := verifySHA256(payload, "deadbeef"); err == nil {
		t.Error("불일치 해시인데 통과 — 변조된 다운로드가 설치될 수 있음")
	}
}

// 지난 설치 이후 직접 고친 스킬 파일만 백업된다.
func TestBackupModifiedSkillFiles(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	dest := filepath.Join(home, ".agents", "skills", "kosis")
	_ = os.MkdirAll(filepath.Join(dest, "references"), 0o755)
	_ = os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("v1"), 0o644)
	_ = os.WriteFile(filepath.Join(dest, "references", "a.md"), []byte("a"), 0o644)

	if ch, _, err := backupModifiedSkillFiles(dest, "s0"); err != nil || len(ch) != 0 {
		t.Fatalf("기록이 없으면 건너뜀: %v %v", ch, err)
	}
	if err := writeSkillManifest(dest, "v0.5.13", []string{"SKILL.md", filepath.Join("references", "a.md")}); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dest, "SKILL.md"), []byte("내가 고침"), 0o644)
	ch, backup, err := backupModifiedSkillFiles(dest, "s1")
	if err != nil || len(ch) != 1 || ch[0] != "SKILL.md" {
		t.Fatalf("고친 파일만: %v %v", ch, err)
	}
	if got, _ := os.ReadFile(filepath.Join(backup, "SKILL.md")); string(got) != "내가 고침" {
		t.Errorf("백업 내용: %q (%s)", got, backup)
	}
	if !strings.Contains(backup, filepath.Join("skill-backup", "s1", ".agents_skills_kosis")) {
		t.Errorf("백업 위치: %s", backup)
	}
}

func TestPivotRows(t *testing.T) {
	rows := []map[string]interface{}{
		{"시도별": "중구", "항목": "생산액", "시점": "2023", "수치값": "1", "비고": "a"},
		{"시도별": "남구", "항목": "생산액", "시점": "2023", "수치값": "2", "비고": "b"},
		{"시도별": "중구", "항목": "출하액", "시점": "2023", "수치값": "3"},
		{"시도별": "중구", "항목": "출하액", "시점": "2023", "수치값": "9"},
	}
	out, cols, dups := pivotRows(rows, []string{"시도별", "항목", "시점", "수치값", "비고"}, "시도별", "수치값", map[string]bool{"비고": true}, "")
	if strings.Join(cols, ",") != "항목,시점,중구,남구" || len(out) != 2 || dups != 1 {
		t.Fatalf("cols=%v out=%v dups=%d", cols, out, dups)
	}
	if out[0]["중구"] != "1" || out[0]["남구"] != "2" || out[1]["중구"] != "3" || out[1]["남구"] != nil {
		t.Errorf("값: %v", out)
	}
}

func TestMissingClassFlags(t *testing.T) {
	s := &api.MetaSummaryResult{Classifications: []api.MetaResult{
		{ObjID: "A", ObjNM: "지역별"}, {ObjID: "A", ObjNM: "지역별"}, {ObjID: "B", ObjNM: "산업별"}, {ObjID: "C", ObjNM: "규모별"},
	}}
	got := missingClassFlags(s, []string{"ALL", "C", "", "", "", "", "", ""})
	if axisCount(s) != 3 || len(got) != 1 || got[0] != "--class3(규모별)" {
		t.Errorf("%v", got)
	}
	if len(missingClassFlags(s, []string{"a", "b", "c", "", "", "", "", ""})) != 0 {
		t.Error("다 있으면 없음")
	}
}

// --api-keys 는 --fields(한국어 라벨)·--pivot(C1·분류 이름)을 라벨로 처리한 뒤 출력 열 이름만 바꾼다.
func TestAPIKeysWithFieldsAndPivot(t *testing.T) {
	cm := (&api.MetaSummaryResult{
		Classifications: []api.MetaResult{{ObjID: "A", ObjNM: "시도별", ItmID: "26010", ItmNM: "중구"}, {ObjID: "B", ObjNM: "산업별", ItmID: "C", ItmNM: "제조업"}},
		Items:           []api.MetaResult{{ObjID: "ITEM", ObjNM: "항목", ItmID: "T02", ItmNM: "종사자수"}},
	}).BuildColumnMetaWithCode()
	rowsOf := func() ([]map[string]interface{}, []string) {
		var cols []string
		for _, c := range cm.Columns {
			cols = append(cols, c.Label)
		}
		mk := func(c1, c1nm, itm, itmnm, dt string) map[string]interface{} {
			r := api.DataRow{C1: c1, C1NM: c1nm, C2: "C", C2NM: "제조업", ItmID: itm, ItmNM: itmnm, PrdSe: "A", PrdDe: "2024", DT: dt, UnitNM: "명"}
			m := map[string]interface{}{}
			for _, c := range cm.Columns {
				m[c.Label] = r.GetField(c.Key)
			}
			return m
		}
		return []map[string]interface{}{
			mk("26010", "중구", "T02", "종사자수", "1"), mk("26020", "남구", "T02", "종사자수", "2"),
			mk("26010", "중구", "T04", "출하액", "3"), mk("26020", "남구", "T04", "출하액", "4"),
		}, cols
	}
	toKey := map[string]string{}
	for _, c := range cm.Columns {
		toKey[c.Label] = c.Key
	}

	// --fields '시도별,수치값' --api-keys
	rows, _ := rowsOf()
	out, cols := renameColumns(rows, []string{"시도별", "수치값"}, toKey)
	if strings.Join(cols, ",") != "C1_NM,DT" || out[0]["C1_NM"] != "중구" || out[0]["DT"] != "1" {
		t.Errorf("fields: cols=%v row=%v", cols, out[0])
	}

	// --pivot C1 / --pivot 시도별 + --api-keys → 지역 이름이 열, 2행(항목별)
	for _, arg := range []string{"C1", "시도별"} {
		rows, cols := rowsOf()
		pcol, drop, ok := pivotColumn(cm, arg)
		if !ok {
			t.Fatalf("pivot %s: 축 없음", arg)
		}
		prow, pcols, _ := pivotRows(rows, cols, pcol, cm.GetLabel("DT"), drop, "")
		out, ocols := renameColumns(prow, pcols, toKey)
		joined := strings.Join(ocols, ",")
		if len(out) != 2 || !strings.Contains(joined, "중구") || !strings.Contains(joined, "남구") || strings.Contains(joined, "C1") {
			t.Errorf("pivot %s: rows=%d cols=%v", arg, len(out), ocols)
		}
		if out[0]["중구"] != "1" || out[0]["남구"] != "2" || out[0]["ITM_NM"] != "종사자수" {
			t.Errorf("pivot %s 값: %v", arg, out[0])
		}
	}
}

// 메타가 없을 때(--user-id·메타 실패)도 --api-keys 는 API 키 열 이름을 낸다.
func TestAPIKeysFallbackWithoutMeta(t *testing.T) {
	rows := []map[string]interface{}{apiKeyRow(api.DataRow{C1: "11", C1NM: "서울", ItmID: "T1", ItmNM: "인구", PrdSe: "A", PrdDe: "2024", DT: "9", UnitNM: "명"})}
	if got := strings.Join(apiKeyColumns(rows), ","); got != "C1,C1_NM,ITM_ID,ITM_NM,PRD_SE,PRD_DE,DT,UNIT_NM" {
		t.Errorf("cols=%s", got)
	}
	if rows[0]["C1_NM"] != "서울" || rows[0]["DT"] != "9" {
		t.Errorf("row=%v", rows[0])
	}
}

// JSON 으로 나가면(-f json 이든 -o *.json 이든) 코드 열을 넣는다.
func TestNeedCodeColumns(t *testing.T) {
	cases := []struct {
		format, out, fields string
		with                bool
		want                bool
	}{
		{"json", "", "", false, true},
		{"table", "결과.json", "", false, true},
		{"table", "결과.JSON", "", false, true},
		{"table", "결과.csv", "", false, false},
		{"table", "결과.parquet", "", false, true},
		{"table", "", "", false, false},
		{"table", "결과.xlsx", "", true, true},
		{"csv", "", "C1,DT", false, true},
	}
	for _, c := range cases {
		if got := needCodeColumns(c.format, c.out, c.with, c.fields); got != c.want {
			t.Errorf("%+v → %v", c, got)
		}
	}
}

// 항목을 열로 펼치면 항목마다 다른 단위가 행을 가르지 않고 열 이름에 붙는다.
func TestPivotItemUnits(t *testing.T) {
	rows := []map[string]interface{}{
		{"시도별": "중구", "항목": "종사자수", "단위": "명", "수치값": "1"},
		{"시도별": "중구", "항목": "출하액", "단위": "백만원", "수치값": "2"},
		{"시도별": "남구", "항목": "종사자수", "단위": "명", "수치값": "3"},
		{"시도별": "남구", "항목": "출하액", "단위": "백만원", "수치값": "4"},
	}
	out, cols, _ := pivotRows(rows, []string{"시도별", "항목", "단위", "수치값"}, "항목", "수치값", map[string]bool{"단위": true}, "단위")
	if strings.Join(cols, ",") != "시도별,종사자수(명),출하액(백만원)" || len(out) != 2 || out[1]["출하액(백만원)"] != "4" {
		t.Errorf("cols=%v out=%v", cols, out)
	}
}

// parquet 은 열이 API 키로 고정이라 --api-keys 없이도 API 키로 저장해야 값이 남는다.
func TestOutputUsesAPIKeys(t *testing.T) {
	if !outputUsesAPIKeys(false, "결과.parquet") || !outputUsesAPIKeys(false, "R.PARQUET") || !outputUsesAPIKeys(true, "") ||
		outputUsesAPIKeys(false, "결과.csv") || outputUsesAPIKeys(false, "") {
		t.Error("parquet·--api-keys 일 때만")
	}
}
