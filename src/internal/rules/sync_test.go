package rules

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// 두 컴퓨터(A·B)가 비공개 저장소 하나로 규칙을 합치는 흐름.
func TestSyncTwoComputers(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	if out, err := exec.Command("git", "init", "-q", "--bare", remote).CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	a, b := filepath.Join(root, "a"), filepath.Join(root, "b")

	if _, err := Sync(a, ""); err != ErrNoRemote {
		t.Fatalf("연결 전에는 ErrNoRemote: %v", err)
	}
	ea := sampleTable()
	ea.Host = "a"
	if _, err := Write(a, ea, now); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(a, ".omc", "state"), 0o700) // 다른 도구의 숨김 폴더
	_ = os.WriteFile(filepath.Join(a, ".omc", "state", "x.json"), []byte("{}"), 0o600)
	if r, err := Sync(a, remote); err != nil || r.Committed != 2 { // 규칙 + .gitignore (숨김 폴더 제외)
		t.Fatalf("A init: %+v %v", r, err)
	}

	eb := sampleTable()
	eb.Host = "b"
	if _, err := Write(b, eb, now); err != nil {
		t.Fatal(err)
	}
	r, err := Sync(b, remote)
	if err != nil || len(r.Pulled) != 1 {
		t.Fatalf("B init 은 A 규칙을 받아야 함: %+v %v", r, err)
	}
	if r, err := Sync(a, ""); err != nil || len(r.Pulled) != 1 {
		t.Fatalf("A 는 B 규칙을 받아야 함: %+v %v", r, err)
	}
	for _, d := range []string{a, b} {
		if got, probs := LoadDir(d); len(got) != 2 || len(probs) != 0 {
			t.Errorf("%s: 합집합 2개여야 함 %d %+v", d, len(got), probs)
		}
	}

	// 같은 파일을 양쪽에서 다르게 고치면 충돌 → merge 되돌림, 파일 이름 알림
	got, _ := LoadDir(a)
	f := filepath.Base(got[0].File)
	appendTo := func(dir, s string) {
		p := filepath.Join(dir, f)
		data, _ := os.ReadFile(p)
		_ = os.WriteFile(p, append(data, []byte(s)...), 0o600)
	}
	appendTo(a, "status: wrong\n")
	if _, err := Sync(a, ""); err != nil {
		t.Fatal(err)
	}
	appendTo(b, "note: 다름\n")
	_, err = Sync(b, "")
	if err == nil || !strings.Contains(err.Error(), f) {
		t.Fatalf("충돌 파일을 알려야 함: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(b, ".git", "MERGE_HEAD")); statErr == nil {
		t.Error("merge 는 되돌려져야 함")
	}
}

func TestLogAndRevert(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git 없음")
	}
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	_ = exec.Command("git", "init", "-q", "--bare", remote).Run()
	dir := filepath.Join(root, "a")
	path, err := Write(dir, sampleTable(), now)
	if err != nil {
		t.Fatal(err)
	}
	name := filepath.Base(path)
	if _, err := Revert(dir, name, ""); err == nil {
		t.Error("이력 없으면 오류")
	}
	if _, err := Sync(dir, remote); err != nil {
		t.Fatal(err)
	}
	if _, err := Revert(dir, name, ""); err == nil || !strings.Contains(err.Error(), "처음 버전") {
		t.Errorf("처음 버전은 되돌릴 수 없음: %v", err)
	}
	orig, _ := os.ReadFile(path)
	_ = os.WriteFile(path, append(append([]byte{}, orig...), "status: wrong\n"...), 0o600)

	// 미동기화 변경이 이력 맨 위에, 제목과 함께
	ch, err := Log(dir, "울주", 10)
	if err != nil || len(ch) != 2 || ch[0].Commit != "" || ch[0].Title != sampleTable().Title || ch[1].Status != "A" {
		t.Fatalf("log: %+v %v", ch, err)
	}
	// sync 전 변경 → 마지막 sync 상태로
	if _, err := Revert(dir, name, ""); err != nil {
		t.Fatal(err)
	}
	if now, _ := os.ReadFile(path); string(now) != string(orig) {
		t.Error("sync 전 변경이 버려져야 함")
	}
	// sync 후 고친 것 → 직전 커밋으로
	_ = os.WriteFile(path, append(append([]byte{}, orig...), "status: wrong\n"...), 0o600)
	if _, err := Sync(dir, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := Revert(dir, name, ""); err != nil {
		t.Fatal(err)
	}
	if now, _ := os.ReadFile(path); string(now) != string(orig) {
		t.Error("직전 커밋 내용이어야 함")
	}
	// 지운 파일 살리기
	_, _ = Sync(dir, "")
	_ = os.Remove(path)
	_, _ = Sync(dir, "")
	if _, err := Revert(dir, name, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Error("지운 규칙이 살아나야 함")
	}
	if _, err := Revert(dir, "울산.yaml", ""); err == nil {
		t.Error("규칙 파일 이름이 아니면 오류")
	}
}
