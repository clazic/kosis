package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/clazic/kosis/internal/api"
)

// calcKosis 가짜 KOSIS: 표 둘(DT_T 인구·세대, DT_U 사업체)의 메타와 데이터를 요청 인자대로 돌려준다.
func calcKosis(t *testing.T) string {
	t.Helper()
	names := map[string]string{"00": "전국", "11": "서울특별시", "26": "울산광역시", "31": "경기도"}
	order := []string{"00", "11", "26", "31"}
	vals := map[string]map[string]map[string]string{ // 표·항목 → 코드 → 연도 → 값
		"DT_T/T1": {"00": {"2024": "100", "2025": "110"}, "11": {"2024": "40", "2025": "44"}, "26": {"2024": "10", "2025": "9"}, "31": {"2024": "50", "2025": "57"}},
		"DT_T/T2": {"00": {"2024": "50", "2025": "55"}, "11": {"2024": "20", "2025": "22"}, "26": {"2024": "5", "2025": "5"}, "31": {"2024": "25", "2025": "28"}},
		"DT_U/U1": {"11": {"2025": "4"}, "26": {"2025": "3"}, "31": {"2025": "5"}},
	}
	items := map[string][][2]string{"DT_T": {{"T1", "인구"}, {"T2", "세대"}}, "DT_U": {{"U1", "사업체"}}}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		tbl := q.Get("tblId")
		if strings.Contains(r.URL.Path, "statisticsData.do") { // 메타
			var out []map[string]any
			switch q.Get("type") {
			case "ITM":
				for _, c := range order {
					if tbl == "DT_U" && c == "00" {
						continue
					}
					out = append(out, map[string]any{"OBJ_ID": "A", "OBJ_NM": "시도별", "OBJ_ID_SN": "1", "ITM_ID": c, "ITM_NM": names[c], "UP_ITM_ID": ""})
				}
				for _, it := range items[tbl] {
					out = append(out, map[string]any{"OBJ_ID": "ITEM", "OBJ_NM": "항목", "ITM_ID": it[0], "ITM_NM": it[1], "UNIT_NM": "명"})
				}
			case "PRD":
				out = append(out, map[string]any{"PRD_SE": "년", "STRT_PRD_DE": "2024", "END_PRD_DE": "2025"})
			default:
				w.Write([]byte(`{"err":"30","errMsg":"데이터가 존재하지 않습니다."}`))
				return
			}
			json.NewEncoder(w).Encode(out)
			return
		}
		codes := strings.Split(q.Get("objL1"), "+")
		if q.Get("objL1") == "ALL" {
			codes = order
		}
		its := strings.Split(q.Get("itmId"), "+")
		if q.Get("itmId") == "ALL" {
			its = nil
			for _, it := range items[tbl] {
				its = append(its, it[0])
			}
		}
		years := []string{"2024", "2025"}
		if n, _ := strconv.Atoi(q.Get("newEstPrdCnt")); n > 0 && n < len(years) {
			years = years[len(years)-n:]
		} else if s, e := q.Get("startPrdDe"), q.Get("endPrdDe"); s != "" {
			var ys []string
			for _, y := range years {
				if y >= s && y <= e {
					ys = append(ys, y)
				}
			}
			years = ys
		}
		itemName := map[string]string{}
		for _, it := range items[tbl] {
			itemName[it[0]] = it[1]
		}
		var out []map[string]any
		for _, c := range codes {
			for _, it := range its {
				for _, y := range years {
					v, ok := vals[tbl+"/"+it][c][y]
					if !ok {
						continue
					}
					out = append(out, map[string]any{"ORG_ID": "101", "TBL_ID": tbl, "C1": c, "C1_NM": names[c], "C1_OBJ_NM": "시도별",
						"ITM_ID": it, "ITM_NM": itemName[it], "UNIT_NM": "명", "PRD_SE": "A", "PRD_DE": y, "DT": v})
				}
			}
		}
		if len(out) == 0 {
			w.Write([]byte(`{"err":"30","errMsg":"데이터가 존재하지 않습니다."}`))
			return
		}
		json.NewEncoder(w).Encode(out)
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func calcServer(t *testing.T) *server {
	t.Helper()
	t.Setenv("HOME", t.TempDir()) // 로컬 규칙·스킬 폴더 없음
	up := calcKosis(t)
	s := newServer(nil, "dev", []string{"CALCKEY000000001"}, nil)
	base := s.newClient
	s.newClient = func(keys []string) (*api.Client, error) {
		c, err := base(keys)
		if err == nil {
			c.SetBaseURL(up)
		}
		return c, err
	}
	return s
}

// call 도구 하나를 부르고 결과 JSON(오류면 글)을 돌려준다.
func calcCall(t *testing.T, s *server, tool, argsJSON string) (map[string]any, string, bool) {
	t.Helper()
	got := runSession(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":%s}}`, tool, argsJSON))
	text, isErr := callResult(t, got["1"])
	var obj map[string]any
	if !isErr {
		if err := json.Unmarshal([]byte(text), &obj); err != nil {
			t.Fatalf("%s 결과가 JSON 이 아님: %v\n%s", tool, err, text)
		}
	}
	return obj, text, isErr
}

func calcRows(t *testing.T, obj map[string]any) []map[string]any {
	t.Helper()
	arr, _ := obj["rows"].([]any)
	out := make([]map[string]any, len(arr))
	for i, x := range arr {
		out[i], _ = x.(map[string]any)
	}
	return out
}

func noticesOf(obj map[string]any) string {
	arr, _ := obj["notices"].([]any)
	parts := make([]string, len(arr))
	for i, x := range arr {
		parts[i] = fmt.Sprint(x)
	}
	sort.Strings(parts)
	return strings.Join(parts, "\n")
}

const dtT = `"org_id":"101","tbl_id":"DT_T","period":"Y"`

// 순위: 합계 행을 빼고 정렬·top, RANK 열. 눈으로 정렬하지 않아도 1위 이름이 나온다.
func TestDataSortTopNoTotal(t *testing.T) {
	s := calcServer(t)
	obj, text, isErr := calcCall(t, s, "kosis_data", `{`+dtT+`,"class1":"ALL","item":"T1","latest":1,"sort":"desc","top":2,"no_total":true,"format":"json"}`)
	if isErr {
		t.Fatal(text)
	}
	rows := calcRows(t, obj)
	if len(rows) != 2 || rows[0]["C1_NM"] != "경기도" || rows[0]["RANK"] != "1" || rows[1]["C1_NM"] != "서울특별시" || rows[1]["RANK"] != "2" {
		t.Fatalf("순위: %v", rows)
	}
	if n := noticesOf(obj); !strings.Contains(n, "no_total: 합계 행 1개 뺌") || !strings.Contains(n, "RANK") {
		t.Errorf("안내: %s", n)
	}
	// 합계를 안 빼면 전국이 1위 — no_total 이 실제로 빼는지 대조
	obj, _, _ = calcCall(t, s, "kosis_data", `{`+dtT+`,"class1":"ALL","item":"T1","latest":1,"sort":"desc","top":1,"format":"json"}`)
	if rows := calcRows(t, obj); len(rows) != 1 || rows[0]["C1_NM"] != "전국" {
		t.Errorf("합계 포함 1위: %v", rows)
	}
}

// 증감: 비교 시점 행은 빠지고 DT_CHG·DT_CHG_PCT 가 붙는다. sort 는 증감 기준, sort_by=pct 면 증감률 기준.
func TestDataChange(t *testing.T) {
	s := calcServer(t)
	obj, text, isErr := calcCall(t, s, "kosis_data", `{`+dtT+`,"class1":"11+26+31","item":"T1","start":"2024","end":"2025","change":"yoy","format":"json"}`)
	if isErr {
		t.Fatal(text)
	}
	got := map[string]string{}
	for _, r := range calcRows(t, obj) {
		if r["PRD_DE"] != "2025" {
			t.Errorf("비교 기준 행이 남음: %v", r)
		}
		got[fmt.Sprint(r["C1"])] = fmt.Sprint(r["DT_CHG"], "/", r["DT_CHG_PCT"])
	}
	if got["11"] != "4/10.00" || got["26"] != "-1/-10.00" || got["31"] != "7/14.00" {
		t.Errorf("증감: %v", got)
	}
	// 증감(7) 1위 경기, 증감률(14%) 1위도 경기 — 최저는 울산
	obj, _, _ = calcCall(t, s, "kosis_data", `{`+dtT+`,"class1":"11+26+31","item":"T1","start":"2024","end":"2025","change":"yoy","sort":"asc","sort_by":"pct","top":1,"format":"json"}`)
	if rows := calcRows(t, obj); len(rows) != 1 || rows[0]["C1_NM"] != "울산광역시" || rows[0]["RANK"] != "1" {
		t.Errorf("증감률 최저: %v", rows)
	}
}

// 같은 축 두 코드의 비율.
func TestDataRatio(t *testing.T) {
	s := calcServer(t)
	obj, text, isErr := calcCall(t, s, "kosis_data", `{`+dtT+`,"class1":"11","item":"T1+T2","latest":1,"ratio":"T2/T1","format":"json"}`)
	if isErr {
		t.Fatal(text)
	}
	found := false
	for _, r := range calcRows(t, obj) {
		if r["ITM_ID"] == "T2" {
			found = r["DT_RATIO_PCT"] == "50.00"
		}
	}
	if !found || !strings.Contains(noticesOf(obj), "T2 ÷ T1 × 100") {
		t.Errorf("비율: %v / %s", calcRows(t, obj), noticesOf(obj))
	}
	// 분모 코드를 조회에 안 넣으면 오류
	if _, text, isErr := calcCall(t, s, "kosis_data", `{`+dtT+`,"class1":"11","item":"T2","latest":1,"ratio":"T2/T1"}`); !isErr || !strings.Contains(text, "같은 축에 함께 있지 않습니다") {
		t.Errorf("분모 없음: %v %s", isErr, text)
	}
}

func TestDataDeriveArgErrors(t *testing.T) {
	s := calcServer(t)
	for _, c := range []struct{ args, want string }{
		{`"top":3`, "sort(desc|asc)와 함께"},
		{`"sort":"desc","sort_by":"change"`, "change·pct 는 change"},
		{`"sort":"up"`, "desc, asc 중 하나"},
		{`"ratio":"T1"`, "형식이 맞지 않습니다"},
		{`"change":"mom"`, "yoy, prev 중 하나"},
	} {
		_, text, isErr := calcCall(t, s, "kosis_data", `{`+dtT+`,"class1":"11","item":"T1","latest":1,`+c.args+`}`)
		if !isErr || !strings.Contains(text, c.want) {
			t.Errorf("%s: %v %q", c.args, isErr, text)
		}
	}
}

// 다른 두 표의 비율: 이름으로 짝을 맞추고 scale·sort·top 을 건다.
func TestRatioTool(t *testing.T) {
	s := calcServer(t)
	num := `{"org_id":"101","tbl_id":"DT_T","class1":"11+26+31","item":"T1","period":"Y","latest":1}`
	den := `{"org_id":"101","tbl_id":"DT_U","class1":"11+26+31","item":"U1","period":"Y","latest":1}`
	obj, text, isErr := calcCall(t, s, "kosis_ratio", `{"num":`+num+`,"den":`+den+`,"scale":1,"sort":"desc","top":1}`)
	if isErr {
		t.Fatal(text)
	}
	rows := calcRows(t, obj)
	if len(rows) != 1 || rows[0]["NAME"] != "경기도" || rows[0]["RATIO"] != "11.4000" || rows[0]["RANK"] != "1" || rows[0]["NUM"] != "57" || rows[0]["DEN"] != "5" {
		t.Fatalf("비율 1위: %v", rows)
	}
	// 기본 scale 은 100(%), 정렬 없이 3행
	obj, _, _ = calcCall(t, s, "kosis_ratio", `{"num":`+num+`,"den":`+den+`}`)
	if rows := calcRows(t, obj); len(rows) != 3 || rows[0]["RATIO"] != "1100.0000" {
		t.Errorf("기본 scale: %v", rows)
	}
	// 한쪽에 항목이 둘이면 오류, 조회 인자가 빠지면 오류
	bad := strings.Replace(num, `"item":"T1"`, `"item":"T1+T2"`, 1)
	if _, text, isErr := calcCall(t, s, "kosis_ratio", `{"num":`+bad+`,"den":`+den+`}`); !isErr || !strings.Contains(text, "항목이 2개") {
		t.Errorf("항목 둘: %v %s", isErr, text)
	}
	if _, text, isErr := calcCall(t, s, "kosis_ratio", `{"num":{"org_id":"101"},"den":`+den+`}`); !isErr || !strings.Contains(text, "tbl_id 는 필수") {
		t.Errorf("인자 빠짐: %v %s", isErr, text)
	}
	if _, text, isErr := calcCall(t, s, "kosis_ratio", `{"num":`+num+`,"den":`+den+`,"scale":0}`); !isErr || !strings.Contains(text, "0 이 아닌 수") {
		t.Errorf("scale 0: %v %s", isErr, text)
	}
}

// 여러 조회를 한 번에: 순서대로, 한 조회의 오류가 나머지를 막지 않는다.
func TestBatchTool(t *testing.T) {
	s := calcServer(t)
	obj, text, isErr := calcCall(t, s, "kosis_batch", `{"queries":[`+ // stdio 는 한 줄에 한 요청
		`{`+dtT+`,"class1":"11","item":"T1","latest":1},`+
		`{"org_id":"101","tbl_id":"DT_U","class1":"26","item":"U1","period":"Y","latest":1},`+
		`{`+dtT+`,"class1":"ALL","item":"T1","latest":1,"sort":"desc","top":1,"no_total":true},`+
		`{`+dtT+`,"class1":"11","item":"T1","latest":1,"ratio":"T2/T1"}]}`)
	if isErr {
		t.Fatal(text)
	}
	res, _ := obj["results"].([]any)
	if len(res) != 4 || obj["failed"] != float64(1) {
		t.Fatalf("결과 수·실패 수: %v", obj)
	}
	get := func(i int) map[string]any { m, _ := res[i].(map[string]any); return m }
	rows := func(i int) []map[string]any { return calcRows(t, get(i)) }
	if r := rows(0); len(r) != 1 || r[0]["DT"] != "44" || get(0)["index"] != float64(1) {
		t.Errorf("1번: %v", get(0))
	}
	if r := rows(1); len(r) != 1 || r[0]["DT"] != "3" {
		t.Errorf("2번(다른 표): %v", get(1))
	}
	if r := rows(2); len(r) != 1 || r[0]["C1_NM"] != "경기도" || r[0]["RANK"] != "1" {
		t.Errorf("3번(순위): %v", get(2))
	}
	if e, _ := get(3)["error"].(string); !strings.Contains(e, "같은 축에 함께 있지 않습니다") {
		t.Errorf("4번은 오류여야: %v", get(3))
	}
	for _, c := range []struct{ args, want string }{
		{`{"queries":[]}`, "1~10개"},
		{`{"queries":[{"org_id":"101"}]}`, "1번째: tbl_id 는 필수"},
		{`{"queries":[{` + dtT + `,"class1":"11","item":"T1","latest":1,"bogus":1}]}`, "모르는 인자: bogus"},
	} {
		if _, text, isErr := calcCall(t, s, "kosis_batch", c.args); !isErr || !strings.Contains(text, c.want) {
			t.Errorf("%s: %v %q", c.args, isErr, text)
		}
	}
}
