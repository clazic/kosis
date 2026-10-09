package learn

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// 학습이 확정한 교정을 스킬 문서(SKILL.md, references/*.md)에 반영한다.
//
// 안전장치:
//   - 사람이 확인한 교정만 호출한다 (호출부 책임: --yes·낮은 신뢰도는 제외)
//   - "키워드"와 "틀린 표 ID"가 같은 줄에 있을 때만 그 줄을 고친다 — 같은 표를 다른 용도로 언급한 줄은 건드리지 않는다
//   - 파일을 처음 고치기 전에 같은 폴더에 숨김 백업(.<파일명>.bak)을 남긴다
//   - LEARNINGS.md 는 이력이므로 고치지 않고 오답 행을 추가만 한다
//
// ponytail: kosis update 가 스킬 파일을 덮어쓰면 수정도 사라진다. 그래도 learnings.yaml 이 남아 있어
// 다음 교정 때 다시 반영된다. 영구 반영은 저장소 문서를 고쳐 릴리스해야 한다.

// TableRef 통계표 식별자.
type TableRef struct{ Org, Tbl string }

// ParseTableRef "ORG/TBL" 문자열을 나눈다.
func ParseTableRef(s string) TableRef {
	org, tbl, _ := strings.Cut(s, "/")
	return TableRef{Org: org, Tbl: tbl}
}

func (t TableRef) String() string { return t.Org + "/" + t.Tbl }

// DocChange 문서 한 줄 변경 기록.
type DocChange struct {
	File   string
	Line   int
	Before string
	After  string
}

// docFiles 고칠 대상 문서. LEARNINGS.md 는 제외(이력).
func docFiles(dir string) []string {
	files := []string{filepath.Join(dir, "SKILL.md")}
	refs, _ := filepath.Glob(filepath.Join(dir, "references", "*.md"))
	return append(files, refs...)
}

// tableIDPattern 표 ID는 영문·숫자·밑줄로 이뤄져 \b가 밑줄을 단어 문자로 본다. 경계를 직접 정의한다.
func tableIDPattern(id string) *regexp.Regexp {
	return regexp.MustCompile(`(^|[^A-Za-z0-9_])` + regexp.QuoteMeta(id) + `($|[^A-Za-z0-9_])`)
}

// FixDocs 스킬 문서에서 keyword 와 bad 표가 함께 나오는 줄의 표를 good 으로 바꾼다.
// 기관이 다르면 "기관 표" 쌍이 붙어 있는 경우(예: `101 DT_X`, `101 / DT_X`)만 고친다.
func FixDocs(dirs []string, keyword string, bad, good TableRef) ([]DocChange, error) {
	if bad.Tbl == "" || good.Tbl == "" || bad == good || NormKey(keyword) == "" {
		return nil, nil
	}
	kw := NormKey(keyword)
	tblRe := tableIDPattern(bad.Tbl)
	pairRe := regexp.MustCompile(`(^|[^0-9])` + regexp.QuoteMeta(bad.Org) + "([\\s/`]*)" + regexp.QuoteMeta(bad.Tbl) + `($|[^A-Za-z0-9_])`)

	var changes []DocChange
	seen := map[string]bool{}
	for _, dir := range dirs {
		for _, path := range docFiles(dir) {
			real, err := filepath.EvalSymlinks(path)
			if err != nil || seen[real] { // 심링크로 같은 파일을 두 번 고치지 않는다
				continue
			}
			seen[real] = true

			data, err := os.ReadFile(real)
			if err != nil {
				continue
			}
			lines := strings.Split(string(data), "\n")
			changed := false
			for i, line := range lines {
				if !tblRe.MatchString(line) || !strings.Contains(NormKey(line), kw) {
					continue
				}
				var after string
				if bad.Org == good.Org {
					after = tblRe.ReplaceAllString(line, "${1}"+good.Tbl+"${2}")
				} else if pairRe.MatchString(line) {
					after = pairRe.ReplaceAllString(line, "${1}"+good.Org+"${2}"+good.Tbl+"${3}")
				} else {
					continue // 기관을 함께 확정할 수 없으면 손대지 않는다
				}
				if after == line {
					continue
				}
				changes = append(changes, DocChange{File: path, Line: i + 1, Before: line, After: after})
				lines[i] = after
				changed = true
			}
			if !changed {
				continue
			}
			if err := backupOnce(real, data); err != nil {
				return changes, err
			}
			if err := os.WriteFile(real, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				return changes, fmt.Errorf("문서 수정 실패 %s: %w", path, err)
			}
		}
	}
	return changes, nil
}

func backupOnce(path string, data []byte) error {
	bak := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".bak")
	if _, err := os.Stat(bak); err == nil {
		return nil // 최초 원본만 보존
	}
	return os.WriteFile(bak, data, 0o644)
}

// AppendLearning LEARNINGS.md 의 "## 테이블별 함정" 표 끝에 행을 추가한다. 같은 내용이 있으면 건너뛴다.
// 반환값: 실제로 추가한 파일 목록.
func AppendLearning(dirs []string, table, pitfall, why, next string) ([]string, error) {
	row := fmt.Sprintf("| %s | %s | %s | %s | %s |", time.Now().Format("2006-01-02"),
		mdCell(table), mdCell(pitfall), mdCell(why), mdCell(next))
	var added []string
	seen := map[string]bool{}
	for _, dir := range dirs {
		path := filepath.Join(dir, "LEARNINGS.md")
		real, err := filepath.EvalSymlinks(path)
		if err != nil || seen[real] {
			continue
		}
		seen[real] = true
		data, err := os.ReadFile(real)
		if err != nil {
			continue
		}
		text := string(data)
		if strings.Contains(text, mdCell(pitfall)) {
			continue
		}
		lines := strings.Split(text, "\n")
		sec := -1
		for i, l := range lines {
			if strings.HasPrefix(l, "## 테이블별 함정") {
				sec = i
				break
			}
		}
		if sec < 0 {
			continue
		}
		last := -1
		for i := sec + 1; i < len(lines); i++ {
			if strings.HasPrefix(lines[i], "## ") {
				break
			}
			if strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
				last = i
			} else if last >= 0 && strings.TrimSpace(lines[i]) != "" {
				break
			}
		}
		if last < 0 {
			continue
		}
		lines = append(lines[:last+1], append([]string{row}, lines[last+1:]...)...)
		if err := os.WriteFile(real, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return added, fmt.Errorf("LEARNINGS.md 기록 실패: %w", err)
		}
		added = append(added, path)
	}
	return added, nil
}

// mdCell 표 칸을 깨뜨리는 문자를 정리한다.
func mdCell(s string) string {
	return strings.TrimSpace(strings.NewReplacer("|", "/", "\r", " ", "\n", " ").Replace(s))
}
