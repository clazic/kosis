// Package rules 는 kosis 를 쓰는 AI 에이전트용 규칙을 읽고·검증하고·쓴다.
//
//   - 배포 규칙: 스킬의 references/rules.yaml (절: rules, tables, concepts, breakdowns, crosswalks, gaps)
//   - 로컬 규칙: ~/.kosis/rules.d/*.yaml — 규칙 하나 = 파일 하나. 여러 컴퓨터를 git 으로 합칠 때
//     충돌이 나지 않도록(합집합) 나눈다. 파일 이름은 ASCII 만 쓴다 (macOS NFD / Windows·Linux NFC 차이 회피).
//
// 설계: .plan/2026-10-12-11:00:00-rules-설계-다중-데이터셋.md
package rules

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/clazic/kosis/internal/config"
	"gopkg.in/yaml.v3"
)

// Kinds 규칙 종류. 배포 rules.yaml 의 절 이름(복수형)과 1:1 대응한다.
var Kinds = []string{"rule", "table", "concept", "breakdown", "crosswalk", "gap"}

// sectionKind 배포 rules.yaml 의 절 → 종류.
var sectionKind = map[string]string{
	"rules": "rule", "tables": "table", "concepts": "concept",
	"breakdowns": "breakdown", "crosswalks": "crosswalk", "gaps": "gap",
}

// Entry 규칙 한 항목. 공통 메타 + 종류별 본문(Body).
type Entry struct {
	Kind       string `yaml:"kind"`
	Title      string `yaml:"title"`
	Host       string `yaml:"host,omitempty"`
	Created    string `yaml:"created,omitempty"`
	Verified   string `yaml:"verified"`
	Evidence   string `yaml:"evidence"`
	Confidence string `yaml:"confidence,omitempty"` // guess | tendency (없으면 확인됨)
	Status     string `yaml:"status,omitempty"`     // wrong (틀린 것으로 판명, 지우지 않음)

	Body map[string]any `yaml:",inline"` // 종류별 필드 (배포 rules.yaml 항목과 같은 이름)

	Source string `yaml:"-"` // "shipped" | "local"
	File   string `yaml:"-"` // 읽어 온 파일
}

// Dir 로컬 규칙 디렉토리.
func Dir() string { return filepath.Join(config.ConfigDir(), "rules.d") }

// LegacyLocalFile 이전 형식의 로컬 규칙 파일 (rules.d 로 옮긴다).
func LegacyLocalFile() string { return filepath.Join(config.ConfigDir(), "rules.local.yaml") }

var (
	fileNameRe = regexp.MustCompile(`^\d{8}-[0-9a-f]{8}\.yaml$`)
	dateRe     = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	tableRe    = regexp.MustCompile(`^[0-9A-Za-z]+/[0-9A-Za-z_]+$`)
)

// Validate 항목의 문제 목록. 비어 있으면 통과.
func Validate(e Entry) []string {
	var p []string
	if !contains(Kinds, e.Kind) {
		p = append(p, fmt.Sprintf("kind 는 %s 중 하나 (지금: %q)", strings.Join(Kinds, "|"), e.Kind))
	}
	if strings.TrimSpace(e.Title) == "" {
		p = append(p, "title 이 비어 있음")
	}
	if strings.TrimSpace(e.Evidence) == "" {
		p = append(p, "evidence 가 비어 있음 (무엇을 실행해 무엇을 봤는지)")
	}
	switch {
	case e.Confidence == "guess" && e.Verified == "":
	case !dateRe.MatchString(e.Verified):
		p = append(p, fmt.Sprintf("verified 는 YYYY-MM-DD (지금: %q) — 확인 전이면 confidence: guess", e.Verified))
	}
	if e.Confidence != "" && e.Confidence != "guess" && e.Confidence != "tendency" {
		p = append(p, fmt.Sprintf("confidence 는 guess|tendency (지금: %q)", e.Confidence))
	}
	if e.Status != "" && e.Status != "wrong" {
		p = append(p, fmt.Sprintf("status 는 wrong 만 (지금: %q)", e.Status))
	}
	need := func(keys ...string) {
		for _, k := range keys {
			if v, ok := e.Body[k]; !ok || v == nil || v == "" {
				p = append(p, fmt.Sprintf("%s 항목에는 %s 가 필요", e.Kind, k))
			}
		}
	}
	switch e.Kind {
	case "rule":
		need("when", "do")
	case "table":
		need("table")
		if t, ok := e.Body["table"].(string); ok && !tableRe.MatchString(t) {
			p = append(p, fmt.Sprintf("table 은 ORG/TBL 형식 (지금: %q)", t))
		}
	case "concept":
		need("id", "sources")
	case "breakdown":
		need("term", "level")
	case "crosswalk":
		need("id", "members")
	case "gap":
		need("id", "missing")
	}
	return p
}

// FileName 새 항목의 파일 이름: YYYYMMDD-<8hex>.yaml (내용·host·생성 시각 해시 — 컴퓨터끼리 겹치지 않음).
func FileName(e Entry, now time.Time) string {
	body, _ := yaml.Marshal(e.Body)
	h := sha256.Sum256([]byte(e.Kind + "\x00" + e.Title + "\x00" + e.Host + "\x00" + e.Created + "\x00" + e.Evidence + "\x00" + string(body)))
	return now.Format("20060102") + "-" + hex.EncodeToString(h[:4]) + ".yaml"
}

// ErrExists 같은 이름의 규칙 파일이 이미 있음.
var ErrExists = errors.New("같은 이름의 규칙 파일이 이미 있습니다")

// Write 검증 후 rules.d 에 새 파일로 쓴다 (임시 파일 → rename, 0600). 반환: 파일 경로.
func Write(dir string, e Entry, now time.Time) (string, error) {
	if e.Created == "" {
		e.Created = now.Format(time.RFC3339)
	}
	if e.Host == "" {
		e.Host, _ = os.Hostname()
	}
	if probs := Validate(e); len(probs) > 0 {
		return "", fmt.Errorf("규칙 형식 오류: %s", strings.Join(probs, "; "))
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, FileName(e, now))
	if _, err := os.Stat(path); err == nil {
		return "", fmt.Errorf("%w: %s", ErrExists, path)
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(e); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, ".rule-*.tmp")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	_, werr := f.Write(buf.Bytes())
	cerr := f.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp)
		return "", fmt.Errorf("규칙 저장 실패: %v %v", werr, cerr)
	}
	_ = os.Chmod(tmp, 0o600)
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return "", err
	}
	return path, nil
}

// Parse YAML 한 항목을 읽는다 (rules add 입력). 모르는 최상위 키는 종류별 본문으로 들어간다.
func Parse(data []byte) (Entry, error) {
	var e Entry
	if err := yaml.Unmarshal(data, &e); err != nil {
		return e, fmt.Errorf("YAML 오류: %w", err)
	}
	if e.Kind == "" && e.Title == "" && len(e.Body) == 0 {
		return e, errors.New("입력이 비어 있습니다")
	}
	for _, k := range []string{"source", "file"} { // 출력(list -f json) 키가 섞여 들어온 경우
		delete(e.Body, k)
	}
	return e, nil
}

// Similar 같은 종류에서 제목(공백·대소문자 무시)이 같거나 같은 표를 가리키는 기존 항목.
func Similar(e Entry, existing []Entry) []Entry {
	var out []Entry
	t, _ := e.Body["table"].(string)
	for _, x := range existing {
		if x.Kind != e.Kind {
			continue
		}
		xt, _ := x.Body["table"].(string)
		if norm(x.Title) == norm(e.Title) || (t != "" && strings.EqualFold(t, xt)) {
			out = append(out, x)
		}
	}
	return out
}

// FileProblem 파일 하나의 문제.
type FileProblem struct {
	File     string
	Problems []string
}

// LoadDir 로컬 규칙 디렉토리를 읽는다. 읽을 수 없거나 형식이 틀린 파일은 problems 로 돌려준다.
func LoadDir(dir string) ([]Entry, []FileProblem) {
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, nil // 디렉토리 없음 = 로컬 규칙 없음
	}
	var out []Entry
	var probs []FileProblem
	for _, n := range names {
		name := n.Name()
		if n.IsDir() || strings.HasPrefix(name, ".") || strings.ToLower(filepath.Ext(name)) != ".yaml" {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			probs = append(probs, FileProblem{path, []string{err.Error()}})
			continue
		}
		var e Entry
		if err := yaml.Unmarshal(data, &e); err != nil {
			probs = append(probs, FileProblem{path, []string{"YAML 오류: " + err.Error()}})
			continue
		}
		e.Source, e.File = "local", path
		var p []string
		if !fileNameRe.MatchString(name) {
			p = append(p, "파일 이름은 YYYYMMDD-<8hex>.yaml (ASCII) 이어야 함")
		}
		p = append(p, Validate(e)...)
		if len(p) > 0 {
			probs = append(probs, FileProblem{path, p})
		}
		out = append(out, e)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, probs
}

// LoadShipped 배포 rules.yaml 을 항목 목록으로 읽는다. 절마다 kind 를 붙이고, 각 항목의
// 공통 필드(verified·evidence·confidence)를 Entry 로 올린다. title 은 id·name·when 순으로 정한다.
func LoadShipped(path string) ([]Entry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	var out []Entry
	for _, section := range []string{"rules", "tables", "concepts", "breakdowns", "crosswalks", "gaps"} {
		items, _ := doc[section].([]any)
		for _, it := range items {
			m, ok := it.(map[string]any)
			if !ok {
				continue
			}
			e := Entry{Kind: sectionKind[section], Source: "shipped", File: path, Body: map[string]any{}}
			for k, v := range m {
				switch k {
				case "verified":
					e.Verified = fmt.Sprint(v)
				case "evidence":
					e.Evidence = fmt.Sprint(v)
				case "confidence":
					e.Confidence = fmt.Sprint(v)
				case "status":
					e.Status = fmt.Sprint(v)
				default:
					e.Body[k] = v
				}
			}
			if e.Kind == "table" { // 배포 tables 는 id 가 ORG/TBL — 로컬 형식(table 필드)과 맞춘다
				if id, ok := e.Body["id"].(string); ok {
					e.Body["table"] = id
				}
				if e.Evidence == "" {
					e.Evidence = "references/rules.yaml tables (m·d 실측)"
				}
			}
			e.Title = titleOf(e)
			out = append(out, e)
		}
	}
	return out, nil
}

func titleOf(e Entry) string {
	for _, k := range []string{"title", "name", "id", "when", "term"} {
		if v, ok := e.Body[k]; ok && v != nil {
			s := fmt.Sprint(v)
			if k == "name" {
				if id, ok := e.Body["id"].(string); ok {
					return id + " " + s
				}
			}
			return s
		}
	}
	return e.Kind
}

// MigrateLegacy 이전 형식 ~/.kosis/rules.local.yaml(절 rules/tables)을 rules.d 로 옮기고
// 원본을 rules.local.yaml.migrated 로 바꾼다. 반환: 옮긴 항목 수.
func MigrateLegacy(legacyPath, dir string, now time.Time) (int, error) {
	data, err := os.ReadFile(legacyPath)
	if err != nil {
		return 0, nil // 옮길 것 없음
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return 0, fmt.Errorf("%s 를 읽지 못해 옮기지 않았습니다: %w", legacyPath, err)
	}
	tmp := filepath.Join(filepath.Dir(legacyPath), ".rules-legacy-"+now.Format("150405")+".yaml")
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return 0, err
	}
	defer os.Remove(tmp)
	entries, err := LoadShipped(tmp) // 절 형식이 배포 파일과 같다
	if err != nil {
		return 0, err
	}
	host, _ := os.Hostname()
	n := 0
	for _, e := range entries {
		e.Source, e.File = "", ""
		e.Host = host
		if e.Evidence == "" {
			e.Evidence = "이전 rules.local.yaml 에서 옮김"
		}
		if e.Verified == "" && e.Confidence == "" {
			e.Confidence = "guess"
		}
		if _, err := Write(dir, e, now); err != nil && !errors.Is(err, ErrExists) {
			return n, fmt.Errorf("'%s' 옮기기 실패 (원본은 그대로 둡니다): %w", e.Title, err)
		}
		n++
	}
	if err := os.Rename(legacyPath, legacyPath+".migrated"); err != nil {
		return n, err
	}
	return n, nil
}

// Matches 찾을 말이 제목·본문에 있는지 (공백·대소문자 무시).
func Matches(e Entry, query string) bool {
	q := norm(query)
	if q == "" {
		return true
	}
	body, _ := yaml.Marshal(e.Body)
	return strings.Contains(norm(e.Title+" "+e.Evidence+" "+string(body)), q)
}

func norm(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
