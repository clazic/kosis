// Package catalog 는 미리 수집한 KOSIS 통계표 목록(표 이름·주제 경로·분류 축·항목·수록 시점)을 읽어
// 로컬에서 찾는다. KOSIS 검색 API 는 표 이름 위주라 항목 이름(예: 적용요금별 '산업용')으로는 잘 걸리지 않는다.
//
// 파일: 한 줄에 표 하나인 JSON (gzip). 만드는 법: src/tools/build_catalog.py
//
//	{"o":"310","t":"TX_31002_A019","n":"판매전력량 추이","p":["에너지","한국전력통계",…],
//	 "a":["적용요금별","월별"],"i":["판매전력량 추이"],"c":["합계","산업용",…],"pr":[["년","1961","2025"]]}
//
// 찾는 순서: KOSIS_CATALOG_FILE → ~/.kosis/catalog.jsonl.gz. 없으면 로컬 검색을 하지 않는다.
package catalog

import (
	"bufio"
	"compress/gzip"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/clazic/kosis/internal/config"
)

// Table 카탈로그의 표 하나.
type Table struct {
	Org     string     `json:"o"`
	Tbl     string     `json:"t"`
	Name    string     `json:"n"`
	Path    []string   `json:"p"`
	Axes    []string   `json:"a"`
	Items   []string   `json:"i"`
	Codes   []string   `json:"c"`  // 분류 축의 위 단계 코드 이름 (축마다 몇 개)
	Periods [][]string `json:"pr"` // [주기, 시작, 끝]

	norm string // 찾기용 (이름·경로·축·항목·코드 이름, 공백 없이 소문자)
}

// Hit 찾은 표와 점수·맞은 곳.
type Hit struct {
	*Table
	Score   int
	Matched []string // 이름·경로가 아닌 곳에서 맞은 말 (항목 '산업용' 등)
}

// End 가장 늦은 수록 끝 시점 (없으면 "").
func (t *Table) End() string {
	end := ""
	for _, p := range t.Periods {
		if len(p) == 3 && cmpPrd(p[2], end) > 0 {
			end = p[2]
		}
	}
	return end
}

// Start 가장 이른 수록 시작 시점.
func (t *Table) Start() string {
	start := ""
	for _, p := range t.Periods {
		if len(p) == 3 && (start == "" || cmpPrd(p[1], start) < 0) {
			start = p[1]
		}
	}
	return start
}

// cmpPrd 시점 비교: 앞 4자리(연도) 먼저, 같으면 문자열.
func cmpPrd(a, b string) int {
	ya, yb := year(a), year(b)
	if ya != yb {
		if ya < yb {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

func year(s string) int {
	if len(s) < 4 {
		return 0
	}
	n, _ := strconv.Atoi(s[:4])
	return n
}

// Catalog 불러온 카탈로그.
type Catalog struct {
	Tables  []*Table
	File    string
	ModTime time.Time
}

var (
	once   sync.Once
	loaded *Catalog
)

// Path 카탈로그 파일 위치 (없으면 "").
func Path() string {
	if p := os.Getenv("KOSIS_CATALOG_FILE"); p != "" {
		if _, err := os.Stat(p); err == nil {
			return p
		}
		return ""
	}
	p := filepath.Join(config.ConfigDir(), "catalog.jsonl.gz")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// Default 이 프로세스에서 한 번 읽은 카탈로그 (없거나 못 읽으면 nil).
func Default() *Catalog {
	once.Do(func() {
		if p := Path(); p != "" {
			loaded, _ = Load(p)
		}
	})
	return loaded
}

// Load 파일을 읽는다 (.gz 면 압축 풀어서).
func Load(path string) (*Catalog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	st, _ := f.Stat()
	var r io.Reader = f
	if strings.HasSuffix(path, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, err
		}
		defer gz.Close()
		r = gz
	}
	c := &Catalog{File: path}
	if st != nil {
		c.ModTime = st.ModTime()
	}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	for sc.Scan() {
		var t Table
		if json.Unmarshal(sc.Bytes(), &t) != nil || t.Org == "" || t.Tbl == "" {
			continue
		}
		t.norm = squash(strings.Join([]string{t.Name, strings.Join(t.Path, " "), strings.Join(t.Axes, " "), strings.Join(t.Items, " "), strings.Join(t.Codes, " ")}, "\x00"))
		c.Tables = append(c.Tables, &t)
	}
	return c, sc.Err()
}

func squash(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), "")) }

// Search 검색어의 낱말(2글자 이상)이 모두 든 표를 점수 순으로 n 개까지.
// 점수: 표 이름에 든 낱말 10, 주제 경로 4, 항목 3, 코드 이름 2, 축 이름 2. 수록이 2년 넘게 멈춘 표는 −6.
// now 는 '멈춘 표' 판단 기준 (보통 time.Now()).
func (c *Catalog) Search(keyword string, n int, now time.Time) []Hit {
	if c == nil {
		return nil
	}
	var toks []string
	for _, w := range strings.Fields(keyword) {
		if utf8.RuneCountInString(w) >= 2 {
			toks = append(toks, strings.ToLower(w))
		}
	}
	if len(toks) == 0 {
		return nil
	}
	var hits []Hit
	for _, t := range c.Tables {
		ok := true
		for _, w := range toks {
			if !strings.Contains(t.norm, w) {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		h := Hit{Table: t}
		name, path := squash(t.Name), squash(strings.Join(t.Path, " "))
		for _, w := range toks {
			switch {
			case strings.Contains(name, w):
				h.Score += 10
			case strings.Contains(path, w):
				h.Score += 4
			case anyContains(t.Items, w):
				h.Score += 3
				h.Matched = append(h.Matched, "항목 "+firstContaining(t.Items, w))
			case anyContains(t.Codes, w):
				h.Score += 2
				h.Matched = append(h.Matched, "분류 "+firstContaining(t.Codes, w))
			case anyContains(t.Axes, w):
				h.Score += 2
				h.Matched = append(h.Matched, "축 "+firstContaining(t.Axes, w))
			}
		}
		if y := year(t.End()); y > 0 && y < now.Year()-2 {
			h.Score -= 6
		}
		hits = append(hits, h)
	}
	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return cmpPrd(hits[i].End(), hits[j].End()) > 0
	})
	if len(hits) > n {
		hits = hits[:n]
	}
	return hits
}

func anyContains(list []string, w string) bool { return firstContaining(list, w) != "" }

func firstContaining(list []string, w string) string {
	for _, s := range list {
		if strings.Contains(squash(s), w) {
			return strings.Join(strings.Fields(s), " ")
		}
	}
	return ""
}
