package main

// 결과 저장소: 큰 결과를 디스크에 <id>.jsonl(쪽 나누기) · <id>.csv(내려받기) · <id>.meta.json 으로 둔다.
// id 는 무작위 144bit(base64url) — 맞히기 불가 = 읽기 권한. 키·토큰은 넣지 않는다(공개 통계 행만).
// 여러 프로세스가 같은 폴더를 써도 되게 쓰기는 임시 파일 → rename.

import (
	"bufio"
	"crypto/rand"
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	defaultResultsTTL      = time.Hour
	defaultResultsMaxBytes = 1 << 30
	resultsCleanEvery      = 10 * time.Minute
	resultScheme           = "kosis://results/"
)

// maxResourceReadBytes resources/read 로 돌려줄 파일 상한 (넘으면 내려받기를 쓰라고 오류).
var maxResourceReadBytes int64 = 20 << 20

var resultIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{22,64}$`)

// errExpired 저장된 결과가 없거나 지났다.
var errExpired = errors.New("결과가 만료됐어요 — 다시 조회하세요")

type resultMeta struct {
	ID          string   `json:"id"`
	Tool        string   `json:"tool"`
	Created     string   `json:"created"`
	Rows        int      `json:"rows"`
	Columns     []column `json:"columns"`
	Bytes       int64    `json:"bytes"`
	Formats     []string `json:"formats"`
	ArgsSummary string   `json:"args_summary"`
}

type resultStore struct {
	dir      string
	ttl      time.Duration
	maxBytes int64
	now      func() time.Time
}

// resultStoreFromEnv MCP_RESULTS_DIR(없으면 <캐시 폴더>/mcp-results), MCP_RESULTS_TTL(1h, 초 또는 Go 기간), MCP_RESULTS_MAX_BYTES(1GB).
func resultStoreFromEnv(cacheDir string) (*resultStore, error) {
	dir := os.Getenv("MCP_RESULTS_DIR")
	if dir == "" {
		dir = filepath.Join(cacheDir, "mcp-results")
	}
	ttl := defaultResultsTTL
	if v := strings.TrimSpace(os.Getenv("MCP_RESULTS_TTL")); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			ttl = time.Duration(n) * time.Second
		} else if d, err := time.ParseDuration(v); err == nil && d > 0 {
			ttl = d
		} else {
			return nil, fmt.Errorf("MCP_RESULTS_TTL 형식이 맞지 않습니다 (초 또는 1h 같은 기간): %q", v)
		}
	}
	maxBytes := int64(defaultResultsMaxBytes)
	if v := strings.TrimSpace(os.Getenv("MCP_RESULTS_MAX_BYTES")); v != "" {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil || n <= 0 {
			return nil, fmt.Errorf("MCP_RESULTS_MAX_BYTES 는 양의 정수(바이트)여야 합니다: %q", v)
		}
		maxBytes = n
	}
	return newResultStore(dir, ttl, maxBytes)
}

func newResultStore(dir string, ttl time.Duration, maxBytes int64) (*resultStore, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("결과 폴더를 만들지 못했습니다: %w", err)
	}
	return &resultStore{dir: dir, ttl: ttl, maxBytes: maxBytes, now: time.Now}, nil
}

func newResultID() string {
	b := make([]byte, 18) // 144bit
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (st *resultStore) path(id, ext string) string { return filepath.Join(st.dir, id+"."+ext) }

// writeAtomic 임시 파일에 쓰고 rename.
func (st *resultStore) writeAtomic(final string, fill func(f *os.File) error) (int64, error) {
	tmp, err := os.CreateTemp(st.dir, ".tmp-*")
	if err != nil {
		return 0, err
	}
	defer os.Remove(tmp.Name())
	if err := fill(tmp); err != nil {
		tmp.Close()
		return 0, err
	}
	info, err := tmp.Stat()
	if err != nil {
		tmp.Close()
		return 0, err
	}
	if err := tmp.Close(); err != nil {
		return 0, err
	}
	if err := os.Chmod(tmp.Name(), 0o600); err != nil {
		return 0, err
	}
	return info.Size(), os.Rename(tmp.Name(), final)
}

// save 표를 저장하고 메타를 돌려준다. meta.json 을 마지막에 써서, 메타가 보이면 파일이 다 있다.
func (st *resultStore) save(tool, argsSummary string, t *table) (*resultMeta, error) {
	id := newResultID()
	_, err := st.writeAtomic(st.path(id, "jsonl"), func(f *os.File) error {
		w := bufio.NewWriterSize(f, 1<<16)
		enc := json.NewEncoder(w)
		enc.SetEscapeHTML(false)
		for _, r := range t.Rows {
			if err := enc.Encode(r); err != nil {
				return err
			}
		}
		return w.Flush()
	})
	if err != nil {
		return nil, fmt.Errorf("결과 저장 실패: %w", err)
	}
	csvBytes, err := st.writeAtomic(st.path(id, "csv"), func(f *os.File) error {
		w := bufio.NewWriterSize(f, 1<<16)
		w.WriteString("\xEF\xBB\xBF") // 엑셀에서 한글이 깨지지 않게 (UTF-8 BOM)
		cw := csv.NewWriter(w)
		cw.Write(keysOf(t.Columns))
		if err := cw.WriteAll(t.Rows); err != nil {
			return err
		}
		return w.Flush()
	})
	if err != nil {
		os.Remove(st.path(id, "jsonl"))
		return nil, fmt.Errorf("결과 저장 실패: %w", err)
	}
	m := &resultMeta{ID: id, Tool: tool, Created: st.now().UTC().Format(time.RFC3339), Rows: len(t.Rows),
		Columns: t.Columns, Bytes: csvBytes, Formats: []string{"jsonl", "csv"}, ArgsSummary: argsSummary}
	if _, err := st.writeAtomic(st.path(id, "meta.json"), func(f *os.File) error {
		return json.NewEncoder(f).Encode(m)
	}); err != nil {
		os.Remove(st.path(id, "jsonl"))
		os.Remove(st.path(id, "csv"))
		return nil, fmt.Errorf("결과 저장 실패: %w", err)
	}
	return m, nil
}

// meta 저장된 결과의 메타. 없거나 TTL 이 지났으면 errExpired (지난 것은 지운다).
func (st *resultStore) meta(id string) (*resultMeta, error) {
	if !resultIDRe.MatchString(id) {
		return nil, errExpired
	}
	b, err := os.ReadFile(st.path(id, "meta.json"))
	if err != nil {
		return nil, errExpired
	}
	var m resultMeta
	if json.Unmarshal(b, &m) != nil {
		return nil, errExpired
	}
	if created, err := time.Parse(time.RFC3339, m.Created); err != nil || st.now().Sub(created) > st.ttl {
		st.remove(id)
		return nil, errExpired
	}
	return &m, nil
}

// readRows offset 부터 최대 n 행 (n<=0 이면 끝까지).
func (st *resultStore) readRows(id string, offset, n int) ([][]string, error) {
	f, err := os.Open(st.path(id, "jsonl"))
	if err != nil {
		return nil, errExpired
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	var out [][]string
	for i := 0; sc.Scan(); i++ {
		if i < offset {
			continue
		}
		var r []string
		if err := json.Unmarshal(sc.Bytes(), &r); err != nil {
			return nil, fmt.Errorf("저장된 결과가 손상됐습니다: %w", err)
		}
		out = append(out, r)
		if n > 0 && len(out) >= n {
			break
		}
	}
	return out, sc.Err()
}

func (st *resultStore) load(id string) (*resultMeta, *table, error) {
	m, err := st.meta(id)
	if err != nil {
		return nil, nil, err
	}
	rows, err := st.readRows(id, 0, 0)
	if err != nil {
		return nil, nil, err
	}
	return m, &table{Columns: m.Columns, Rows: rows}, nil
}

func (st *resultStore) remove(id string) {
	for _, ext := range []string{"meta.json", "jsonl", "csv"} {
		os.Remove(st.path(id, ext))
	}
}

// clean TTL 이 지난 결과를 지우고, 폴더가 maxBytes 를 넘으면 오래된 것부터 지운다.
func (st *resultStore) clean() {
	entries, err := os.ReadDir(st.dir)
	if err != nil {
		return
	}
	type group struct {
		id      string
		size    int64
		created time.Time
	}
	groups := map[string]*group{}
	var total int64
	now := st.now()
	for _, e := range entries {
		info, err := e.Info()
		if err != nil || e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".tmp-") { // 죽은 프로세스가 남긴 임시 파일
			if now.Sub(info.ModTime()) > time.Hour {
				os.Remove(filepath.Join(st.dir, name))
			}
			continue
		}
		id, _, ok := strings.Cut(name, ".")
		if !ok || !resultIDRe.MatchString(id) {
			continue
		}
		g := groups[id]
		if g == nil {
			g = &group{id: id, created: info.ModTime()}
			groups[id] = g
		}
		g.size += info.Size()
		total += info.Size()
		if info.ModTime().Before(g.created) {
			g.created = info.ModTime()
		}
	}
	var list []*group
	for _, g := range groups {
		if b, err := os.ReadFile(st.path(g.id, "meta.json")); err == nil {
			var m resultMeta
			if json.Unmarshal(b, &m) == nil {
				if c, err := time.Parse(time.RFC3339, m.Created); err == nil {
					g.created = c
				}
			}
		}
		if now.Sub(g.created) > st.ttl {
			st.remove(g.id)
			total -= g.size
			continue
		}
		list = append(list, g)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].created.Before(list[j].created) })
	for _, g := range list {
		if total <= st.maxBytes {
			break
		}
		st.remove(g.id)
		total -= g.size
	}
}

// runCleaner 시작할 때와 10분마다 정리.
func (st *resultStore) runCleaner() {
	st.clean()
	go func() {
		for range time.Tick(resultsCleanEvery) {
			st.clean()
		}
	}()
}

// ── 쪽 커서 ───────────────────────────────────────────────────

func makeCursor(id string, offset int, format string) string {
	return base64.RawURLEncoding.EncodeToString([]byte("v1:" + id + ":" + strconv.Itoa(offset) + ":" + format))
}

func parseCursor(c string) (id string, offset int, format string, err error) {
	b, derr := base64.RawURLEncoding.DecodeString(c)
	parts := strings.Split(string(b), ":")
	if derr != nil || len(parts) != 4 || parts[0] != "v1" || !resultIDRe.MatchString(parts[1]) {
		return "", 0, "", errors.New("cursor 형식이 맞지 않습니다 — 앞 응답의 next_cursor 를 그대로 넣으세요")
	}
	offset, err = strconv.Atoi(parts[2])
	if err != nil || offset < 0 {
		return "", 0, "", errors.New("cursor 형식이 맞지 않습니다")
	}
	ok := false
	for _, f := range formats {
		ok = ok || f == parts[3]
	}
	if !ok {
		return "", 0, "", errors.New("cursor 형식이 맞지 않습니다")
	}
	return parts[1], offset, parts[3], nil
}

// resourceLink 저장된 결과의 CSV 를 알리는 MCP resource_link 내용 항목.
func resourceLink(m *resultMeta) map[string]any {
	return map[string]any{
		"type": "resource_link", "uri": resultScheme + m.ID + ".csv", "name": m.ID + ".csv",
		"mimeType": "text/csv", "size": m.Bytes, "description": fmt.Sprintf("전체 %d행 (CSV, UTF-8)", m.Rows),
	}
}

// parseResultURI kosis://results/<id>.<ext> 또는 /results/<id>.<ext> → id, ext (csv·jsonl 만).
func parseResultName(name string) (id, ext string, ok bool) {
	id, ext, found := strings.Cut(name, ".")
	if !found || !resultIDRe.MatchString(id) || (ext != "csv" && ext != "jsonl") {
		return "", "", false
	}
	return id, ext, true
}

func mimeOf(ext string) string {
	if ext == "csv" {
		return "text/csv; charset=utf-8"
	}
	return "application/jsonl"
}
