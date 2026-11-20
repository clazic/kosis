package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/cache"
)

// 키별 사용량 테스트의 키 (가짜 상위 서버가 키에 따라 다르게 답한다).
const (
	usageKeyOK  = "USAGEKEYOK000001"
	usageKeyLim = "USAGEKEYLIM00002" // 처음 한 번 429, 다음부터 200
	usageKeyErr = "USAGEKEYERR00003" // KOSIS 오류 본문
	usageKeyDef = "USAGEKEYDEF00004" // 기본 키
)

// fakeKosis 가짜 KOSIS: apiKey 에 따라 200 · 429 뒤 200 · 오류 본문. hits 는 받은 요청 수.
func fakeKosis(t *testing.T) (url string, hits func() int) {
	t.Helper()
	var mu sync.Mutex
	n, limited := 0, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n++
		first429 := r.URL.Query().Get("apiKey") == usageKeyLim && !limited
		if first429 {
			limited = true
		}
		mu.Unlock()
		switch {
		case first429:
			w.WriteHeader(http.StatusTooManyRequests)
		case r.URL.Query().Get("apiKey") == usageKeyErr:
			w.Write([]byte(`{"err":"20","errMsg":"필수요청변수값이 누락되었습니다."}`))
		default:
			w.Write([]byte(`[{"ORG_ID":"101","ORG_NM":"통계청","TBL_ID":"DT_1B040A3","TBL_NM":"주민등록인구"}]`))
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL, func() int { mu.Lock(); defer mu.Unlock(); return n }
}

// usageServer 가짜 상위 서버에 붙은 서버. search3 는 검색을 세 번(라운드로빈으로 키 셋에 한 번씩), search3_fail 은 그 뒤 오류.
func usageServer(t *testing.T, defaults []string, fc *cache.Cache) (*server, func() int) {
	t.Helper()
	up, hits := fakeKosis(t)
	s := newServer(nil, "dev", defaults, fc)
	base := s.newClient
	s.newClient = func(keys []string) (*api.Client, error) {
		c, err := base(keys)
		if err == nil {
			c.SetBaseURL(up)
		}
		return c, err
	}
	search3 := func(rc *runCtx) {
		for _, kw := range []string{"가", "나", "다"} {
			_, _ = rc.client.Search(kw, api.SearchOptions{})
		}
	}
	s.tools = append(append([]*tool{}, tools...),
		&tool{Name: "test_search3", Description: "test", run: func(rc *runCtx, in args) (*toolResult, error) {
			search3(rc)
			return textResult(`{"ok":true}`), nil
		}},
		&tool{Name: "test_search3_fail", Description: "test", run: func(rc *runCtx, in args) (*toolResult, error) {
			search3(rc)
			return nil, errors.New("일부러 낸 오류")
		}})
	return s, hits
}

// stdioRaw 한 줄을 넣고 stdout 원문 한 줄과 그 result.
func stdioRaw(t *testing.T, s *server, line string) (string, map[string]any) {
	t.Helper()
	var out bytes.Buffer
	s.out = &out
	if err := s.serve(strings.NewReader(line + "\n")); err != nil {
		t.Fatal(err)
	}
	raw := strings.TrimSpace(out.String())
	var m map[string]any
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		t.Fatalf("응답이 JSON 한 줄이 아님: %v (%q)", err, raw)
	}
	res, ok := m["result"].(map[string]any)
	if !ok {
		t.Fatalf("result 없음: %s", raw)
	}
	return raw, res
}

// usageOf result._meta["kr.pes/key-usage"] 를 다시 JSON 으로 (없으면 "").
func usageOf(t *testing.T, res map[string]any) string {
	t.Helper()
	meta, ok := res["_meta"].(map[string]any)
	if !ok {
		return ""
	}
	u, ok := meta[metaKeyUsageField]
	if !ok {
		return ""
	}
	var list []keyUsageEntry
	b, _ := json.Marshal(u)
	if err := json.Unmarshal(b, &list); err != nil {
		t.Fatalf("키별 사용량 형식: %v (%s)", err, b)
	}
	b, _ = json.Marshal(list)
	return string(b)
}

func noKeyText(t *testing.T, raw string) {
	t.Helper()
	for _, k := range []string{usageKeyOK, usageKeyLim, usageKeyErr, usageKeyDef} {
		for _, part := range []string{k, k[len(k)-4:], k[:8]} {
			if strings.Contains(raw, part) {
				t.Errorf("응답에 키 글자 %q 가 있다: %s", part, raw)
			}
		}
	}
}

func usageCall(name, meta string) string {
	if meta == "" {
		meta = "{}"
	}
	return fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q,"arguments":{},"_meta":%s}}`, name, meta)
}

// 요청별 키: 받은 목록의 자리(i, 겹친 키는 처음 자리)로 calls · errors · limited 를 센다. 성공 · 오류 결과 모두.
func TestKeyUsageStdio(t *testing.T) {
	// 받은 목록 [OK, OK(겹침), LIM, ERR] → 쓰는 키 셋의 자리는 0 · 2 · 3
	meta := fmt.Sprintf(`{"kr.pes/keys":[%q,%q,%q,%q]}`, usageKeyOK, usageKeyOK, usageKeyLim, usageKeyErr)
	const want = `[{"i":0,"calls":1,"errors":0,"limited":0},{"i":2,"calls":2,"errors":1,"limited":1},{"i":3,"calls":1,"errors":1,"limited":0}]`
	for _, name := range []string{"test_search3", "test_search3_fail"} {
		s, hits := usageServer(t, []string{usageKeyDef}, nil)
		raw, res := stdioRaw(t, s, usageCall(name, meta))
		if isErr := res["isError"] == true; isErr != (name == "test_search3_fail") {
			t.Errorf("%s: isError=%v", name, isErr)
		}
		if got := usageOf(t, res); got != want {
			t.Errorf("%s: 키별 사용량\n got %s\nwant %s", name, got, want)
		}
		if hits() != 4 {
			t.Errorf("%s: 상위 요청 4번(429 재시도 포함)이어야 한다: %d", name, hits())
		}
		if _, ok := res["content"].([]any); !ok {
			t.Errorf("%s: content 는 그대로 있어야 한다: %s", name, raw)
		}
		noKeyText(t, raw)
	}
}

// 기본 키로 한 호출에는 _meta 를 붙이지 않는다.
func TestKeyUsageDefaultKeysOmitted(t *testing.T) {
	s, hits := usageServer(t, []string{usageKeyDef}, nil)
	raw, res := stdioRaw(t, s, usageCall("test_search3", ""))
	if _, ok := res["_meta"]; ok || strings.Contains(raw, metaKeyUsageField) {
		t.Errorf("기본 키에는 키별 사용량이 없어야 한다: %s", raw)
	}
	if hits() != 3 {
		t.Errorf("상위 요청 3번: %d", hits())
	}
	noKeyText(t, raw)
}

// 쓰지 않은 키는 빼고, 상위 요청이 없으면(캐시 · 키가 필요 없는 도구) 항목 자체를 뺀다.
func TestKeyUsageUnusedAndCacheHit(t *testing.T) {
	fc, err := cache.New(t.TempDir(), 1)
	if err != nil {
		t.Fatal(err)
	}
	s, hits := usageServer(t, nil, fc)
	meta := fmt.Sprintf(`{"kr.pes/keys":[%q,%q]}`, usageKeyOK, "USAGEKEYOK000009")
	line := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"인구"},"_meta":%s}}`, meta)

	_, res := stdioRaw(t, s, line)
	if res["isError"] == true {
		t.Fatalf("검색 실패: %v", res)
	}
	var list []keyUsageEntry
	if err := json.Unmarshal([]byte(usageOf(t, res)), &list); err != nil || len(list) != 1 {
		t.Fatalf("한 번 불렀으니 쓴 키 하나만: %v %v", list, err)
	}
	if e := list[0]; (e.I != 0 && e.I != 1) || e.Calls != 1 || e.Errors != 0 || e.Limited != 0 {
		t.Errorf("항목: %+v", e)
	}

	raw, res := stdioRaw(t, s, line) // 같은 검색 — 캐시에서 답한다
	if res["isError"] == true || hits() != 1 {
		t.Fatalf("두 번째는 캐시여야 한다: hits=%d %s", hits(), raw)
	}
	if _, ok := res["_meta"]; ok {
		t.Errorf("상위 요청이 없으면 키별 사용량도 없다: %s", raw)
	}

	// 저장된 결과만 읽는 도구(키를 쓰지 않음)도 붙이지 않는다
	raw, res = stdioRaw(t, s, fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"kosis_result_page","arguments":{"result_id":"r_0000000000000000000000","page":1},"_meta":%s}}`, meta))
	if _, ok := res["_meta"]; ok {
		t.Errorf("상위 요청이 없는 도구: %s", raw)
	}
}

// 이미 있는 _meta 항목은 지우지 않고 합친다.
func TestWithKeyUsageMerges(t *testing.T) {
	res := withKeyUsage(map[string]any{"_meta": map[string]any{"other": 1}}, []keyUsageEntry{{I: 2, Calls: 1}})
	b, _ := json.Marshal(res)
	if string(b) != `{"_meta":{"kr.pes/key-usage":[{"i":2,"calls":1,"errors":0,"limited":0}],"other":1}}` {
		t.Errorf("합치기: %s", b)
	}
	if res := withKeyUsage(map[string]any{"isError": false}, nil); res["_meta"] != nil {
		t.Errorf("빈 사용량이면 _meta 를 만들지 않는다: %v", res)
	}
}

// HTTP: X-Api-Keys 머리글 키(JSON 응답)와 SSE 응답 이벤트에도 붙는다.
func TestKeyUsageHTTP(t *testing.T) {
	const want = `[{"i":0,"calls":1,"errors":0,"limited":0},{"i":1,"calls":2,"errors":1,"limited":1},{"i":2,"calls":1,"errors":1,"limited":0}]`
	s, _ := usageServer(t, []string{usageKeyDef}, nil)
	c := startHTTP(t, s, httpConfig{})
	c.initialize()

	// 머리글 (빈 칸은 자리로 세지 않는다)
	resp := c.do("POST", usageCall("test_search3", ""), "X-Api-Keys", usageKeyOK+", ,"+usageKeyLim+","+usageKeyErr)
	var buf bytes.Buffer
	buf.ReadFrom(resp.Body)
	resp.Body.Close()
	var m map[string]any
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatal(err)
	}
	if got := usageOf(t, m["result"].(map[string]any)); got != want {
		t.Errorf("머리글 키 사용량\n got %s\nwant %s", got, want)
	}
	noKeyText(t, buf.String())

	// 기본 키 — 붙지 않는다
	def := readJSON(t, c.do("POST", usageCall("test_search3", "")))
	if _, ok := def["result"].(map[string]any)["_meta"]; ok {
		t.Errorf("기본 키: %v", def)
	}

	// SSE (progressToken) — 마지막 응답 이벤트의 result 에
	s2, _ := usageServer(t, nil, nil)
	c2 := startHTTP(t, s2, httpConfig{})
	c2.initialize()
	meta := fmt.Sprintf(`{"progressToken":"p1","kr.pes/keys":[%q,%q,%q]}`, usageKeyOK, usageKeyLim, usageKeyErr)
	sse := c2.do("POST", usageCall("test_search3", meta))
	if ct := sse.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("SSE 여야 한다: %s", ct)
	}
	evs := sseEvents(t, sse.Body)
	last := evs[len(evs)-1]
	res, ok := last["result"].(map[string]any)
	if !ok {
		t.Fatalf("마지막 이벤트는 응답: %v", last)
	}
	if got := usageOf(t, res); got != want {
		t.Errorf("SSE 키 사용량\n got %s\nwant %s", got, want)
	}
	for _, e := range evs[:len(evs)-1] {
		if b, _ := json.Marshal(e); strings.Contains(string(b), metaKeyUsageField) {
			t.Errorf("진행 알림에는 붙지 않는다: %s", b)
		}
	}
}
