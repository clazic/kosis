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
	if r, err := Sync(a, remote); err != nil || r.Committed != 2 { // 규칙 + .gitignore
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
