package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	out, cols, dups := pivotRows(rows, []string{"시도별", "항목", "시점", "수치값", "비고"}, "시도별", "수치값", map[string]bool{"비고": true})
	if strings.Join(cols, ",") != "항목,시점,중구,남구" || len(out) != 2 || dups != 1 {
		t.Fatalf("cols=%v out=%v dups=%d", cols, out, dups)
	}
	if out[0]["중구"] != "1" || out[0]["남구"] != "2" || out[1]["중구"] != "3" || out[1]["남구"] != nil {
		t.Errorf("값: %v", out)
	}
}
