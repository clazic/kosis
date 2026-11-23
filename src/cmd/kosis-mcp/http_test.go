package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/clazic/kosis/internal/api"
)

const initBody = `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"t","version":"0"}}}`

type httpClient struct {
	t       *testing.T
	url     string
	session string
	header  map[string]string
}

func (c *httpClient) do(method, body string, hdr ...string) *http.Response {
	c.t.Helper()
	req, _ := http.NewRequest(method, c.url+"/mcp", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	if c.session != "" {
		req.Header.Set(sessionHeader, c.session)
	}
	for k, v := range c.header {
		req.Header.Set(k, v)
	}
	for i := 0; i+1 < len(hdr); i += 2 {
		if hdr[i+1] == "" {
			req.Header.Del(hdr[i])
		} else {
			req.Header.Set(hdr[i], hdr[i+1])
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp
}

func readJSON(t *testing.T, resp *http.Response) map[string]any {
	t.Helper()
	defer resp.Body.Close()
	var m map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatalf("JSON 응답이 아님 (%d): %v", resp.StatusCode, err)
	}
	return m
}

func startHTTP(t *testing.T, s *server, cfg httpConfig) *httpClient {
	srv := httptest.NewServer(newHTTPServer(s, cfg))
	t.Cleanup(srv.Close)
	return &httpClient{t: t, url: srv.URL}
}

func (c *httpClient) initialize() {
	c.t.Helper()
	resp := c.do("POST", initBody)
	if resp.StatusCode != 200 {
		c.t.Fatalf("initialize: %d", resp.StatusCode)
	}
	c.session = resp.Header.Get(sessionHeader)
	if len(c.session) < 32 {
		c.t.Fatalf("세션 머리글이 128bit 이상이어야 한다: %q", c.session)
	}
	readJSON(c.t, resp)
}

func TestHTTPSessionFlow(t *testing.T) {
	c := startHTTP(t, newServer(nil, "dev", nil, nil), httpConfig{})
	c.initialize()

	// 세션 없는 요청 400, 모르는 세션 404
	if r := c.do("POST", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, sessionHeader, ""); r.StatusCode != 400 {
		t.Errorf("세션 없음: %d", r.StatusCode)
	}
	if r := c.do("POST", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, sessionHeader, "nope"); r.StatusCode != 404 {
		t.Errorf("모르는 세션: %d", r.StatusCode)
	}
	if r := c.do("POST", `{"jsonrpc":"2.0","method":"notifications/initialized"}`); r.StatusCode != 202 {
		t.Errorf("알림만 온 POST 는 202: %d", r.StatusCode)
	}
	list := readJSON(t, c.do("POST", `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`, "MCP-Protocol-Version", "2025-06-18"))
	if n := len(list["result"].(map[string]any)["tools"].([]any)); n != 11 {
		t.Errorf("도구 11개: %d", n)
	}
	call := readJSON(t, c.do("POST", `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"인구"}}}`))
	if text, isErr := callResult(t, call); !isErr || !strings.Contains(text, "API 키가 없어요") {
		t.Errorf("키 없음 오류: %v %q", isErr, text)
	}
	if r := c.do("POST", `{"jsonrpc":"2.0","id":4,"method":"ping"}`, "MCP-Protocol-Version", "1999-01-01"); r.StatusCode != 400 {
		t.Errorf("모르는 프로토콜 버전 400: %d", r.StatusCode)
	}
	var big []string
	for i := 0; i < 11; i++ {
		big = append(big, fmt.Sprintf(`{"jsonrpc":"2.0","id":%d,"method":"ping"}`, i+10))
	}
	if r := c.do("POST", "["+strings.Join(big, ",")+"]"); r.StatusCode != 400 {
		t.Errorf("배치 11개는 400: %d", r.StatusCode)
	}
	if r := c.do("POST", strings.Repeat(" ", maxBodyBytes+10)+"{}"); r.StatusCode != 413 {
		t.Errorf("1MB 넘는 본문 413: %d", r.StatusCode)
	}
	batch := c.do("POST", `[{"jsonrpc":"2.0","id":5,"method":"ping"},{"jsonrpc":"2.0","id":6,"method":"ping"}]`)
	var arr []map[string]any
	json.NewDecoder(batch.Body).Decode(&arr)
	if len(arr) != 2 {
		t.Errorf("배치 응답 2개: %v", arr)
	}
	if r := c.do("DELETE", ""); r.StatusCode != 200 {
		t.Errorf("DELETE: %d", r.StatusCode)
	}
	if r := c.do("POST", `{"jsonrpc":"2.0","id":7,"method":"ping"}`); r.StatusCode != 404 {
		t.Errorf("끝난 세션 404: %d", r.StatusCode)
	}
}

func TestHTTPToken(t *testing.T) {
	c := startHTTP(t, newServer(nil, "dev", nil, nil), httpConfig{Token: "s3cret-token"})
	for _, auth := range []string{"", "Bearer wrong", "s3cret-token", "Basic s3cret-token"} {
		r := c.do("POST", initBody, "Authorization", auth)
		if r.StatusCode != 401 {
			t.Errorf("Authorization %q: %d", auth, r.StatusCode)
		}
	}
	if r := c.do("POST", initBody, "Authorization", "Bearer s3cret-token"); r.StatusCode != 200 {
		t.Errorf("맞는 토큰: %d", r.StatusCode)
	}
}

func TestCheckListen(t *testing.T) {
	for _, a := range []string{"127.0.0.1:8080", "[::1]:9", "localhost:1"} {
		if err := checkListen(a, ""); err != nil {
			t.Errorf("%s: %v", a, err)
		}
	}
	for _, a := range []string{"0.0.0.0:8080", ":8080", "192.168.0.10:8080"} {
		if err := checkListen(a, ""); err == nil {
			t.Errorf("%s 는 토큰 없이 시작을 거부해야 한다", a)
		}
		if err := checkListen(a, "tok"); err != nil {
			t.Errorf("%s + 토큰: %v", a, err)
		}
	}
	h := newHTTPServer(newServer(nil, "dev", nil, nil), httpConfig{})
	if err := h.listenAndServe("0.0.0.0:0"); err == nil || !strings.Contains(err.Error(), "KOSIS_MCP_TOKEN") {
		t.Errorf("비 loopback + 토큰 없음은 시작 거부: %v", err)
	}
}

func TestHTTPOrigin(t *testing.T) {
	c := startHTTP(t, newServer(nil, "dev", nil, nil), httpConfig{AllowedOrigins: []string{"https://app.example.org"}})
	cases := map[string]int{
		"http://evil.example":       403,
		"null":                      403,
		"http://localhost.evil.com": 403,
		"http://localhost:3000":     200,
		"http://127.0.0.1:5173":     200,
		"https://app.example.org":   200,
	}
	for o, want := range cases {
		if r := c.do("POST", initBody, "Origin", o); r.StatusCode != want {
			t.Errorf("Origin %s: %d (기대 %d)", o, r.StatusCode, want)
		}
	}
}

// X-Api-Keys 머리글이 기본 키보다 우선하고, _meta 가 있으면 _meta. 응답에서 키는 지워진다.
func TestHTTPAPIKeysHeader(t *testing.T) {
	const def, hdr1, hdr2, meta = "DEFAULTKEY000001", "HEADERKEY0000001", "HEADERKEY0000002", "METAKEY000000001"
	s := newServer(nil, "dev", []string{def}, nil)
	var mu sync.Mutex
	var used []string
	s.newClient = func(keys []string) (*api.Client, error) {
		mu.Lock()
		used = append(used, strings.Join(keys, ","))
		mu.Unlock()
		return nil, fmt.Errorf("boom apiKey=%s", keys[0])
	}
	c := startHTTP(t, s, httpConfig{})
	c.initialize()
	call := `{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"인구"}}}`
	res := readJSON(t, c.do("POST", call, "X-Api-Keys", hdr1+", "+hdr2))
	text, isErr := callResult(t, res)
	if !isErr || strings.Contains(text, hdr1) || !strings.Contains(text, "***0001") {
		t.Errorf("오류·스크럽: %v %q", isErr, text)
	}
	callMeta := `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"인구"},"_meta":{"kr.pes/keys":["` + meta + `"]}}}`
	readJSON(t, c.do("POST", callMeta, "X-Api-Keys", hdr1))
	readJSON(t, c.do("POST", call))
	bad := readJSON(t, c.do("POST", call, "X-Api-Keys", " , "))
	if _, isErr := callResult(t, bad); !isErr {
		t.Error("빈 X-Api-Keys 는 오류 (기본 키로 넘어가지 않음)")
	}
	want := []string{hdr1 + "," + hdr2, meta, def}
	if strings.Join(used, "|") != strings.Join(want, "|") {
		t.Errorf("쓴 키:\n got %v\nwant %v", used, want)
	}
}

// sseEvents SSE 본문의 data 를 차례로.
func sseEvents(t *testing.T, r io.Reader) []map[string]any {
	t.Helper()
	var out []map[string]any
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 8<<20)
	for sc.Scan() {
		if d, ok := strings.CutPrefix(sc.Text(), "data: "); ok {
			var m map[string]any
			if err := json.Unmarshal([]byte(d), &m); err != nil {
				t.Fatalf("SSE data 가 JSON 이 아님: %q", d)
			}
			out = append(out, m)
		}
	}
	return out
}

// 진행 조각을 내는 가짜 도구 (네트워크 없이).
func chunkTool() *tool {
	return &tool{Name: "test_chunks", Description: "test", run: func(rc *runCtx, in args) (*toolResult, error) {
		time.Sleep(progressInterval + 50*time.Millisecond)
		rc.prog.step(1, 3, "조각")
		rc.prog.step(2, 3, "조각") // 1초 안 — 건너뜀
		return textResult(`{"ok":true}`), nil
	}}
}

func TestHTTPSSEProgressBeforeResult(t *testing.T) {
	s := newServer(nil, "dev", []string{"DEFAULTKEY000001"}, nil)
	s.tools = append(append([]*tool{}, tools...), chunkTool())
	c := startHTTP(t, s, httpConfig{})
	c.initialize()
	resp := c.do("POST", `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"test_chunks","arguments":{},"_meta":{"progressToken":"p1"}}}`)
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/event-stream") {
		t.Fatalf("SSE 여야 한다: %s", ct)
	}
	evs := sseEvents(t, resp.Body)
	if len(evs) < 3 {
		t.Fatalf("진행 알림 + 응답: %v", evs)
	}
	last := evs[len(evs)-1]
	if fmt.Sprint(last["id"]) != "9" || last["result"] == nil {
		t.Errorf("마지막 이벤트는 응답: %v", last)
	}
	var values []float64
	for _, e := range evs[:len(evs)-1] {
		if e["method"] != "notifications/progress" {
			t.Fatalf("응답 전 이벤트는 진행 알림: %v", e)
		}
		p := e["params"].(map[string]any)
		if p["progressToken"] != "p1" {
			t.Errorf("progressToken: %v", p)
		}
		values = append(values, p["progress"].(float64))
	}
	for i := 1; i < len(values); i++ {
		if values[i] <= values[i-1] {
			t.Errorf("progress 는 늘기만: %v", values)
		}
	}
	if fmt.Sprint(values) != "[0 1 3]" { // 시작 0, 조각 1/3 (2/3 은 1초 안이라 건너뜀), 끝 3/3
		t.Errorf("진행 값: %v", values)
	}

	// progressToken 이 없으면 진행 알림 없이 JSON
	plain := c.do("POST", `{"jsonrpc":"2.0","id":10,"method":"tools/call","params":{"name":"test_chunks","arguments":{}}}`)
	if ct := plain.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("progressToken 없으면 JSON: %s", ct)
	}
	readJSON(t, plain)
}

// 취소 알림을 기다리는 가짜 도구.
func blockTool(started chan<- struct{}, stopped chan<- bool) *tool {
	return &tool{Name: "test_block", Description: "test", run: func(rc *runCtx, in args) (*toolResult, error) {
		close(started)
		select {
		case <-rc.ctx.Done():
			stopped <- true
			return nil, rc.ctx.Err()
		case <-time.After(5 * time.Second):
			stopped <- false
			return textResult("끝까지 돌았다"), nil
		}
	}}
}

func TestStdioCancelledStopsWork(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan bool, 1)
	s := newServer(nil, "dev", []string{"DEFAULTKEY000001"}, nil)
	s.tools = append(append([]*tool{}, tools...), blockTool(started, stopped))
	pr, pw := io.Pipe()
	var out strings.Builder
	var outMu sync.Mutex
	s.out = writerFunc(func(b []byte) (int, error) { outMu.Lock(); defer outMu.Unlock(); return out.Write(b) })
	done := make(chan error)
	go func() { done <- s.serve(pr) }()
	fmt.Fprintln(pw, `{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"test_block","arguments":{}}}`)
	<-started
	fmt.Fprintln(pw, `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":5,"reason":"user"}}`)
	if !<-stopped {
		t.Fatal("취소 알림으로 작업이 멈춰야 한다")
	}
	pw.Close()
	<-done
	outMu.Lock()
	defer outMu.Unlock()
	if strings.Contains(out.String(), `"id":5`) {
		t.Errorf("취소된 요청에는 답하지 않는다: %s", out.String())
	}
}

func TestHTTPCancelledStopsWork(t *testing.T) {
	started, stopped := make(chan struct{}), make(chan bool, 1)
	s := newServer(nil, "dev", []string{"DEFAULTKEY000001"}, nil)
	s.tools = append(append([]*tool{}, tools...), blockTool(started, stopped))
	c := startHTTP(t, s, httpConfig{})
	c.initialize()
	got := make(chan int)
	go func() {
		r := c.do("POST", `{"jsonrpc":"2.0","id":"x7","method":"tools/call","params":{"name":"test_block","arguments":{}}}`)
		io.Copy(io.Discard, r.Body)
		got <- r.StatusCode
	}()
	<-started
	if r := c.do("POST", `{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":"x7"}}`); r.StatusCode != 202 {
		t.Errorf("취소 알림 202: %d", r.StatusCode)
	}
	if !<-stopped {
		t.Fatal("취소 알림으로 작업이 멈춰야 한다")
	}
	if code := <-got; code != 202 {
		t.Errorf("취소된 요청의 POST 는 응답 없이 202: %d", code)
	}
}

func TestProgressThrottleAndNoToken(t *testing.T) {
	if p := newProgress(nil, func(any) { t.Fatal("토큰 없으면 보내지 않는다") }); p != nil {
		t.Fatal("토큰 없으면 nil")
	}
	var nilP *progress
	nilP.start("x")
	nilP.step(1, 2, "x")
	nilP.finish()

	var sent []float64
	p := newProgress(json.RawMessage(`7`), func(m any) {
		sent = append(sent, m.(map[string]any)["params"].(map[string]any)["progress"].(float64))
	})
	p.start("kosis_data")
	for i := 1; i <= 10; i++ {
		p.step(i, 10, "조각") // 모두 1초 안
	}
	p.finish()
	if fmt.Sprint(sent) != "[0 10]" {
		t.Errorf("1초 안 조각은 묶이고 끝은 늘 보낸다: %v", sent)
	}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }
