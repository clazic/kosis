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
		return strings.TrimRight(string(out), "\r\n "), fmt.Errorf("git %s: %s", args[0], strings.TrimSpace(stderr.String()))
	}
	// 앞 공백은 남긴다 (git status " M file"). Windows 줄 끝 \r\n → \n
	return strings.TrimRight(strings.ReplaceAll(string(out), "\r\n", "\n"), "\n "), nil
}

// Change rules.d 이력의 한 줄 (파일 하나의 변경).
type Change struct {
	Commit  string `json:"commit"` // "" 이면 아직 sync 하지 않은 변경
	Date    string `json:"date"`
	Subject string `json:"subject,omitempty"`
	Status  string `json:"status"` // A 추가, M 수정, D 삭제, R 이름 바뀜, ?? 새 파일(미동기화)
	File    string `json:"file"`
	Title   string `json:"title"`
}

// Log rules.d 의 git 이력(최근 순)과 아직 sync 하지 않은 변경. query 가 있으면 제목·파일 이름으로 거른다.
func Log(dir, query string, limit int) ([]Change, error) {
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return nil, errors.New("rules.d 에 이력이 없습니다 — kosis rules sync --init <저장소 URL> 뒤부터 기록됩니다")
	}
	g := func(args ...string) (string, error) { return git(dir, args...) }
	var out []Change
	keep := func(c Change) bool {
		if c.Title == "" {
			c.Title = titleAt(dir, c)
		}
		if query != "" && !strings.Contains(norm(c.Title+" "+c.File), norm(query)) {
			return len(out) < limit
		}
		out = append(out, c)
		return len(out) < limit
	}
	status, _ := g("status", "--porcelain", "--untracked-files=all", "--", "*.yaml")
	for _, line := range strings.Split(status, "\n") {
		if len(line) < 4 {
			continue
		}
		if !keep(Change{Date: "미동기화", Status: strings.TrimSpace(line[:2]), File: strings.TrimSpace(line[3:])}) {
			return out, nil
		}
	}
	logOut, err := g("log", "--date=short", "--format=%x1e%h%x1f%ad%x1f%s", "--name-status", "--", "*.yaml")
	if err != nil {
		return out, nil // 커밋이 아직 없음
	}
	for _, rec := range strings.Split(logOut, "\x1e") {
		lines := strings.Split(strings.TrimSpace(rec), "\n")
		head := strings.Split(lines[0], "\x1f")
		if len(head) < 3 {
			continue
		}
		for _, l := range lines[1:] {
			f := strings.Split(l, "\t")
			if len(f) < 2 {
				continue
			}
			if !keep(Change{Commit: head[0], Date: head[1], Subject: head[2], Status: f[0][:1], File: f[len(f)-1]}) {
				return out, nil
			}
		}
	}
	return out, nil
}

// titleAt 그 변경 시점의 규칙 제목 (삭제면 직전 내용).
func titleAt(dir string, c Change) string {
	var data []byte
	switch {
	case c.Commit == "":
		data, _ = os.ReadFile(filepath.Join(dir, c.File))
	case c.Status == "D":
		s, _ := git(dir, "show", c.Commit+"^:"+c.File)
		data = []byte(s)
	default:
		s, _ := git(dir, "show", c.Commit+":"+c.File)
		data = []byte(s)
	}
	if e, err := Parse(data); err == nil {
		return e.Title
	}
	return ""
}

// Revert 규칙 파일 하나를 이전 내용으로 되돌린다.
//   - sync 전 변경이 있으면: 마지막 sync 상태로
//   - 없으면: 그 파일의 직전 커밋 내용으로 (지운 파일이면 지우기 직전 내용으로 살림)
//   - to 가 있으면: 그 커밋의 내용으로
//
// 되돌린 결과는 아직 sync 하지 않은 변경이다. 반환: 무엇으로 되돌렸는지.
func Revert(dir, file, to string) (string, error) {
	name := filepath.Base(file)
	if !fileNameRe.MatchString(name) {
		return "", fmt.Errorf("규칙 파일 이름이 아닙니다: %s (YYYYMMDD-<8hex>.yaml)", name)
	}
	if _, err := os.Stat(filepath.Join(dir, ".git")); err != nil {
		return "", errors.New("rules.d 에 이력이 없어 되돌릴 수 없습니다 (kosis rules sync --init 전)")
	}
	g := func(args ...string) (string, error) { return git(dir, args...) }
	if to == "" {
		st, _ := g("status", "--porcelain", "--", name)
		switch {
		case strings.HasPrefix(st, "??"):
			return "", errors.New("아직 sync 하지 않은 새 규칙이라 이전 내용이 없습니다 — 틀렸으면 status: wrong 을 적으세요")
		case st != "":
			if _, err := g("checkout", "HEAD", "--", name); err != nil {
				return "", err
			}
			return "sync 전 변경을 버리고 마지막 sync 상태로", nil
		}
		hs, _ := g("log", "-n2", "--format=%h", "--", name)
		h := strings.Fields(hs)
		if len(h) < 2 {
			return "", errors.New("처음 버전이라 되돌릴 이전 내용이 없습니다 — 틀렸으면 status: wrong 을 적으세요")
		}
		to = h[1]
	}
	if _, err := g("checkout", to, "--", name); err != nil {
		return "", fmt.Errorf("%s 시점에 이 파일이 없습니다: %w", to, err)
	}
	return to + " 시점 내용으로", nil
}
