package rules

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SyncResult 동기화 결과.
type SyncResult struct {
	Committed int      // 이번에 올린 로컬 변경 파일 수
	Pulled    []string // 다른 컴퓨터에서 받아 온 규칙 파일
	Remote    string
}

// ErrNoRemote rules.d 가 아직 git 저장소와 연결되지 않음.
var ErrNoRemote = errors.New("rules.d 가 동기화 저장소와 연결되지 않았습니다 — kosis rules sync --init <저장소 URL>")

// Sync rules.d 를 git 저장소로 합친다: 로컬 변경 commit → fetch → merge → push.
// initURL 이 있으면 먼저 git init·origin 설정을 한다 (이미 있으면 origin 만 바꾼다).
// 파일이 규칙마다 따로라 합치기는 대개 합집합이다. 같은 파일을 양쪽에서 고쳐 충돌하면
// merge 를 되돌리고 충돌 파일을 알려 준다 (로컬 변경은 commit 으로 남아 있다).
func Sync(dir, initURL string) (SyncResult, error) {
	var r SyncResult
	if _, err := exec.LookPath("git"); err != nil {
		return r, errors.New("git 이 필요합니다 (https://git-scm.com)")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return r, err
	}
	g := func(args ...string) (string, error) { return git(dir, args...) }
	_, statErr := os.Stat(filepath.Join(dir, ".git"))
	if initURL != "" {
		if statErr != nil {
			if _, err := g("init", "-q"); err != nil {
				return r, err
			}
			if _, err := g("symbolic-ref", "HEAD", "refs/heads/main"); err != nil {
				return r, err
			}
		}
		if _, err := g("remote", "get-url", "origin"); err == nil {
			_, err = g("remote", "set-url", "origin", initURL)
			if err != nil {
				return r, err
			}
		} else if _, err := g("remote", "add", "origin", initURL); err != nil {
			return r, err
		}
	} else if statErr != nil {
		return r, ErrNoRemote
	}
	remote, err := g("remote", "get-url", "origin")
	if err != nil {
		return r, ErrNoRemote
	}
	r.Remote = remote
	ignore := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(ignore); err != nil {
		if err := os.WriteFile(ignore, []byte(".rule-*.tmp\n"), 0o600); err != nil {
			return r, err
		}
	}

	// 1) 로컬 변경 commit
	if _, err := g("add", "-A"); err != nil {
		return r, err
	}
	changed, _ := g("diff", "--cached", "--name-only")
	if changed != "" {
		r.Committed = len(strings.Split(changed, "\n"))
		host, _ := os.Hostname()
		if _, err := g("commit", "-q", "-m", fmt.Sprintf("rules: %s 에서 %d개", host, r.Committed)); err != nil {
			return r, err
		}
	}

	// 2) 원격 받아 합치기 (원격이 비어 있으면 건너뜀)
	if _, err := g("fetch", "-q", "origin"); err != nil {
		return r, err
	}
	if _, err := g("rev-parse", "-q", "--verify", "refs/remotes/origin/main"); err == nil {
		before, _ := g("rev-parse", "-q", "--verify", "HEAD")
		if _, err := g("merge", "-q", "--no-edit", "--allow-unrelated-histories", "origin/main"); err != nil {
			conflicts, _ := g("diff", "--name-only", "--diff-filter=U")
			_, _ = g("merge", "--abort")
			return r, fmt.Errorf("다른 컴퓨터와 같은 규칙 파일을 다르게 고쳐 합치지 못했습니다: %s\n  직접 합치기: git -C %s merge origin/main → 파일 고치기 → git -C %s commit -am 합침 → kosis rules sync", strings.ReplaceAll(conflicts, "\n", ", "), dir, dir)
		}
		if before != "" {
			out, _ := g("diff", "--name-only", "--diff-filter=AM", before, "HEAD")
			for _, f := range strings.Split(out, "\n") {
				if strings.HasSuffix(f, ".yaml") {
					r.Pulled = append(r.Pulled, f)
				}
			}
		}
	}

	// 3) 올리기
	if _, err := g("push", "-q", "-u", "origin", "main"); err != nil {
		return r, err
	}
	return r, nil
}

// git dir 에서 git 을 실행한다. 사용자 이름이 설정되지 않은 컴퓨터에서도 commit 되도록 기본값을 준다.
func git(dir string, args ...string) (string, error) {
	pre := []string{"-C", dir}
	if out, _ := exec.Command("git", "-C", dir, "config", "user.email").Output(); len(bytes.TrimSpace(out)) == 0 {
		host, _ := os.Hostname()
		pre = append(pre, "-c", "user.name=kosis", "-c", "user.email=kosis@"+host)
	}
	cmd := exec.Command("git", append(pre, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return strings.TrimSpace(string(out)), fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(string(out)), nil
}
