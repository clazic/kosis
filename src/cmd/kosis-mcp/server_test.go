package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clazic/kosis/internal/api"
)

// ── 스크럽 ────────────────────────────────────────────────────

func TestScrubKeys(t *testing.T) {
	key := "AbCdEf+GhIj/KlMn=="
	other := "ZZZZ1234567890wxyz"
	cases := []struct{ in, want string }{
		{"오류: " + key, "오류: ***Mn=="},
		{"GET https://kosis.kr/openapi/x?apiKey=" + url.QueryEscape(key) + "&format=json", "GET https://kosis.kr/openapi/x?apiKey=***Mn==&format=json"},
		{"path/" + url.PathEscape(key), "path/***Mn=="},
		{"다른 키 apiKey=SOMEUNKNOWNKEY&x=1", "다른 키 apiKey=***&x=1"},
		{other + " 그리고 " + key, "***wxyz 그리고 ***Mn=="},
		{"키 없음", "키 없음"},
	}
	for _, c := range cases {
		if got := scrubKeys(c.in, []string{key, other}); got != c.want {
			t.Errorf("scrubKeys(%q)\n got %q\nwant %q", c.in, got, c.want)
		}
	}
	// 짧은 키는 끝자리도 보이지 않는다
	if got := scrubKeys("k=short1", []string{"short1"}); got != "k=***" {
		t.Errorf("짧은 키: %q", got)
	}
	// 한 키가 다른 키를 품어도 긴 키부터 가린다
	if got := scrubKeys("LONGKEY123456789", []string{"KEY12345", "LONGKEY123456789"}); strings.Contains(got, "KEY12345") {
		t.Errorf("겹친 키: %q", got)
	}
}

// ── 키 선택 ───────────────────────────────────────────────────

func TestSelectKeys(t *testing.T) {
	defaults := []string{"DEFAULTKEY0001"}
	meta := func(s string) map[string]json.RawMessage {
		return map[string]json.RawMessage{metaKeysField: json.RawMessage(s)}
	}

	keys, per, err := selectKeys(meta(`["REQKEY000001","REQKEY000002"]`), defaults)
	if err != nil || !per || strings.Join(keys, ",") != "REQKEY000001,REQKEY000002" {
		t.Fatalf("요청별 키만 써야 한다: %v %v %v", keys, per, err)
	}
	for _, bad := range []string{`[]`, `"REQKEY"`, `[""]`, `["a b"]`, `["k1,k2"]`, `[1]`, `null`} {
		keys, _, err := selectKeys(meta(bad), defaults)
		if err == nil {
			t.Errorf("%s: 오류여야 한다 (받은 키 %v)", bad, keys)
		}
		for _, k := range keys {
			if k == defaults[0] {
				t.Errorf("%s: 기본 키로 넘어갔다", bad)
			}
		}
	}
	if keys, per, err := selectKeys(nil, defaults); err != nil || per || keys[0] != defaults[0] {
		t.Errorf("_meta 가 없으면 기본 키: %v %v %v", keys, per, err)
	}
	if _, _, err := selectKeys(map[string]json.RawMessage{"other": json.RawMessage(`1`)}, nil); err != errNoKey {
		t.Errorf("기본 키도 없으면 errNoKey: %v", err)
	}
}

// ── 입력 검증 ─────────────────────────────────────────────────

func TestValidateArgs(t *testing.T) {
	data, _ := testTool("kosis_data")
	search, _ := testTool("kosis_search")
	ind, _ := testTool("kosis_indicator")

	bad := []struct {
		t    *tool
		args string
	}{
		{data, `{"org_id":"101","tbl_id":"DT_1IN1502","item":"T100"}`},                                // period 빠짐
		{data, `{"org_id":"10 1","tbl_id":"DT_1IN1502","item":"T100","period":"Y"}`},                  // org_id 형식
		{data, `{"org_id":"101","tbl_id":"DT_1IN1502","item":"인구","period":"Y"}`},                     // 한글 코드
		{data, `{"org_id":"101","tbl_id":"DT_1IN1502","item":"T100","period":"Y","latest":0}`},        // 범위
		{data, `{"org_id":"101","tbl_id":"DT_1IN1502","item":"T100","period":"Y","latest":1.5}`},      // 정수 아님
		{data, `{"org_id":"101","tbl_id":"DT_1IN1502","item":"T100","period":"Y","start":"20xx"}`},    // 시점 형식
		{data, `{"org_id":"101","tbl_id":"DT_1IN1502","item":"T100","period":"Y","bogus":"1"}`},       // 모르는 인자
		{data, `{"org_id":"101","tbl_id":"DT_1IN1502","item":"T100","period":"Y","class1":"11; rm"}`}, // 코드 형식
		{search, `{"keyword":"` + strings.Repeat("가", 101) + `"}`},                                    // 길이
		{search, `{"keyword":"인구\n"}`},                                                                // 제어 문자 → 다듬은 뒤엔 통과해야 하므로 아래 good 참고
		{search, `{"keyword":"인구","sort":"BEST"}`},                                                    // enum
		{search, `[1,2]`},
	}
	for i, b := range bad {
		if i == 9 { // TrimSpace 로 끝 개행은 지워진다 — 가운데 제어 문자로 다시
			b.args = `{"keyword":"인\u0001구"}`
		}
		if _, err := validateArgs(b.t, json.RawMessage(b.args)); err == nil {
			t.Errorf("%d %s: 오류여야 한다", i, b.args)
		}
	}

	in, err := validateArgs(search, json.RawMessage(`{"keyword":" 인구 ","sort":"date"}`))
	if err != nil {
		t.Fatal(err)
	}
	if in.str("keyword") != "인구" || in.str("sort") != "DATE" || in.num("limit") != 20 || in.num("page") != 1 {
		t.Errorf("기본값·정규화: %v", in)
	}
	if _, err := validateArgs(data, json.RawMessage(`{"org_id":"101","tbl_id":"DT_1IN1502","class1":"level:1+!03","class2":"ALL","item":"T10,T20","period":"분기","periods":"2020, 2022"}`)); err != nil {
		t.Errorf("정상 인자가 거부됨: %v", err)
	}
	if _, err := validateArgs(ind, json.RawMessage(`{"action":"search","name":"GDP"}`)); err != nil {
		t.Errorf("indicator: %v", err)
	}
}

// ── 결과 자르기 ───────────────────────────────────────────────

func TestFitResultTruncates(t *testing.T) {
	var rows []any
	for i := 0; i < 5000; i++ {
		rows = append(rows, map[string]string{"C1_NM": strings.Repeat("서울특별시", 4), "DT": fmt.Sprint(i)})
	}
	text := fitResult(map[string]any{"org_id": "101"}, "rows", rows, "범위를 좁히세요")
	if len(text) > maxResultBytes {
		t.Fatalf("상한 초과: %d", len(text))
	}
	var got struct {
		Rows      []map[string]string `json:"rows"`
		Truncated string              `json:"truncated"`
	}
	if err := json.Unmarshal([]byte(text), &got); err != nil {
		t.Fatalf("잘린 결과도 JSON 이어야 한다: %v", err)
	}
	if got.Truncated == "" || len(got.Rows) == 0 || len(got.Rows) >= 5000 {
		t.Errorf("잘림 안내·행 수: %q %d", got.Truncated, len(got.Rows))
	}
}

// ── stdio 프로토콜 ────────────────────────────────────────────

// runSession 줄들을 stdin 으로 넣고 stdout 의 응답을 id 로 모은다.
func runSession(t *testing.T, s *server, lines ...string) map[string]map[string]any {
	t.Helper()
	var out bytes.Buffer
	s.out = &out
	if err := s.serve(strings.NewReader(strings.Join(lines, "\n") + "\n")); err != nil {
		t.Fatal(err)
	}
	got := map[string]map[string]any{}
	sc := bufio.NewScanner(&out)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("stdout 에 JSON-RPC 가 아닌 줄: %q", sc.Text())
		}
		got[fmt.Sprint(m["id"])] = m
	}
	return got
}

func TestInitializeAndToolsList(t *testing.T) {
	s := newServer(nil, "v9.9.9", nil, nil) // 키 없이
	got := runSession(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":3,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":4,"method":"prompts/list"}`,
	)
	if len(got) != 4 {
		t.Fatalf("응답 4개(알림 제외)를 기대: %d", len(got))
	}
	init := got["1"]["result"].(map[string]any)
	if init["protocolVersion"] != "2025-06-18" || init["serverInfo"].(map[string]any)["version"] != "v9.9.9" {
		t.Errorf("initialize: %v", init)
	}
	var names []string
	for _, tl := range got["2"]["result"].(map[string]any)["tools"].([]any) {
		m := tl.(map[string]any)
		names = append(names, m["name"].(string))
		schema := m["inputSchema"].(map[string]any)
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Errorf("%s 스키마: %v", m["name"], schema)
		}
		if d, _ := m["description"].(string); d == "" {
			t.Errorf("%s 설명 없음", m["name"])
		}
	}
	if strings.Join(names, ",") != "kosis_search,kosis_meta,kosis_data,kosis_result_page,kosis_result_query,kosis_batch,kosis_ratio,kosis_rules,kosis_list,kosis_explain,kosis_indicator" {
		t.Errorf("도구 목록: %v", names)
	}
	if got["4"]["error"] == nil {
		t.Errorf("모르는 메서드는 오류: %v", got["4"])
	}
}

func TestOldProtocolVersionEchoed(t *testing.T) {
	got := runSession(t, newServer(nil, "dev", nil, nil),
		`{"jsonrpc":"2.0","id":"a","method":"initialize","params":{"protocolVersion":"2024-11-05"}}`)
	if v := got["a"]["result"].(map[string]any)["protocolVersion"]; v != "2024-11-05" {
		t.Errorf("옛 버전은 그대로: %v", v)
	}
}

func callResult(t *testing.T, m map[string]any) (string, bool) {
	t.Helper()
	res, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("result 없음: %v", m)
	}
	content := res["content"].([]any)[0].(map[string]any)
	return content["text"].(string), res["isError"] == true
}

func TestNoKeyIsError(t *testing.T) {
	got := runSession(t, newServer(nil, "dev", nil, nil),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"인구"}}}`)
	text, isErr := callResult(t, got["1"])
	if !isErr || !strings.Contains(text, "API 키가 없어요") {
		t.Errorf("키 없음: %v %q", isErr, text)
	}
}

// kosis_rules 는 API 키도 스킬 폴더도 없이 바이너리에 내장된 규칙으로 답한다.
func TestRulesToolWithoutKeysOrSkill(t *testing.T) {
	t.Setenv("HOME", t.TempDir()) // 스킬 폴더·로컬 규칙 없음
	t.Setenv("KOSIS_RULES_FILE", "")
	got := runSession(t, newServer(nil, "dev", nil, nil),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"kosis_rules","arguments":{"id":"116/DT_MLTM_2086"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kosis_rules","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kosis_rules","arguments":{"query":"전기차","kind":"gap"}}}`)
	text, isErr := callResult(t, got["1"])
	if isErr || !strings.Contains(text, "미분양주택현황") || !strings.Contains(text, "연간만 있다") {
		t.Errorf("표 규칙: %v %q", isErr, text)
	}
	if text, isErr := callResult(t, got["2"]); !isErr || !strings.Contains(text, "하나는 주세요") {
		t.Errorf("인자 없이: %v %q", isErr, text)
	}
	if text, isErr := callResult(t, got["3"]); isErr || !strings.Contains(text, "ev-registrations-by-region") {
		t.Errorf("gap 찾기: %v %q", isErr, text)
	}
}

// 요청별 키가 있으면 그 키로만 클라이언트를 만들고(기본 키 안 씀), 결과의 키 원문은 가린다.
func TestRequestKeysOnlyAndScrubbed(t *testing.T) {
	const def = "DEFAULTKEY0000def1"
	const req = "REQUESTKEY+000/req2="
	s := newServer(nil, "dev", []string{def}, nil)
	var mu sync.Mutex
	var used [][]string
	s.newClient = func(keys []string) (*api.Client, error) {
		mu.Lock()
		used = append(used, append([]string(nil), keys...))
		mu.Unlock()
		// 상위 오류에 키가 든 URL 이 섞여 나오는 경우
		return nil, fmt.Errorf("GET https://kosis.kr/openapi/x?apiKey=%s 실패 (%s, %s)", url.QueryEscape(keys[0]), keys[0], def)
	}
	got := runSession(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"인구"},"_meta":{"kr.pes/keys":["`+req+`"]}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"인구"},"_meta":{"kr.pes/keys":[]}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"인구"}}}`,
	)
	if len(used) != 2 {
		t.Fatalf("클라이언트는 id 1·3 에서만 만들어야 한다 (빈 요청별 키는 기본 키로 넘어가지 않음): %v", used)
	}
	var sawReq, sawDef bool
	for _, u := range used {
		switch strings.Join(u, ",") {
		case req:
			sawReq = true
		case def:
			sawDef = true
		default:
			t.Errorf("키가 섞였다: %v", u)
		}
	}
	if !sawReq || !sawDef {
		t.Errorf("요청별 키(id 1)·기본 키(id 3): %v", used)
	}
	for _, id := range []string{"1", "2", "3"} {
		text, isErr := callResult(t, got[id])
		if !isErr {
			t.Errorf("id %s: 오류여야 한다", id)
		}
		for _, k := range []string{req, url.QueryEscape(req), def} {
			if strings.Contains(text, k) {
				t.Errorf("id %s: 결과에 키 원문 %q: %s", id, k, text)
			}
		}
	}
	if text, _ := callResult(t, got["1"]); !strings.Contains(text, "***eq2=") || !strings.Contains(text, "***def1") {
		t.Errorf("가린 키는 ***끝4자리: %s", text)
	}
}

// tools/call 은 따로 처리된다 — 느린 요청이 뒤 요청을 막지 않는다.
func TestToolsCallConcurrent(t *testing.T) {
	s := newServer(nil, "dev", nil, nil)
	fastDone := make(chan struct{})
	var slowSawFast bool
	s.newClient = func(keys []string) (*api.Client, error) {
		if keys[0] == "SLOWKEY000000" {
			select {
			case <-fastDone:
				slowSawFast = true
			case <-time.After(5 * time.Second):
			}
			return nil, fmt.Errorf("slow")
		}
		close(fastDone)
		return nil, fmt.Errorf("fast")
	}
	runSession(t, s,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"a"},"_meta":{"kr.pes/keys":["SLOWKEY000000"]}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"b"},"_meta":{"kr.pes/keys":["FASTKEY000000"]}}}`,
	)
	if !slowSawFast {
		t.Error("두 번째 요청이 첫 요청이 끝나기를 기다렸다")
	}
}

func TestInvalidArgsIsError(t *testing.T) {
	got := runSession(t, newServer(nil, "dev", []string{"DEFAULTKEY0000"}, nil),
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"kosis_data","arguments":{"org_id":"101","tbl_id":"DT_1IN1502","item":"인구","period":"Y"}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kosis_nope","arguments":{}}}`,
		`not json`,
	)
	if text, isErr := callResult(t, got["1"]); !isErr || !strings.Contains(text, "item") {
		t.Errorf("잘못된 인자: %v %q", isErr, text)
	}
	if got["2"]["error"] == nil {
		t.Errorf("모르는 도구는 프로토콜 오류: %v", got["2"])
	}
	if got["<nil>"]["error"] == nil {
		t.Errorf("파싱 오류: %v", got)
	}
}

func testTool(name string) (*tool, bool) {
	return newServer(nil, "dev", nil, nil).toolByName(name)
}
