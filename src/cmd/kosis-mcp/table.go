package main

// 큰 결과 다루기 (규약 3단계): 표(열 + 행) · 서버 집계(filter/latest/aggregate/pivot) · 형식(json/columns/csv) · 쪽 맞추기.

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

type column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

// table 행은 열 순서대로 문자열.
type table struct {
	Columns []column
	Rows    [][]string
}

func (t *table) index(name string) int {
	for i, c := range t.Columns {
		if c.Key == name {
			return i
		}
	}
	for i, c := range t.Columns {
		if c.Label == name {
			return i
		}
	}
	return -1
}

func (t *table) keys() []string {
	out := make([]string, len(t.Columns))
	for i, c := range t.Columns {
		out[i] = c.Key
	}
	return out
}

func (t *table) mustIndex(name, what string) (int, error) {
	if i := t.index(name); i >= 0 {
		return i, nil
	}
	return -1, fmt.Errorf("%s 의 열 %q 가 결과에 없습니다. 있는 열: %s", what, name, strings.Join(t.keys(), ", "))
}

// nameKey 코드 열의 이름 열 (C1 → C1_NM, ITM_ID → ITM_NM).
func nameKey(code string) string {
	if code == "ITM_ID" {
		return "ITM_NM"
	}
	if len(code) == 2 && code[0] == 'C' && code[1] >= '1' && code[1] <= '8' {
		return code + "_NM"
	}
	return ""
}

// ── 변환 ──────────────────────────────────────────────────────

type filterCond struct {
	In       map[string]bool
	From, To string
}

type aggSpec struct {
	GroupBy []string
	Op      string
	Value   string
}

type pivotSpec struct {
	Rows  []string
	Cols  string
	Value string
}

// transform 받아온 뒤 서버에서 줄이기. 순서: filter → latest → aggregate → pivot.
type transform struct {
	Filter map[string]filterCond
	Latest bool
	Agg    *aggSpec
	Pivot  *pivotSpec
}

func (tr transform) empty() bool {
	return len(tr.Filter) == 0 && !tr.Latest && tr.Agg == nil && tr.Pivot == nil
}

// apply 변환 결과 표와 안내 줄.
func (tr transform) apply(t *table) (*table, []string, error) {
	var notes []string
	var err error
	if len(tr.Filter) > 0 {
		if t, err = applyFilter(t, tr.Filter); err != nil {
			return nil, nil, err
		}
	}
	if tr.Latest {
		if t, err = applyLatest(t); err != nil {
			return nil, nil, err
		}
	}
	if tr.Agg != nil {
		var n []string
		if t, n, err = applyAggregate(t, *tr.Agg); err != nil {
			return nil, nil, err
		}
		notes = append(notes, n...)
	}
	if tr.Pivot != nil {
		var n []string
		if t, n, err = applyPivot(t, *tr.Pivot); err != nil {
			return nil, nil, err
		}
		notes = append(notes, n...)
	}
	return t, notes, nil
}

// cmpBound 시점처럼 길이가 다른 값: 경계 길이까지만 비교 ("202407" 은 "2024" 범위 안).
func cmpBound(v, bound string) int {
	if len(v) > len(bound) {
		v = v[:len(bound)]
	}
	return strings.Compare(v, bound)
}

func applyFilter(t *table, f map[string]filterCond) (*table, error) {
	type cond struct {
		i int
		c filterCond
	}
	var cs []cond
	names := make([]string, 0, len(f))
	for k := range f {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		i, err := t.mustIndex(k, "filter")
		if err != nil {
			return nil, err
		}
		cs = append(cs, cond{i, f[k]})
	}
	out := &table{Columns: t.Columns}
	for _, r := range t.Rows {
		ok := true
		for _, c := range cs {
			v := r[c.i]
			if c.c.In != nil && !c.c.In[v] {
				ok = false
			}
			if c.c.From != "" && cmpBound(v, c.c.From) < 0 {
				ok = false
			}
			if c.c.To != "" && cmpBound(v, c.c.To) > 0 {
				ok = false
			}
			if !ok {
				break
			}
		}
		if ok {
			out.Rows = append(out.Rows, r)
		}
	}
	return out, nil
}

// applyLatest 계열(시점·값 말고 모든 열이 같은 행)마다 가장 최근 시점(PRD_DE)의 행만.
func applyLatest(t *table) (*table, error) {
	p, err := t.mustIndex("PRD_DE", "latest")
	if err != nil {
		return nil, fmt.Errorf("latest 는 시점 열(PRD_DE)이 있어야 합니다: %v", err)
	}
	skip := map[string]bool{"PRD_DE": true, "DT": true, "LST_CHN_DE": true}
	seriesKey := func(r []string) string {
		var b strings.Builder
		for i, c := range t.Columns {
			if !skip[c.Key] {
				b.WriteString(r[i])
				b.WriteByte(0)
			}
		}
		return b.String()
	}
	last := map[string]string{}
	for _, r := range t.Rows {
		k := seriesKey(r)
		if r[p] > last[k] {
			last[k] = r[p]
		}
	}
	out := &table{Columns: t.Columns}
	for _, r := range t.Rows {
		if r[p] == last[seriesKey(r)] {
			out.Rows = append(out.Rows, r)
		}
	}
	return out, nil
}

// parseNum KOSIS 수치값 → 숫자 (쉼표 허용). "-", "X", "" 등은 false.
func parseNum(s string) (float64, bool) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
	return f, err == nil && !math.IsNaN(f) && !math.IsInf(f, 0)
}

func fmtNum(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// keyCols 묶을 열과, 코드 열이면 그 이름 열도 함께 (이름을 잃지 않게).
func keyCols(t *table, names []string, what string) ([]int, error) {
	var idx []int
	seen := map[int]bool{}
	for _, g := range names {
		i, err := t.mustIndex(g, what)
		if err != nil {
			return nil, err
		}
		if !seen[i] {
			idx, seen[i] = append(idx, i), true
		}
		if nk := nameKey(t.Columns[i].Key); nk != "" {
			if j := t.index(nk); j >= 0 && !seen[j] {
				idx, seen[j] = append(idx, j), true
			}
		}
	}
	return idx, nil
}

func applyAggregate(t *table, a aggSpec) (*table, []string, error) {
	gi, err := keyCols(t, a.GroupBy, "aggregate.group_by")
	if err != nil {
		return nil, nil, err
	}
	vi, err := t.mustIndex(a.Value, "aggregate.value")
	if err != nil {
		return nil, nil, err
	}
	type acc struct {
		key       []string
		n, rows   int
		sum       float64
		min, max  float64
		hasNumber bool
	}
	groups := map[string]*acc{}
	var order []string
	skipped := 0
	for _, r := range t.Rows {
		parts := make([]string, len(gi))
		for k, i := range gi {
			parts[k] = r[i]
		}
		k := strings.Join(parts, "\x00")
		g := groups[k]
		if g == nil {
			g = &acc{key: parts}
			groups[k] = g
			order = append(order, k)
		}
		g.rows++
		f, ok := parseNum(r[vi])
		if !ok {
			skipped++
			continue
		}
		if !g.hasNumber || f < g.min {
			g.min = f
		}
		if !g.hasNumber || f > g.max {
			g.max = f
		}
		g.hasNumber = true
		g.n++
		g.sum += f
	}
	vc := t.Columns[vi]
	out := &table{}
	for _, i := range gi {
		out.Columns = append(out.Columns, t.Columns[i])
	}
	out.Columns = append(out.Columns, column{Key: vc.Key + "_" + a.Op, Label: fmt.Sprintf("%s(%s)", vc.Label, a.Op)})
	for _, k := range order {
		g := groups[k]
		val := ""
		switch a.Op {
		case "count":
			val = strconv.Itoa(g.rows)
		case "sum":
			if g.hasNumber {
				val = fmtNum(g.sum)
			}
		case "avg":
			if g.hasNumber {
				val = fmtNum(g.sum / float64(g.n))
			}
		case "min":
			if g.hasNumber {
				val = fmtNum(g.min)
			}
		case "max":
			if g.hasNumber {
				val = fmtNum(g.max)
			}
		}
		out.Rows = append(out.Rows, append(append([]string{}, g.key...), val))
	}
	var notes []string
	if skipped > 0 && a.Op != "count" {
		notes = append(notes, fmt.Sprintf("aggregate: 숫자가 아닌 %s 값 %d개(-, X 등)는 건너뛰었습니다", a.Value, skipped))
	}
	return out, notes, nil
}

func applyPivot(t *table, p pivotSpec) (*table, []string, error) {
	ri, err := keyCols(t, p.Rows, "pivot.rows")
	if err != nil {
		return nil, nil, err
	}
	ci, err := t.mustIndex(p.Cols, "pivot.cols")
	if err != nil {
		return nil, nil, err
	}
	vi, err := t.mustIndex(p.Value, "pivot.value")
	if err != nil {
		return nil, nil, err
	}
	colSet := map[string]bool{}
	var colVals []string
	type rowAcc struct {
		key   []string
		cells map[string]string
	}
	rows := map[string]*rowAcc{}
	var order []string
	dups := 0
	for _, r := range t.Rows {
		parts := make([]string, len(ri))
		for k, i := range ri {
			parts[k] = r[i]
		}
		k := strings.Join(parts, "\x00")
		ra := rows[k]
		if ra == nil {
			ra = &rowAcc{key: parts, cells: map[string]string{}}
			rows[k] = ra
			order = append(order, k)
		}
		cv := r[ci]
		if !colSet[cv] {
			colSet[cv] = true
			colVals = append(colVals, cv)
		}
		if _, ok := ra.cells[cv]; ok {
			dups++
			continue
		}
		ra.cells[cv] = r[vi]
	}
	sort.Strings(colVals)
	out := &table{}
	for _, i := range ri {
		out.Columns = append(out.Columns, t.Columns[i])
	}
	for _, cv := range colVals {
		out.Columns = append(out.Columns, column{Key: cv, Label: cv})
	}
	for _, k := range order {
		ra := rows[k]
		row := append([]string{}, ra.key...)
		for _, cv := range colVals {
			row = append(row, ra.cells[cv])
		}
		out.Rows = append(out.Rows, row)
	}
	var notes []string
	if dups > 0 {
		notes = append(notes, fmt.Sprintf("pivot: 같은 칸에 값이 %d번 겹쳐 첫 값만 남겼습니다 — rows 에 나눌 열을 더 넣으세요", dups))
	}
	return out, notes, nil
}

// ── 형식 ──────────────────────────────────────────────────────

var formats = []string{"columns", "csv", "json"}

// renderRows rows 를 형식별 값으로 (응답 객체의 칸들).
func renderRows(cols []column, rows [][]string, format string) []kv {
	switch format {
	case "csv":
		return []kv{{"labels", labelsOf(cols)}, {"csv", csvText(cols, rows)}}
	case "columns":
		// 코드 → 이름 사전: 코드 열과 이름 열이 둘 다 있고 이 쪽에서 코드마다 이름이 하나면, 이름 열을 빼고 dict 로
		drop := map[int]bool{}
		dict := map[string]map[string]string{}
		for i, c := range cols {
			nk := nameKey(c.Key)
			if nk == "" {
				continue
			}
			j := -1
			for k, c2 := range cols {
				if c2.Key == nk {
					j = k
				}
			}
			if j < 0 {
				continue
			}
			m := map[string]string{}
			ok := true
			for _, r := range rows {
				if old, seen := m[r[i]]; seen && old != r[j] {
					ok = false
					break
				}
				m[r[i]] = r[j]
			}
			if ok {
				dict[c.Key] = m
				drop[j] = true
			}
		}
		var keys []string
		for i, c := range cols {
			if !drop[i] {
				keys = append(keys, c.Key)
			}
		}
		out := make([][]string, len(rows))
		for n, r := range rows {
			row := make([]string, 0, len(keys))
			for i := range cols {
				if !drop[i] {
					row = append(row, r[i])
				}
			}
			out[n] = row
		}
		res := []kv{{"columns", keys}, {"labels", labelsOf(cols)}, {"rows", out}}
		if len(dict) > 0 {
			res = append(res, kv{"dict", dict})
		}
		return res
	default: // json
		objs := make([]*orderedObj, len(rows))
		for n, r := range rows {
			o := &orderedObj{}
			for i, c := range cols {
				o.kvs = append(o.kvs, kv{c.Key, r[i]})
			}
			objs[n] = o
		}
		return []kv{{"labels", labelsOf(cols)}, {"rows", objs}}
	}
}

func labelsOf(cols []column) *orderedObj {
	o := &orderedObj{}
	for _, c := range cols {
		o.kvs = append(o.kvs, kv{c.Key, c.Label})
	}
	return o
}

func csvText(cols []column, rows [][]string) string {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.Write(keysOf(cols))
	w.WriteAll(rows)
	return buf.String()
}

func keysOf(cols []column) []string {
	out := make([]string, len(cols))
	for i, c := range cols {
		out[i] = c.Key
	}
	return out
}

// ── 순서 있는 JSON (요약 칸을 맨 앞에) ──────────────────────────

type kv struct {
	K string
	V any
}

type orderedObj struct{ kvs []kv }

func (o *orderedObj) MarshalJSON() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, p := range o.kvs {
		if i > 0 {
			buf.WriteByte(',')
		}
		k, _ := json.Marshal(p.K)
		buf.Write(k)
		buf.WriteByte(':')
		var vb bytes.Buffer
		enc := json.NewEncoder(&vb)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(p.V); err != nil {
			return nil, err
		}
		buf.Write(bytes.TrimRight(vb.Bytes(), "\n"))
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func encodeKVs(kvs []kv) string { return encodeJSON(&orderedObj{kvs: kvs}) }

// page 한 쪽: head(요약 + 기타) 뒤에 rows[off:off+n] 을 형식대로. n 은 limit 와 상한 안에서 가장 크게.
// 반환: 텍스트, 담은 행 수.
func fitPage(head func(returned int, more bool) []kv, cols []column, rows [][]string, format string, limit int) (string, int) {
	build := func(n int) string {
		kvs := head(n, n < len(rows))
		kvs = append(kvs, renderRows(cols, rows[:n], format)...)
		return encodeKVs(kvs)
	}
	hi := len(rows)
	if limit > 0 && limit < hi {
		hi = limit
	}
	if hi > 200 { // 앞 200행으로 행당 크기를 어림해 탐색 범위를 줄인다 (15만 행을 통째로 인코딩하지 않게)
		per := (len(build(200)) - len(build(0))) / 200
		if per > 0 {
			hi = min(hi, maxResultBytes/per*2+200)
		}
	}
	if text := build(hi); len(text) <= maxResultBytes {
		return text, hi
	}
	lo := 0
	for lo < hi {
		mid := (lo + hi + 1) / 2
		if len(build(mid)) <= maxResultBytes {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	text := build(lo)
	if len(text) > maxResultBytes {
		cut := maxResultBytes - 200
		for cut > 0 && !utf8.RuneStart(text[cut]) {
			cut--
		}
		text = text[:cut] + "\n…\n잘림: 행 하나도 상한을 넘습니다. fields 로 열을 줄이세요"
	}
	return text, lo
}
