// Package learn은 quick --ai의 선택 결과를 학습해 같은 실수를 반복하지 않게 합니다.
// 저장 위치: ~/.kosis/learnings.yaml (사용자 로컬, kosis update로 덮어쓰이지 않음)
//
// 학습 단위는 두 가지다.
//   - 주제(키워드+조건, TopicKey) → 통계표: 성공/사용자 교정 시 Prefer, 실패/사용자 거부 시 Avoid
//     조건을 키에 넣는 이유: "출생아수"(전국 표)로 학습한 표가 "울산 출생아수"에 쓰이면 지역 없는 전국 값이 조용히 나온다.
//   - 통계표 × 축 × 조건 → 코드: 분류축·항목(@ITEM)·주기(@PRD)를 같은 방식으로 기록
//
// 스킬의 LEARNINGS.md(사람·AI가 읽는 오답노트)와는 역할이 다르다 — 이 파일은 CLI가 기계적으로 읽고 쓴다.
package learn

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/clazic/kosis/internal/config"
	"gopkg.in/yaml.v3"
)

// Truncate 한글이 깨지지 않게 글자(rune) 단위로 자른다. 바이트로 자르면 yaml이 !!binary로 저장한다.
func Truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// Mark 피해야 할 대상과 이유.
type Mark struct {
	Target string `yaml:"target"`
	Reason string `yaml:"reason"`
	At     string `yaml:"at"`
}

// KeywordMemo 주제(TopicKey)별 통계표 학습.
type KeywordMemo struct {
	Prefer string `yaml:"prefer,omitempty"` // "ORG/TBL"
	Avoid  []Mark `yaml:"avoid,omitempty"`  // Target = "ORG/TBL"
}

// TableMemo 통계표별 축 선택 학습. 키는 "축ID|조건키".
type TableMemo struct {
	Picks map[string]string `yaml:"picks,omitempty"`
	Avoid map[string][]Mark `yaml:"avoid,omitempty"` // Target = 코드
}

// Store 학습 저장소 전체.
type Store struct {
	path     string                  // 저장 경로 (LoadFrom 으로 지정, 기본 FilePath)
	Keywords map[string]*KeywordMemo `yaml:"keywords"` // 키: TopicKey
	Tables   map[string]*TableMemo   `yaml:"tables"`
}

// FilePath 학습 파일 경로.
func FilePath() string {
	return filepath.Join(config.ConfigDir(), "learnings.yaml")
}

// Load 학습 파일을 읽는다. 없거나 깨졌으면 빈 저장소를 돌려준다 — 학습은 부가 기능이라 조회를 막지 않는다.
func Load() *Store { return LoadFrom(FilePath()) }

// LoadFrom 지정한 경로의 학습 파일을 읽는다 (평가처럼 실제 학습과 격리할 때).
func LoadFrom(path string) *Store {
	s := &Store{}
	if data, err := os.ReadFile(path); err == nil {
		if yaml.Unmarshal(data, s) != nil {
			fmt.Fprintf(os.Stderr, "경고: %s 파싱 실패, 학습 없이 진행합니다\n", path)
			s = &Store{}
		}
	}
	s.path = path
	if s.Keywords == nil {
		s.Keywords = map[string]*KeywordMemo{}
	}
	if s.Tables == nil {
		s.Tables = map[string]*TableMemo{}
	}
	return s
}

// Save 임시 파일에 쓰고 rename해 중간에 끊겨도 파일이 깨지지 않게 한다.
// ponytail: 동시 실행 시 마지막 저장이 이긴다(병합 없음). 학습 몇 건 유실은 다음 실행에서 다시 쌓이므로 허용.
func (s *Store) Save() error {
	if s.path == "" {
		s.path = FilePath()
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	data, err := yaml.Marshal(s)
	if err != nil {
		return fmt.Errorf("학습 인코딩 실패: %w", err)
	}
	f, err := os.CreateTemp(dir, ".learnings-*.tmp")
	if err != nil {
		return fmt.Errorf("학습 파일 저장 실패: %w", err)
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp)
		return fmt.Errorf("학습 파일 저장 실패: %v %v", werr, cerr)
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, s.path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// NormKey 공백·대소문자 차이를 없앤 비교용 키.
func NormKey(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), ""))
}

// TopicKey 검색 키워드와 조건을 합친 주제 키.
func TopicKey(keyword string, conds []string) string {
	return NormKey(keyword) + "|" + CondKey(conds)
}

// CondKey 조건 단어 목록을 순서 무관한 키로 만든다.
func CondKey(conds []string) string {
	keys := make([]string, 0, len(conds))
	for _, c := range conds {
		if k := NormKey(c); k != "" {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return strings.Join(keys, ",")
}

func now() string { return time.Now().Format("2006-01-02 15:04") }

func (s *Store) kw(keyword string) *KeywordMemo {
	k := NormKey(keyword)
	if s.Keywords[k] == nil {
		s.Keywords[k] = &KeywordMemo{}
	}
	return s.Keywords[k]
}

func (s *Store) tbl(table string) *TableMemo {
	if s.Tables[table] == nil {
		s.Tables[table] = &TableMemo{}
	}
	t := s.Tables[table]
	if t.Picks == nil {
		t.Picks = map[string]string{}
	}
	if t.Avoid == nil {
		t.Avoid = map[string][]Mark{}
	}
	return t
}

// PreferTable 주제에 대해 확정된 통계표("ORG/TBL"). 없으면 "".
func (s *Store) PreferTable(keyword string) string {
	if m := s.Keywords[NormKey(keyword)]; m != nil {
		return m.Prefer
	}
	return ""
}

// AvoidTables 주제에 대해 틀렸던 통계표 집합.
func (s *Store) AvoidTables(keyword string) map[string]string {
	out := map[string]string{}
	if m := s.Keywords[NormKey(keyword)]; m != nil {
		for _, a := range m.Avoid {
			out[a.Target] = a.Reason
		}
	}
	return out
}

// TableOK 조회 성공 또는 사용자 교정으로 확정된 통계표. Avoid에 있었다면 지운다.
func (s *Store) TableOK(keyword, table string) {
	m := s.kw(keyword)
	m.Prefer = table
	m.Avoid = without(m.Avoid, table)
}

// TableBad 틀린 통계표. Prefer였다면 해제한다.
func (s *Store) TableBad(keyword, table, reason string) {
	m := s.kw(keyword)
	if m.Prefer == table {
		m.Prefer = ""
	}
	m.Avoid = append(without(m.Avoid, table), Mark{Target: table, Reason: reason, At: now()})
}

func axisKey(axis, cond string) string { return axis + "|" + cond }

// AxisPick 학습된 축 선택 코드.
func (s *Store) AxisPick(table, axis, cond string) (string, bool) {
	if t := s.Tables[table]; t != nil {
		code, ok := t.Picks[axisKey(axis, cond)]
		return code, ok
	}
	return "", false
}

// AvoidCodes 이 축·조건에서 틀렸던 코드 집합.
func (s *Store) AvoidCodes(table, axis, cond string) map[string]bool {
	out := map[string]bool{}
	if t := s.Tables[table]; t != nil {
		for _, a := range t.Avoid[axisKey(axis, cond)] {
			out[a.Target] = true
		}
	}
	return out
}

// AxisOK 축 선택 확정.
func (s *Store) AxisOK(table, axis, cond, code string) {
	t := s.tbl(table)
	k := axisKey(axis, cond)
	t.Picks[k] = code
	t.Avoid[k] = without(t.Avoid[k], code)
	if len(t.Avoid[k]) == 0 {
		delete(t.Avoid, k)
	}
}

// AxisBad 틀린 축 선택.
func (s *Store) AxisBad(table, axis, cond, code, reason string) {
	t := s.tbl(table)
	k := axisKey(axis, cond)
	if t.Picks[k] == code {
		delete(t.Picks, k)
	}
	t.Avoid[k] = append(without(t.Avoid[k], code), Mark{Target: code, Reason: reason, At: now()})
}

func without(marks []Mark, target string) []Mark {
	out := marks[:0:0]
	for _, m := range marks {
		if m.Target != target {
			out = append(out, m)
		}
	}
	return out
}
