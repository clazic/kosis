package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/clazic/kosis/internal/relay"
)

const relayTestRealKey = "REALkeyMustNotLeak"

// relayEnv 중계 모드 환경을 맞춘다 (테스트가 끝나면 원래대로).
func relayEnv(t *testing.T, relayURL, keys string) {
	t.Helper()
	t.Setenv("PES_RELAY_URL", relayURL)
	t.Setenv("PES_RELAY_TOKEN", "tok-123")
	t.Setenv("PES_RELAY_KOSIS_KEYS", keys)
	relay.ResetForTesting(true)
	t.Cleanup(func() { relay.ResetForTesting(false) })
}

type seenReq struct {
	path, query, token string
}

// fakeRelay 중계 흉내. handler 가 nil 이면 검색 결과 한 줄을 돌려준다.
func fakeRelay(t *testing.T, handler func(w http.ResponseWriter, r *http.Request, n int)) (*httptest.Server, func() []seenReq) {
	t.Helper()
	var mu sync.Mutex
	var seen []seenReq
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, seenReq{r.URL.EscapedPath(), r.URL.RawQuery, r.Header.Get("X-PES-Relay-Token")})
		n := len(seen)
		mu.Unlock()
		if handler != nil {
			handler(w, r, n)
			return
		}
		w.Write([]byte(`[{"TBL_NM":"인구"}]`))
	}))
	t.Cleanup(srv.Close)
	return srv, func() []seenReq {
		mu.Lock()
		defer mu.Unlock()
		return append([]seenReq(nil), seen...)
	}
}

// 중계 모드: 주소가 {중계}/kosis/https/kosis.kr/… 로 바뀌고, 토큰 머리글이 붙고, 키는 자리표시다.
func TestRelayRewritesRequest(t *testing.T) {
	srv, seen := fakeRelay(t, nil)
	relayEnv(t, srv.URL, "3")

	c, err := NewClient([]string{relayTestRealKey}) // 받은 키는 쓰지 않는다
	if err != nil {
		t.Fatal(err)
	}
	if len(c.apiKeys) != 3 {
		t.Fatalf("키 풀은 자리표시 3개여야 한다: %d", len(c.apiKeys))
	}
	keys := map[string]bool{}
	for i := 0; i < 6; i++ {
		if _, err := c.request("statisticsSearch.do?method=getList", map[string]string{"searchNm": "인구 통계"}, true); err != nil {
			t.Fatal(err)
		}
	}
	reqs := seen()
	if len(reqs) != 6 {
		t.Fatalf("중계 호출 6번을 기대: %d", len(reqs))
	}
	for _, r := range reqs {
		if r.path != "/kosis/https/kosis.kr/openapi/statisticsSearch.do" {
			t.Errorf("경로: %s", r.path)
		}
		if r.token != "tok-123" {
			t.Errorf("토큰 머리글: %q", r.token)
		}
		if strings.Contains(r.query, relayTestRealKey) {
			t.Errorf("실제 키가 나갔다")
		}
		if !strings.Contains(r.query, "method=getList") || !strings.Contains(r.query, "searchNm=") || !strings.Contains(r.query, "format=json") {
			t.Errorf("질의가 그대로여야 한다: %s", r.query)
		}
		for _, kv := range strings.Split(r.query, "&") {
			if strings.HasPrefix(kv, "apiKey=") {
				keys[strings.TrimPrefix(kv, "apiKey=")] = true
			}
		}
	}
	// 돌려 쓰기 그대로: 자리표시 3개가 모두 쓰인다
	for _, want := range []string{"__PES_KEY_0__", "__PES_KEY_1__", "__PES_KEY_2__"} {
		if !keys[want] {
			t.Errorf("자리표시 %s 가 쓰이지 않았다: %v", want, keys)
		}
	}
	if len(keys) != 3 {
		t.Errorf("자리표시가 아닌 키가 있다: %v", keys)
	}
}

// 키 수가 0 이거나 변수가 없으면 규약의 글로 끝나고 중계 · 기관을 부르지 않는다.
func TestRelayNoKeys(t *testing.T) {
	for _, n := range []string{"0", "", "abc", "-1"} {
		srv, seen := fakeRelay(t, nil)
		relayEnv(t, srv.URL, n)
		_, err := NewClient([]string{relayTestRealKey})
		if err == nil || err.Error() != "PES 에 kosis 키가 없어요. PES 「조사 도구 · 그림 키」에서 키를 넣으세요." {
			t.Errorf("n=%q: 오류 글이 다르다: %v", n, err)
		}
		if relay.Failed() == nil {
			t.Errorf("n=%q: 0 이 아닌 코드로 끝나게 표시돼야 한다", n)
		}
		if len(seen()) != 0 {
			t.Errorf("n=%q: 호출이 있으면 안 된다", n)
		}
	}
}

// 중계 오류(X-PES-Relay-Error: 1)는 error 글 그대로 돌려주고, 다른 키로 다시 부르지 않는다.
func TestRelayErrorNoRetry(t *testing.T) {
	for _, status := range []int{401, 403, 409, 429} {
		srv, seen := fakeRelay(t, func(w http.ResponseWriter, r *http.Request, n int) {
			w.Header().Set("X-PES-Relay-Error", "1")
			w.WriteHeader(status)
			w.Write([]byte(`{"error":"PES 중계: 토큰이 맞지 않아요"}`))
		})
		relayEnv(t, srv.URL, "3")
		c, err := NewClient(nil)
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.request("statisticsSearch.do?method=getList", map[string]string{"searchNm": "인구"}, true)
		if err == nil || err.Error() != "PES 중계: 토큰이 맞지 않아요" {
			t.Errorf("%d: error 글 그대로여야 한다: %v", status, err)
		}
		if !relay.IsError(err) || relay.Failed() == nil {
			t.Errorf("%d: 중계 오류로 표시돼야 한다", status)
		}
		// 그 뒤 요청은 중계에 가지 않고 같은 오류
		if _, err2 := c.request("statisticsSearch.do?method=getList", map[string]string{"searchNm": "가구"}, true); err2 == nil || err2.Error() != err.Error() {
			t.Errorf("%d: 두 번째 오류: %v", status, err2)
		}
		if len(seen()) != 1 {
			t.Errorf("%d: 중계 호출은 1번이어야 한다: %d", status, len(seen()))
		}
	}
}

// 기관의 429 는 중계 오류가 아니다 — 지금처럼 다시 해 본다 (기존 동작 유지). 여기서는 한 번 429 뒤 성공.
func TestRelayPassesAgency429(t *testing.T) {
	if testing.Short() {
		t.Skip("백오프 1초")
	}
	srv, seen := fakeRelay(t, func(w http.ResponseWriter, r *http.Request, n int) {
		if n == 1 {
			w.WriteHeader(429)
			return
		}
		w.Write([]byte(`[{"TBL_NM":"인구"}]`))
	})
	relayEnv(t, srv.URL, "1")
	c, _ := NewClient(nil)
	if _, err := c.request("statisticsSearch.do?method=getList", map[string]string{"searchNm": "인구"}, true); err != nil {
		t.Fatal(err)
	}
	if len(seen()) != 2 {
		t.Errorf("429 뒤 한 번 더 불러야 한다: %d", len(seen()))
	}
}

// 주소 넘김(3xx): Location 이 기관 주소(절대 · 상대)면 다시 중계 주소로 바꿔 부른다. 중계 밖으로 나가지 않는다.
func TestRelayRedirectRewrapped(t *testing.T) {
	for _, loc := range []string{"https://www.kosis.kr/moved/a.do?x=1", "/moved/a.do?x=1"} {
		srv, seen := fakeRelay(t, func(w http.ResponseWriter, r *http.Request, n int) {
			if n == 1 {
				w.Header().Set("Location", loc)
				w.WriteHeader(302)
				return
			}
			w.Write([]byte(`[{"TBL_NM":"인구"}]`))
		})
		relayEnv(t, srv.URL, "1")
		c, _ := NewClient(nil)
		if _, err := c.request("statisticsSearch.do?method=getList", map[string]string{"searchNm": "인구"}, true); err != nil {
			t.Fatalf("%s: %v", loc, err)
		}
		reqs := seen()
		if len(reqs) != 2 {
			t.Fatalf("%s: 호출 2번을 기대: %d", loc, len(reqs))
		}
		host := "kosis.kr"
		if strings.HasPrefix(loc, "https://") {
			host = "www.kosis.kr"
		}
		if reqs[1].path != "/kosis/https/"+host+"/moved/a.do" || reqs[1].query != "x=1" || reqs[1].token != "tok-123" {
			t.Errorf("%s: 넘겨받은 주소가 중계 주소로 바뀌지 않았다: %+v", loc, reqs[1])
		}
	}
}

// 중계 모드가 아니면(변수 없음, 또는 CLI 가 아님 — kosis-mcp) 지금과 똑같이 기관을 직접 부른다.
func TestRelayOffUnchanged(t *testing.T) {
	cases := []struct {
		name, url string
		cli       bool
	}{
		{"변수 없음", "", true},
		{"CLI 아님(kosis-mcp)", "http://127.0.0.1:1", false},
	}
	for _, tc := range cases {
		var got seenReq
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			got = seenReq{r.URL.Path, r.URL.RawQuery, r.Header.Get("X-PES-Relay-Token")}
			if _, ok := r.Header["X-Pes-Relay-Token"]; ok {
				t.Errorf("%s: 토큰 머리글이 붙으면 안 된다", tc.name)
			}
			w.Write([]byte(`[{"TBL_NM":"인구"}]`))
		}))
		t.Setenv("PES_RELAY_URL", tc.url)
		t.Setenv("PES_RELAY_TOKEN", "tok-123")
		t.Setenv("PES_RELAY_KOSIS_KEYS", "0")
		relay.ResetForTesting(tc.cli)

		c, err := NewClient([]string{relayTestRealKey})
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if _, ok := c.httpClient.Transport.(*http.Transport); !ok {
			t.Errorf("%s: 전송 계층이 그대로여야 한다: %T", tc.name, c.httpClient.Transport)
		}
		if len(c.apiKeys) != 1 || c.apiKeys[0] != relayTestRealKey {
			t.Errorf("%s: 받은 키를 그대로 써야 한다", tc.name)
		}
		c.baseURL = srv.URL
		if _, err := c.request("statisticsSearch.do?method=getList", map[string]string{"searchNm": "인구"}, true); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got.path != "/statisticsSearch.do" || !strings.Contains(got.query, "apiKey="+relayTestRealKey) {
			t.Errorf("%s: 요청이 바뀌었다: %+v", tc.name, got)
		}
		if relay.Failed() != nil {
			t.Errorf("%s: 중계 오류 표시가 없어야 한다", tc.name)
		}
		srv.Close()
		relay.ResetForTesting(false)
	}
}

// 병렬 분할 조회도 한 길목을 지난다 — 조각마다 자리표시 키로 중계를 부른다.
func TestRelayParallelChunks(t *testing.T) {
	srv, seen := fakeRelay(t, func(w http.ResponseWriter, r *http.Request, n int) {
		w.Write([]byte(`[{"TBL_ID":"T","PRD_DE":"` + r.URL.Query().Get("startPrdDe") + `","DT":"1"}]`))
	})
	relayEnv(t, srv.URL, "2")
	c, _ := NewClient(nil)
	chunks := []PeriodChunk{{Start: "2020", End: "2020"}, {Start: "2021", End: "2021"}, {Start: "2022", End: "2022"}, {Start: "2023", End: "2023"}}
	rows, err := c.dataWithAutoSplitParallel("101", "T", DataOptions{PrdSe: "Y", Item: "ALL", Class1: "ALL"}, chunks, 2, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 4 {
		t.Errorf("행 4개를 기대: %d", len(rows))
	}
	reqs := seen()
	if len(reqs) != 4 {
		t.Fatalf("호출 4번을 기대: %d", len(reqs))
	}
	for _, r := range reqs {
		if !strings.HasPrefix(r.path, "/kosis/https/kosis.kr/openapi/") || !strings.Contains(r.query, "apiKey=__PES_KEY_") || r.token != "tok-123" {
			t.Errorf("조각 요청이 중계 규칙과 다르다: %+v", r)
		}
	}
}
