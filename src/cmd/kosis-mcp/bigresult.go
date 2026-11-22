package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/clazic/kosis/internal/api"
)

// runCtx 도구 하나를 실행할 때 쓰는 것들.
type runCtx struct {
	ctx     context.Context
	client  *api.Client // NoKeys 도구는 nil
	prog    *progress
	store   *resultStore // nil 이면 저장하지 않는다(첫 쪽만)
	tool    string
	argsRaw json.RawMessage
}

// toolResult 도구 결과: 본문 텍스트와 덧붙일 내용 항목(resource_link 등).
type toolResult struct {
	Text  string
	Extra []map[string]any
}

func textResult(s string) *toolResult { return &toolResult{Text: s} }

// argsSummary 결과 메타에 남길 인자 요약 (인자에는 키가 없다 — 키는 _meta·머리글로만 온다).
func (rc *runCtx) argsSummary() string {
	s := strings.TrimSpace(string(rc.argsRaw))
	if len(s) > 1000 {
		s = s[:1000] + "…"
	}
	return s
}

// respondTable 표 결과를 돌려준다.
//   - 작은 결과(상한 안)이고 save 가 아니면 바로 (형식 기본 json — 예전과 같음).
//   - 크거나 save 면 결과 저장소에 두고 첫 쪽(형식 기본 columns) + next_cursor + resource_link(CSV).
//
// 응답 맨 앞 요약 칸: total_rows, returned_rows, truncated, format, result_id, next_cursor, download.
func (rc *runCtx) respondTable(t *table, format string, save bool, limit int, head []kv) (*toolResult, error) {
	total := len(t.Rows)
	if !save {
		f := format
		if f == "" {
			f = "json"
		}
		if limit == 0 || limit >= total {
			kvs := append([]kv{{"total_rows", total}, {"returned_rows", total}, {"truncated", false}, {"format", f}}, head...)
			kvs = append(kvs, renderRows(t.Columns, t.Rows, f)...)
			if text := encodeKVs(kvs); len(text) <= maxResultBytes {
				return textResult(text), nil
			}
		}
	}
	f := format
	if f == "" {
		f = "columns"
	}
	if rc.store == nil { // 저장소 없이: 첫 쪽만
		text, _ := fitPage(func(n int, more bool) []kv {
			kvs := []kv{{"total_rows", total}, {"returned_rows", n}, {"truncated", more}, {"format", f}}
			if more {
				kvs = append(kvs, kv{"note", "잘림: 결과 저장소를 쓸 수 없어 나머지 쪽은 받을 수 없습니다. 범위를 좁히세요"})
			}
			return append(kvs, head...)
		}, t.Columns, t.Rows, f, limit)
		return textResult(text), nil
	}
	m, err := rc.store.save(rc.tool, rc.argsSummary(), t)
	if err != nil {
		return nil, err
	}
	text, _ := fitPage(pageHead(m, f, 0, head), t.Columns, t.Rows, f, limit)
	return &toolResult{Text: text, Extra: []map[string]any{resourceLink(m)}}, nil
}

// pageHead 저장된 결과 한 쪽의 요약 칸.
func pageHead(m *resultMeta, format string, offset int, head []kv) func(n int, more bool) []kv {
	return func(n int, more bool) []kv {
		more = offset+n < m.Rows
		kvs := []kv{{"total_rows", m.Rows}, {"returned_rows", n}, {"truncated", more}, {"format", format}, {"result_id", m.ID}}
		if more {
			kvs = append(kvs, kv{"next_cursor", makeCursor(m.ID, offset+n, format)})
		}
		kvs = append(kvs, kv{"offset", offset})
		kvs = append(kvs, kv{"download", map[string]any{
			"uri": resultScheme + m.ID + ".csv", "http_path": "/results/" + m.ID + ".csv", "size": m.Bytes, "rows": m.Rows,
		}})
		if more && offset == 0 {
			kvs = append(kvs, kv{"hint", "나머지는 kosis_result_page(cursor=next_cursor), 줄이려면 kosis_result_query(result_id, latest/filter/aggregate/pivot), 전체는 download(CSV)"})
		}
		return append(kvs, head...)
	}
}

// ── kosis_result_page · kosis_result_query ────────────────────

func runResultPage(rc *runCtx, in args) (*toolResult, error) {
	if rc.store == nil {
		return nil, errors.New("결과 저장소를 쓸 수 없습니다")
	}
	id, offset, format, err := parseCursor(in.str("cursor"))
	if err != nil {
		return nil, err
	}
	m, err := rc.store.meta(id)
	if err != nil {
		return nil, err
	}
	limit := in.num("limit")
	cap := limit
	if cap == 0 {
		cap = 100000
	}
	rows, err := rc.store.readRows(id, offset, cap)
	if err != nil {
		return nil, err
	}
	text, _ := fitPage(pageHead(m, format, offset, nil), m.Columns, rows, format, limit)
	return &toolResult{Text: text, Extra: []map[string]any{resourceLink(m)}}, nil
}

func runResultQuery(rc *runCtx, in args) (*toolResult, error) {
	if rc.store == nil {
		return nil, errors.New("결과 저장소를 쓸 수 없습니다")
	}
	m, t, err := rc.store.load(in.str("result_id"))
	if err != nil {
		return nil, err
	}
	tr, err := transformArgs(in)
	if err != nil {
		return nil, err
	}
	before := len(t.Rows)
	t, notes, err := tr.apply(t)
	if err != nil {
		return nil, err
	}
	head := []kv{{"source_result_id", m.ID}, {"source_rows", before}}
	if len(notes) > 0 {
		head = append(head, kv{"notices", notes})
	}
	return rc.respondTable(t, in.str("format"), in.flag("save"), in.num("limit"), head)
}

// ── 인자 검사 (객체 인자) ─────────────────────────────────────

func strList(v any, what string, maxN int) ([]string, error) {
	arr, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%s 는 문자열 배열이어야 합니다", what)
	}
	if len(arr) > maxN {
		return nil, fmt.Errorf("%s 는 %d개까지입니다", what, maxN)
	}
	var out []string
	for _, x := range arr {
		switch s := x.(type) {
		case string:
			if s = strings.TrimSpace(s); s == "" || len(s) > 200 || hasControl(s) {
				return nil, fmt.Errorf("%s 의 값 형식이 맞지 않습니다", what)
			}
			out = append(out, s)
		case json.Number:
			out = append(out, s.String())
		default:
			return nil, fmt.Errorf("%s 는 문자열 배열이어야 합니다", what)
		}
	}
	return out, nil
}

func colName(v any, what string) (string, error) {
	s, ok := v.(string)
	if s = strings.TrimSpace(s); !ok || s == "" || len(s) > 60 || hasControl(s) {
		return "", fmt.Errorf("%s 는 열 이름(예: C1, ITM_ID, PRD_DE, DT)이어야 합니다", what)
	}
	return s, nil
}

func checkObject(v any) (map[string]any, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("객체여야 합니다")
	}
	return m, nil
}

func onlyKeys(m map[string]any, allowed ...string) error {
	for k := range m {
		ok := false
		for _, a := range allowed {
			ok = ok || a == k
		}
		if !ok {
			return fmt.Errorf("모르는 칸 %q (쓸 수 있는 칸: %s)", k, strings.Join(allowed, ", "))
		}
	}
	return nil
}

// checkLatest kosis_data latest: 정수 N(표의 최근 N개 시점을 받음) 또는 true(받은 뒤 계열마다 가장 최근 시점만).
func checkLatest(v any) (any, error) {
	switch x := v.(type) {
	case bool:
		if !x {
			return nil, nil
		}
		return true, nil
	case json.Number:
		n, err := x.Int64()
		if err != nil || n < 1 || n > 1000 {
			return nil, errors.New("정수 1~1000 또는 true 여야 합니다")
		}
		return int(n), nil
	}
	return nil, errors.New("정수 1~1000 또는 true 여야 합니다")
}

func checkLatestBool(v any) (any, error) {
	b, ok := v.(bool)
	if !ok {
		return nil, errors.New("true/false 여야 합니다")
	}
	if !b {
		return nil, nil
	}
	return true, nil
}

func checkFilter(v any) (any, error) {
	m, err := checkObject(v)
	if err != nil {
		return nil, err
	}
	if len(m) > 20 {
		return nil, errors.New("열은 20개까지입니다")
	}
	out := map[string]filterCond{}
	for k, c := range m {
		if _, err := colName(k, "filter 의 열"); err != nil {
			return nil, err
		}
		switch x := c.(type) {
		case []any:
			list, err := strList(x, "filter."+k, 5000)
			if err != nil {
				return nil, err
			}
			in := map[string]bool{}
			for _, s := range list {
				in[s] = true
			}
			out[k] = filterCond{In: in}
		case map[string]any:
			if err := onlyKeys(x, "from", "to"); err != nil {
				return nil, fmt.Errorf("filter.%s: %v", k, err)
			}
			var fc filterCond
			for name, dst := range map[string]*string{"from": &fc.From, "to": &fc.To} {
				if b, ok := x[name]; ok {
					s, ok := b.(string)
					if n, isNum := b.(json.Number); isNum {
						s, ok = n.String(), true
					}
					if !ok || len(s) > 40 || hasControl(s) {
						return nil, fmt.Errorf("filter.%s.%s 는 문자열이어야 합니다", k, name)
					}
					*dst = strings.TrimSpace(s)
				}
			}
			if fc.From == "" && fc.To == "" {
				return nil, fmt.Errorf("filter.%s 에 from 이나 to 가 필요합니다", k)
			}
			out[k] = fc
		default:
			return nil, fmt.Errorf("filter.%s 는 값 배열([\"11\",\"26\"]) 또는 범위({\"from\":\"2020\",\"to\":\"2024\"})여야 합니다", k)
		}
	}
	return out, nil
}

var aggOps = []string{"sum", "avg", "min", "max", "count"}

func checkAggregate(v any) (any, error) {
	m, err := checkObject(v)
	if err != nil {
		return nil, err
	}
	if err := onlyKeys(m, "group_by", "op", "value"); err != nil {
		return nil, err
	}
	a := &aggSpec{Value: "DT"}
	if g, ok := m["group_by"]; ok {
		if a.GroupBy, err = strList(g, "group_by", 10); err != nil {
			return nil, err
		}
	}
	op, _ := m["op"].(string)
	for _, o := range aggOps {
		if strings.EqualFold(o, op) {
			a.Op = o
		}
	}
	if a.Op == "" {
		return nil, fmt.Errorf("op 는 %s 중 하나여야 합니다", strings.Join(aggOps, ", "))
	}
	if val, ok := m["value"]; ok {
		if a.Value, err = colName(val, "value"); err != nil {
			return nil, err
		}
	}
	return a, nil
}

func checkPivot(v any) (any, error) {
	m, err := checkObject(v)
	if err != nil {
		return nil, err
	}
	if err := onlyKeys(m, "rows", "cols", "value"); err != nil {
		return nil, err
	}
	p := &pivotSpec{Cols: "PRD_DE", Value: "DT"}
	if r, ok := m["rows"]; ok {
		if p.Rows, err = strList(r, "rows", 10); err != nil {
			return nil, err
		}
	}
	if len(p.Rows) == 0 {
		return nil, errors.New("rows(행으로 둘 열, 예 [\"C1\"]) 가 필요합니다")
	}
	if c, ok := m["cols"]; ok {
		if p.Cols, err = colName(c, "cols"); err != nil {
			return nil, err
		}
	}
	if val, ok := m["value"]; ok {
		if p.Value, err = colName(val, "value"); err != nil {
			return nil, err
		}
	}
	return p, nil
}

// transformArgs 검사된 인자 → 변환.
func transformArgs(in args) (transform, error) {
	var tr transform
	if f, ok := in["filter"].(map[string]filterCond); ok {
		tr.Filter = f
	}
	tr.Latest = in.flag("latest")
	if a, ok := in["aggregate"].(*aggSpec); ok {
		tr.Agg = a
	}
	if p, ok := in["pivot"].(*pivotSpec); ok {
		tr.Pivot = p
	}
	// order·order_by 는 kosis_result_query 에만 있다 (kosis_data 의 sort·top 은 조회 쪽에서 먼저 계산)
	if o := in.str("order"); o != "" {
		tr.Order = &orderSpec{By: in.str("order_by"), Desc: o == "desc", Top: in.num("top")}
	} else if in.str("order_by") != "" {
		return tr, fmt.Errorf("order_by 는 order(desc|asc)와 함께 씁니다")
	}
	return tr, nil
}

// 공통 인자 정의 (kosis_data · kosis_result_query)
var (
	schemaFilter = map[string]any{
		"type": "object",
		"additionalProperties": map[string]any{"oneOf": []any{
			map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 5000},
			map[string]any{"type": "object", "properties": map[string]any{"from": map[string]any{"type": "string"}, "to": map[string]any{"type": "string"}}, "additionalProperties": false},
		}},
	}
	schemaAggregate = map[string]any{
		"type": "object", "required": []string{"op"}, "additionalProperties": false,
		"properties": map[string]any{
			"group_by": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "maxItems": 10},
			"op":       map[string]any{"type": "string", "enum": aggOps},
			"value":    map[string]any{"type": "string", "default": "DT"},
		},
	}
	schemaPivot = map[string]any{
		"type": "object", "required": []string{"rows"}, "additionalProperties": false,
		"properties": map[string]any{
			"rows":  map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1, "maxItems": 10},
			"cols":  map[string]any{"type": "string", "default": "PRD_DE"},
			"value": map[string]any{"type": "string", "default": "DT"},
		},
	}
)

func bigResultParams() []*param {
	return []*param{
		{Name: "format", Type: "string", Enum: formats, Desc: "결과 형식: columns=열 이름 + 행 배열 + dict(코드→이름, 이름 열 대신) — 큰 결과 기본·가장 촘촘, csv=머리줄+행, json=객체 배열(작은 결과 기본)"},
		{Name: "save", Type: "boolean", Desc: "true 면 작아도 결과를 저장해 result_id·CSV 내려받기(resource_link)를 준다"},
		{Name: "filter", Schema: schemaFilter, Check: checkFilter, Desc: "받은 뒤 거르기: {\"C1\":[\"11\",\"26\"], \"PRD_DE\":{\"from\":\"2020\",\"to\":\"2024\"}} (열은 API 키 또는 열 이름)"},
		{Name: "aggregate", Schema: schemaAggregate, Check: checkAggregate, Desc: "묶어 계산: {\"group_by\":[\"C1\",\"PRD_DE\"], \"op\":\"sum|avg|min|max|count\", \"value\":\"DT\"} — 숫자 아닌 값(-, X)은 건너뛰고 알린다. 코드 열을 묶으면 이름 열도 함께"},
		{Name: "pivot", Schema: schemaPivot, Check: checkPivot, Desc: "넓은 표로: {\"rows\":[\"C1\"], \"cols\":\"PRD_DE\", \"value\":\"DT\"}"},
	}
}
