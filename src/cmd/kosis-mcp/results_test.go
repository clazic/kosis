package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/clazic/kosis/internal/api"
)

// bigTable 시군구 × 항목 × 월 모양의 표 (코드 → 이름이 되풀이된다).
func bigTable(n int) *table {
	t := &table{Columns: []column{
		{"C1", "행정구역 코드"}, {"C1_NM", "행정구역"}, {"ITM_ID", "항목 코드"}, {"ITM_NM", "항목"},
		{"PRD_DE", "시점"}, {"DT", "수치값"}, {"UNIT_NM", "단위"},
	}}
	for i := 0; i < n; i++ {
		c := fmt.Sprintf("%05d", i%400)
		it := []string{"T20", "T21", "T22"}[i%3]
		t.Rows = append(t.Rows, []string{c, "지역이름이꽤긴곳" + c, it, map[string]string{"T20": "총인구수", "T21": "남자인구수", "T22": "여자인구수"}[it],
			fmt.Sprintf("%04d%02d", 2011+(i/1200)%15, 1+(i/400)%12), fmt.Sprint(1000 + i), "명"})
	}
	return t
}

// tableTool 네트워크 없이 표를 내는 가짜 도구 (kosis_data 와 같은 respondTable·변환 경로).
func tableTool(t *table) *tool {
	return &tool{Name: "test_table", Description: "test", NoKeys: true, Params: append([]*param{{Name: "n", Type: "integer", Min: 1, Max: 1000000}}, bigResultParams()...),
		run: func(rc *runCtx, in args) (*toolResult, error) {
			tt := t
			if n := in.num("n"); n > 0 {
				tt = bigTable(n)
			}
			tr, _ := transformArgs(in)
			if !tr.empty() {
				var err error
				if tt, _, err = tr.apply(tt); err != nil {
					return nil, err
				}
			}
			return rc.respondTable(tt, in.str("format"), in.flag("save"), 0, []kv{{"org_id", "101"}})
		}}
}

func testServerWithStore(t *testing.T, keys []string, extra ...*tool) (*server, *resultStore) {
	t.Helper()
	st, err := newResultStore(t.TempDir(), time.Hour, 1<<30)
	if err != nil {
		t.Fatal(err)
	}
	s := newServer(nil, "dev", keys, nil)
	s.results = st
	s.tools = append(append([]*tool{}, tools...), extra...)
	s.newClient = func(k []string) (*api.Client, error) { return api.NewClient(k) }
	return s, st
}

// call 도구 하나를 stdio 로 부르고 결과 객체·추가 내용 항목.
func call(t *testing.T, s *server, name string, argsJSON string, meta string) (map[string]any, []any, bool) {
	t.Helper()
	if meta == "" {
		meta = "{}"
	}
	got := runSession(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s,"_meta":%s}}`, name, argsJSON, meta))
	res := got["1"]["result"].(map[string]any)
	content := res["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	isErr := res["isError"] == true
	var obj map[string]any
	if !isErr {
		if err := json.Unmarshal([]byte(text), &obj); err != nil {
			t.Fatalf("결과가 JSON 이 아님: %v (%.200s)", err, text)
		}
	} else {
		obj = map[string]any{"error": text}
	}
	return obj, content[1:], isErr
}

// decodeRows 형식별 응답 → 원래 열 순서의 행 (columns 는 dict 로 이름을 되살린다).
func decodeRows(t *testing.T, obj map[string]any, cols []column) [][]string {
	t.Helper()
	var out [][]string
	switch obj["format"] {
	case "json":
		for _, r := range obj["rows"].([]any) {
			m := r.(map[string]any)
			row := make([]string, len(cols))
			for i, c := range cols {
				row[i], _ = m[c.Key].(string)
			}
			out = append(out, row)
		}
	case "csv":
		recs, err := csv.NewReader(strings.NewReader(obj["csv"].(string))).ReadAll()
		if err != nil {
			t.Fatal(err)
		}
		for _, r := range recs[1:] {
			out = append(out, r)
		}
	case "columns":
		keys := obj["columns"].([]any)
		dict, _ := obj["dict"].(map[string]any)
		for _, r := range obj["rows"].([]any) {
			vals := r.([]any)
			m := map[string]string{}
			for i, k := range keys {
				m[k.(string)] = vals[i].(string)
			}
			for code, d := range dict { // 코드 → 이름 되살리기
				m[nameKey(code)] = d.(map[string]any)[m[code]].(string)
			}
			row := make([]string, len(cols))
			for i, c := range cols {
				row[i] = m[c.Key]
			}
			out = append(out, row)
		}
	}
	return out
}

func TestPagingRoundTripAllFormats(t *testing.T) {
	src := bigTable(9000) // json 으로 약 1.3MB — 상한을 넘는다
	s, _ := testServerWithStore(t, nil, tableTool(src))
	for _, f := range []string{"columns", "csv", "json"} {
		obj, links, isErr := call(t, s, "test_table", `{"format":"`+f+`"}`, "")
		if isErr {
			t.Fatalf("%s: %v", f, obj)
		}
		if obj["format"] != f || obj["truncated"] != true || obj["total_rows"].(float64) != 9000 || obj["result_id"] == nil {
			t.Fatalf("%s 요약 칸: %v", f, obj)
		}
		if len(links) != 1 || links[0].(map[string]any)["type"] != "resource_link" {
			t.Fatalf("%s: resource_link 가 있어야 한다: %v", f, links)
		}
		rows := decodeRows(t, obj, src.Columns)
		if int(obj["returned_rows"].(float64)) != len(rows) {
			t.Fatalf("%s: returned_rows %v ≠ %d", f, obj["returned_rows"], len(rows))
		}
		pages := 1
		for cur, ok := obj["next_cursor"].(string); ok; cur, ok = obj["next_cursor"].(string) {
			// 키 없이도 동작해야 한다 (기본 키도 없는 서버)
			if obj, _, isErr = call(t, s, "kosis_result_page", `{"cursor":"`+cur+`"}`, ""); isErr {
				t.Fatalf("%s result_page: %v", f, obj)
			}
			if obj["format"] != f {
				t.Fatalf("같은 형식이어야 한다: %v", obj["format"])
			}
			rows = append(rows, decodeRows(t, obj, src.Columns)...)
			pages++
		}
		if len(rows) != len(src.Rows) {
			t.Fatalf("%s: 모은 행 %d ≠ %d", f, len(rows), len(src.Rows))
		}
		for i := range rows {
			if strings.Join(rows[i], "|") != strings.Join(src.Rows[i], "|") {
				t.Fatalf("%s: %d번째 행이 다르다\n got %v\nwant %v", f, i, rows[i], src.Rows[i])
			}
		}
		t.Logf("%s: %d쪽", f, pages)
	}
	// columns 는 json 보다 첫 쪽에 더 많은 행
	c, _, _ := call(t, s, "test_table", `{"format":"columns"}`, "")
	j, _, _ := call(t, s, "test_table", `{"format":"json"}`, "")
	if c["returned_rows"].(float64) <= j["returned_rows"].(float64) {
		t.Errorf("columns 가 더 촘촘해야 한다: %v vs %v", c["returned_rows"], j["returned_rows"])
	}
}

func TestSmallResultUnchangedAndSave(t *testing.T) {
	s, _ := testServerWithStore(t, nil, tableTool(bigTable(5)))
	obj, links, _ := call(t, s, "test_table", `{}`, "")
	if obj["format"] != "json" || obj["truncated"] != false || obj["result_id"] != nil || len(links) != 0 || len(obj["rows"].([]any)) != 5 {
		t.Errorf("작은 결과는 바로 json: %v %v", obj, links)
	}
	obj, links, _ = call(t, s, "test_table", `{"save":true}`, "")
	if obj["result_id"] == nil || len(links) != 1 || obj["next_cursor"] != nil || obj["format"] != "columns" {
		t.Errorf("save: %v %v", obj, links)
	}
}

func TestResultExpiredAndClean(t *testing.T) {
	s, st := testServerWithStore(t, nil, tableTool(bigTable(9000)))
	obj, _, _ := call(t, s, "test_table", `{}`, "")
	cur := obj["next_cursor"].(string)
	id := obj["result_id"].(string)
	base := time.Now()
	st.now = func() time.Time { return base.Add(2 * time.Hour) }
	if got, _, isErr := call(t, s, "kosis_result_page", `{"cursor":"`+cur+`"}`, ""); !isErr || !strings.Contains(got["error"].(string), "만료") {
		t.Errorf("만료 오류: %v", got)
	}
	if got, _, isErr := call(t, s, "kosis_result_query", `{"result_id":"`+id+`","latest":true}`, ""); !isErr || !strings.Contains(got["error"].(string), "만료") {
		t.Errorf("만료 오류(query): %v", got)
	}
	if _, err := os.Stat(st.path(id, "csv")); !os.IsNotExist(err) {
		t.Error("만료된 결과 파일은 지워야 한다")
	}

	// TTL 정리 + 용량 정리 (오래된 것부터)
	st.now = time.Now
	var ids []string
	for i := 0; i < 3; i++ {
		m, err := st.save("t", "", bigTable(200))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, m.ID)
		meta := st.path(m.ID, "meta.json") // 만든 시각을 1분씩 앞으로
		b, _ := os.ReadFile(meta)
		var mm resultMeta
		json.Unmarshal(b, &mm)
		mm.Created = time.Now().Add(time.Duration(i-3) * time.Minute).UTC().Format(time.RFC3339)
		b, _ = json.Marshal(mm)
		os.WriteFile(meta, b, 0o600)
	}
	size := func(id string) int64 {
		var n int64
		for _, e := range []string{"jsonl", "csv", "meta.json"} {
			if fi, err := os.Stat(st.path(id, e)); err == nil {
				n += fi.Size()
			}
		}
		return n
	}
	st.maxBytes = size(ids[1]) + size(ids[2]) // 둘만 남을 만큼
	st.clean()
	if _, err := st.meta(ids[0]); err == nil {
		t.Error("용량을 넘으면 가장 오래된 것부터 지운다")
	}
	if _, err := st.meta(ids[2]); err != nil {
		t.Error("가장 새 것은 남는다")
	}
	st.maxBytes = 1 << 30
	st.now = func() time.Time { return time.Now().Add(2 * time.Hour) }
	st.clean()
	if entries, _ := os.ReadDir(st.dir); len(entries) != 0 {
		t.Errorf("TTL 이 지나면 모두 지운다: %d개 남음", len(entries))
	}
}

// 손계산 값과 비교하는 작은 고정 표.
func smallFixed() *table {
	return &table{Columns: []column{{"C1", "지역 코드"}, {"C1_NM", "지역"}, {"ITM_ID", "항목 코드"}, {"PRD_DE", "시점"}, {"DT", "값"}},
		Rows: [][]string{
			{"11", "서울", "T1", "2022", "10"},
			{"11", "서울", "T1", "2023", "20"},
			{"11", "서울", "T1", "2024", "-"},
			{"26", "울산", "T1", "2022", "1,000"},
			{"26", "울산", "T1", "2023", "3"},
			{"26", "울산", "T2", "2022", "5"},
			{"31", "경기", "T1", "2023", "X"},
		}}
}

func rowsOf(t *table) string {
	var parts []string
	for _, r := range t.Rows {
		parts = append(parts, strings.Join(r, ","))
	}
	return strings.Join(parts, ";")
}

func TestTransformsHandComputed(t *testing.T) {
	cases := []struct {
		name string
		tr   transform
		cols string
		want string
		note string
	}{
		{"filter in+range", transform{Filter: map[string]filterCond{"C1": {In: map[string]bool{"11": true, "26": true}}, "PRD_DE": {From: "2023", To: "2024"}}},
			"C1,C1_NM,ITM_ID,PRD_DE,DT", "11,서울,T1,2023,20;11,서울,T1,2024,-;26,울산,T1,2023,3", ""},
		{"latest per series", transform{Latest: true},
			"C1,C1_NM,ITM_ID,PRD_DE,DT", "11,서울,T1,2024,-;26,울산,T1,2023,3;26,울산,T2,2022,5;31,경기,T1,2023,X", ""},
		{"sum by C1", transform{Agg: &aggSpec{GroupBy: []string{"C1"}, Op: "sum", Value: "DT"}},
			"C1,C1_NM,DT_sum", "11,서울,30;26,울산,1008;31,경기,", "건너뛰었습니다"},
		{"avg by ITM_ID", transform{Agg: &aggSpec{GroupBy: []string{"ITM_ID"}, Op: "avg", Value: "DT"}},
			"ITM_ID,DT_avg", "T1,258.25;T2,5", "2개"},
		{"min/max/count", transform{Agg: &aggSpec{GroupBy: []string{"PRD_DE"}, Op: "count", Value: "DT"}},
			"PRD_DE,DT_count", "2022,3;2023,3;2024,1", ""},
		{"max", transform{Agg: &aggSpec{GroupBy: []string{"C1"}, Op: "max", Value: "DT"}},
			"C1,C1_NM,DT_max", "11,서울,20;26,울산,1000;31,경기,", ""},
		{"pivot", transform{Filter: map[string]filterCond{"ITM_ID": {In: map[string]bool{"T1": true}}}, Pivot: &pivotSpec{Rows: []string{"C1"}, Cols: "PRD_DE", Value: "DT"}},
			"C1,C1_NM,2022,2023,2024", "11,서울,10,20,-;26,울산,1,000,3,;31,경기,,X,", ""},
		{"aggregate then order top (동률 없음)", transform{Agg: &aggSpec{GroupBy: []string{"C1"}, Op: "sum", Value: "DT"}, Order: &orderSpec{Desc: true, Top: 1}},
			"C1,C1_NM,DT_sum", "26,울산,1008", ""},
		{"order asc by name column, 숫자 아닌 값은 뒤", transform{Filter: map[string]filterCond{"PRD_DE": {In: map[string]bool{"2023": true}}}, Order: &orderSpec{By: "DT"}},
			"C1,C1_NM,ITM_ID,PRD_DE,DT", "26,울산,T1,2023,3;11,서울,T1,2023,20;31,경기,T1,2023,X", ""},
		{"aggregate then pivot", transform{Agg: &aggSpec{GroupBy: []string{"C1", "PRD_DE"}, Op: "sum", Value: "DT"}, Pivot: &pivotSpec{Rows: []string{"C1"}, Cols: "PRD_DE", Value: "DT_sum"}},
			"C1,C1_NM,2022,2023,2024", "11,서울,10,20,;26,울산,1005,3,;31,경기,,,", ""},
	}
	for _, c := range cases {
		out, notes, err := c.tr.apply(smallFixed())
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := strings.Join(keysOf(out.Columns), ","); got != c.cols {
			t.Errorf("%s 열: %s (기대 %s)", c.name, got, c.cols)
		}
		if got := rowsOf(out); got != c.want {
			t.Errorf("%s 행:\n got %s\nwant %s", c.name, got, c.want)
		}
		if c.note != "" && !strings.Contains(strings.Join(notes, " "), c.note) {
			t.Errorf("%s 안내: %v", c.name, notes)
		}
	}
	if _, _, err := (transform{Filter: map[string]filterCond{"NOPE": {From: "1"}}}).apply(smallFixed()); err == nil || !strings.Contains(err.Error(), "있는 열") {
		t.Errorf("없는 열은 오류: %v", err)
	}
}

func TestResultQueryOnStoredResult(t *testing.T) {
	s, _ := testServerWithStore(t, nil, tableTool(bigTable(9000)))
	obj, _, _ := call(t, s, "test_table", `{}`, "")
	id := obj["result_id"].(string)
	agg, _, isErr := call(t, s, "kosis_result_query", `{"result_id":"`+id+`","aggregate":{"group_by":["ITM_ID"],"op":"count"}}`, "")
	if isErr || agg["total_rows"].(float64) != 3 || agg["source_rows"].(float64) != 9000 {
		t.Fatalf("집계: %v", agg)
	}
	for _, r := range agg["rows"].([]any) {
		if r.(map[string]any)["DT_count"] != "3000" {
			t.Errorf("항목마다 3000행: %v", r)
		}
	}
	if bad, _, isErr := call(t, s, "kosis_result_query", `{"result_id":"`+id+`","aggregate":{"op":"median"}}`, ""); !isErr {
		t.Errorf("모르는 op 는 오류: %v", bad)
	}
}

func TestResourcesReadAndLinkShape(t *testing.T) {
	s, st := testServerWithStore(t, nil, tableTool(bigTable(300)))
	obj, links, _ := call(t, s, "test_table", `{"save":true}`, "")
	id := obj["result_id"].(string)
	l := links[0].(map[string]any)
	m, _ := st.meta(id)
	if l["uri"] != "kosis://results/"+id+".csv" || l["name"] != id+".csv" || l["mimeType"] != "text/csv" || l["size"].(float64) != float64(m.Bytes) || !strings.Contains(l["description"].(string), "300") {
		t.Errorf("resource_link 모양: %v", l)
	}
	if dl := obj["download"].(map[string]any); dl["http_path"] != "/results/"+id+".csv" {
		t.Errorf("download: %v", dl)
	}
	got := runSession(t, s, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"kosis://results/`+id+`.csv"}}`,
		`{"jsonrpc":"2.0","id":2,"method":"resources/read","params":{"uri":"kosis://results/AAAAAAAAAAAAAAAAAAAAAAAA.csv"}}`,
		`{"jsonrpc":"2.0","id":3,"method":"initialize","params":{}}`)
	c := got["1"]["result"].(map[string]any)["contents"].([]any)[0].(map[string]any)
	recs, _ := csv.NewReader(strings.NewReader(strings.TrimPrefix(c["text"].(string), "\xEF\xBB\xBF"))).ReadAll()
	if c["mimeType"] != "text/csv; charset=utf-8" || len(recs) != 301 || recs[0][0] != "C1" {
		t.Errorf("resources/read: %v %d", c["mimeType"], len(recs))
	}
	if got["2"]["error"] == nil {
		t.Error("없는 결과는 오류")
	}
	if caps := got["3"]["result"].(map[string]any)["capabilities"].(map[string]any); caps["resources"] == nil {
		t.Errorf("capabilities.resources: %v", caps)
	}
	old := maxResourceReadBytes
	maxResourceReadBytes = 100
	defer func() { maxResourceReadBytes = old }()
	got = runSession(t, s, `{"jsonrpc":"2.0","id":1,"method":"resources/read","params":{"uri":"kosis://results/`+id+`.csv"}}`)
	if e, _ := got["1"]["error"].(map[string]any); e == nil || !strings.Contains(e["message"].(string), "/results/") {
		t.Errorf("큰 파일은 내려받기 안내 오류: %v", got["1"])
	}
}

func TestHTTPGetResultsToken(t *testing.T) {
	s, _ := testServerWithStore(t, nil, tableTool(bigTable(300)))
	obj, _, _ := call(t, s, "test_table", `{"save":true}`, "")
	id := obj["result_id"].(string)
	c := startHTTP(t, s, httpConfig{Token: "tok-123"})
	get := func(path, auth string) *http.Response {
		req, _ := http.NewRequest("GET", c.url+path, nil)
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		r, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	if r := get("/results/"+id+".csv", ""); r.StatusCode != 401 {
		t.Errorf("토큰 없음 401: %d", r.StatusCode)
	}
	if r := get("/results/"+id+".csv", "Bearer nope"); r.StatusCode != 401 {
		t.Errorf("틀린 토큰 401: %d", r.StatusCode)
	}
	r := get("/results/"+id+".csv", "Bearer tok-123")
	body, _ := io.ReadAll(r.Body)
	if r.StatusCode != 200 || r.Header.Get("Content-Type") != "text/csv; charset=utf-8" || r.Header.Get("Content-Length") != fmt.Sprint(len(body)) ||
		!strings.Contains(r.Header.Get("Content-Disposition"), id+".csv") {
		t.Errorf("GET /results: %d %v", r.StatusCode, r.Header)
	}
	if recs, _ := csv.NewReader(strings.NewReader(strings.TrimPrefix(string(body), "\xEF\xBB\xBF"))).ReadAll(); len(recs) != 301 {
		t.Errorf("CSV 행: %d", len(recs))
	}
	for _, p := range []string{"/results/AAAAAAAAAAAAAAAAAAAAAAAA.csv", "/results/" + id + ".meta.json", "/results/../config.yaml"} {
		if r := get(p, "Bearer tok-123"); r.StatusCode != 404 {
			t.Errorf("%s 는 404: %d", p, r.StatusCode)
		}
	}
	// 토큰 없는 loopback 서버는 그대로
	c2 := startHTTP(t, s, httpConfig{})
	if r, _ := http.Get(c2.url + "/results/" + id + ".csv"); r.StatusCode != 200 {
		t.Errorf("토큰 없는 loopback: %d", r.StatusCode)
	}
}

// 결과 파일(행·CSV·메타)에 요청 키·기본 키가 들어가지 않는다.
func TestResultFilesHaveNoKeys(t *testing.T) {
	const def, req = "DEFAULTSECRET0001", "REQUESTSECRET0002"
	s, st := testServerWithStore(t, []string{def}, &tool{Name: "test_keyed", Description: "test", Params: bigResultParams(),
		run: func(rc *runCtx, in args) (*toolResult, error) {
			return rc.respondTable(bigTable(9000), in.str("format"), in.flag("save"), 0, nil)
		}})
	obj, _, isErr := call(t, s, "test_keyed", `{"save":true}`, `{"kr.pes/keys":["`+req+`"]}`)
	if isErr {
		t.Fatal(obj)
	}
	call(t, s, "test_keyed", `{}`, "")
	entries, _ := os.ReadDir(st.dir)
	if len(entries) < 6 {
		t.Fatalf("결과 파일: %d개", len(entries))
	}
	for _, e := range entries {
		b, _ := os.ReadFile(filepath.Join(st.dir, e.Name()))
		for _, k := range []string{def, req} {
			if strings.Contains(string(b), k) || strings.Contains(e.Name(), k) {
				t.Errorf("%s 에 키 %s", e.Name(), k)
			}
		}
	}
}
