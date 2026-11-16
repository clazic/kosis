package main

// 단독 Streamable HTTP 모드 (MCP 2025-06-18). 경로는 /mcp 하나.
//   POST   JSON-RPC 하나 또는 배치(≤10). 진행 알림을 보낼 수 있고 Accept 에 text/event-stream 이 있으면 SSE, 아니면 JSON.
//          알림·응답만 온 POST 는 202.
//   GET    세션의 서버 알림 SSE 스트림 (15초마다 ": ping").
//   DELETE 세션 끝.
// 직접 구현이다 (공식 go-sdk 를 넣지 않음 — 도구·키·진행·취소 코드를 stdio 와 그대로 함께 쓰려고).

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	defaultHTTPAddr = "127.0.0.1:8080"
	maxBodyBytes    = 1 << 20
	maxBatch        = 10
	sessionIdle     = time.Hour
	keepAlive       = 15 * time.Second
	sessionHeader   = "Mcp-Session-Id"
)

// httpConfig HTTP 모드 설정 (환경변수 KOSIS_MCP_TOKEN, KOSIS_MCP_ALLOWED_ORIGINS).
type httpConfig struct {
	Token          string
	AllowedOrigins []string
}

func httpConfigFromEnv() httpConfig {
	return httpConfig{
		Token:          strings.TrimSpace(os.Getenv("KOSIS_MCP_TOKEN")),
		AllowedOrigins: splitKeys(os.Getenv("KOSIS_MCP_ALLOWED_ORIGINS")),
	}
}

// checkListen loopback(127.0.0.1·::1·localhost)이 아닌 주소는 토큰 없이 시작하지 않는다.
func checkListen(addr, token string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("주소 형식이 맞지 않습니다 (예: 127.0.0.1:8080): %v", err)
	}
	if isLoopbackHost(host) || token != "" {
		return nil
	}
	return fmt.Errorf("%s 은(는) loopback 주소가 아닙니다 — KOSIS_MCP_TOKEN 없이는 시작하지 않습니다", addr)
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

type httpSession struct {
	id       string
	mu       sync.Mutex
	lastUsed time.Time
	inflight inflightSet
	events   chan []byte   // GET 스트림으로 보낼 서버 알림
	closed   chan struct{} // DELETE·만료
}

type httpServer struct {
	s   *server
	cfg httpConfig

	mu       sync.Mutex
	sessions map[string]*httpSession
}

func newHTTPServer(s *server, cfg httpConfig) *httpServer {
	return &httpServer{s: s, cfg: cfg, sessions: map[string]*httpSession{}}
}

// serveHTTP addr 에서 듣는다 (시작 전 checkListen).
func (h *httpServer) listenAndServe(addr string) error {
	if err := checkListen(addr, h.cfg.Token); err != nil {
		return err
	}
	go h.reap()
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	fmt.Fprintf(os.Stderr, "kosis-mcp: HTTP http://%s/mcp (토큰 %s)\n", addr, map[bool]string{true: "필요", false: "없음"}[h.cfg.Token != ""])
	return srv.ListenAndServe()
}

// reap 1시간 쓰지 않은 세션을 버린다.
func (h *httpServer) reap() {
	for range time.Tick(time.Minute) {
		h.expire(time.Now())
	}
}

func (h *httpServer) expire(now time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for id, ss := range h.sessions {
		ss.mu.Lock()
		idle := now.Sub(ss.lastUsed)
		ss.mu.Unlock()
		if idle > sessionIdle {
			h.dropLocked(id)
		}
	}
}

func (h *httpServer) dropLocked(id string) {
	if ss := h.sessions[id]; ss != nil {
		ss.inflight.cancelAll()
		close(ss.closed)
		delete(h.sessions, id)
	}
}

func (h *httpServer) newSession() *httpSession {
	b := make([]byte, 24) // 192bit
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	ss := &httpSession{id: hex.EncodeToString(b), lastUsed: time.Now(), events: make(chan []byte, 16), closed: make(chan struct{})}
	h.mu.Lock()
	h.sessions[ss.id] = ss
	h.mu.Unlock()
	return ss
}

// session 머리글의 세션. 머리글 없음 400, 모르는 세션 404.
func (h *httpServer) session(w http.ResponseWriter, r *http.Request) *httpSession {
	id := r.Header.Get(sessionHeader)
	if id == "" {
		httpError(w, http.StatusBadRequest, "Mcp-Session-Id 머리글이 필요합니다 (initialize 응답의 값)")
		return nil
	}
	h.mu.Lock()
	ss := h.sessions[id]
	h.mu.Unlock()
	if ss == nil {
		httpError(w, http.StatusNotFound, "모르는 세션입니다 — 다시 initialize 하세요")
		return nil
	}
	ss.mu.Lock()
	ss.lastUsed = time.Now()
	ss.mu.Unlock()
	return ss
}

func httpError(w http.ResponseWriter, code int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	b, _ := json.Marshal(errorMsg(json.RawMessage("null"), -32600, msg))
	w.Write(b)
}

// ServeHTTP 출처·인증·버전 검사 뒤 메서드별 처리.
func (h *httpServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/mcp" {
		http.NotFound(w, r)
		return
	}
	if o := r.Header.Get("Origin"); o != "" && !h.originAllowed(o) {
		httpError(w, http.StatusForbidden, "허용되지 않은 Origin 입니다")
		return
	}
	if h.cfg.Token != "" && !tokenOK(r.Header.Get("Authorization"), h.cfg.Token) {
		w.Header().Set("WWW-Authenticate", `Bearer realm="kosis-mcp"`)
		httpError(w, http.StatusUnauthorized, "인증이 필요합니다 (Authorization: Bearer <토큰>)")
		return
	}
	if v := r.Header.Get("MCP-Protocol-Version"); v != "" && !supportedVersions[v] {
		httpError(w, http.StatusBadRequest, "모르는 MCP-Protocol-Version: "+v)
		return
	}
	switch r.Method {
	case http.MethodPost:
		h.post(w, r)
	case http.MethodGet:
		h.get(w, r)
	case http.MethodDelete:
		h.mu.Lock()
		_, ok := h.sessions[r.Header.Get(sessionHeader)]
		if ok {
			h.dropLocked(r.Header.Get(sessionHeader))
		}
		h.mu.Unlock()
		if !ok {
			if r.Header.Get(sessionHeader) == "" {
				httpError(w, http.StatusBadRequest, "Mcp-Session-Id 머리글이 필요합니다")
			} else {
				httpError(w, http.StatusNotFound, "모르는 세션입니다")
			}
			return
		}
		w.WriteHeader(http.StatusOK)
	default:
		w.Header().Set("Allow", "GET, POST, DELETE")
		httpError(w, http.StatusMethodNotAllowed, "GET·POST·DELETE 만 씁니다")
	}
}

// originAllowed loopback 출처(http(s)://localhost·127.0.0.1·[::1]:*)이거나 KOSIS_MCP_ALLOWED_ORIGINS 안.
func (h *httpServer) originAllowed(o string) bool {
	for _, a := range h.cfg.AllowedOrigins {
		if strings.EqualFold(strings.TrimRight(a, "/"), strings.TrimRight(o, "/")) {
			return true
		}
	}
	u, err := url.Parse(o)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return false
	}
	return isLoopbackHost(u.Hostname())
}

// tokenOK 상수 시간 비교 (길이도 드러나지 않게 해시끼리).
func tokenOK(header, token string) bool {
	got, ok := strings.CutPrefix(header, "Bearer ")
	if !ok {
		return false
	}
	a, b := sha256.Sum256([]byte(strings.TrimSpace(got))), sha256.Sum256([]byte(token))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

func acceptsSSE(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "text/event-stream")
}

func (h *httpServer) post(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBodyBytes))
	if err != nil {
		httpError(w, http.StatusRequestEntityTooLarge, fmt.Sprintf("본문이 너무 큽니다 (%d바이트 상한)", maxBodyBytes))
		return
	}
	body = bytes.TrimSpace(body)
	var msgs []rpcRequest
	batch := len(body) > 0 && body[0] == '['
	if batch {
		if json.Unmarshal(body, &msgs) != nil || len(msgs) == 0 {
			httpError(w, http.StatusBadRequest, "JSON-RPC 배치 형식이 맞지 않습니다")
			return
		}
		if len(msgs) > maxBatch {
			httpError(w, http.StatusBadRequest, fmt.Sprintf("배치는 %d개까지입니다", maxBatch))
			return
		}
	} else {
		var m rpcRequest
		if json.Unmarshal(body, &m) != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write(h.s.encodeMsg(errorMsg(json.RawMessage("null"), -32700, "JSON 파싱 실패"), nil))
			return
		}
		msgs = []rpcRequest{m}
	}

	// 세션: initialize 가 든 POST 는 새 세션, 나머지는 머리글의 세션
	var ss *httpSession
	hasInit := false
	for _, m := range msgs {
		if m.Method == "initialize" && !m.isNotification() {
			hasInit = true
		}
	}
	if hasInit {
		ss = h.newSession()
		w.Header().Set(sessionHeader, ss.id)
	} else if ss = h.session(w, r); ss == nil {
		return
	}

	// 요청별 키 머리글 (형식 오류면 그 POST 의 tools/call 모두 오류 — 기본 키로 넘어가지 않는다)
	var hdrKeys []string
	var hdrErr error
	hasHdr := false
	if v, ok := r.Header["X-Api-Keys"]; ok {
		hasHdr = true
		list := splitKeys(strings.Join(v, ","))
		raw, _ := json.Marshal(list)
		hdrKeys, _, hdrErr = selectKeys(map[string]json.RawMessage{metaKeysField: raw}, nil)
		if hdrErr != nil {
			hdrErr = errors.New(strings.ReplaceAll(hdrErr.Error(), `_meta["`+metaKeysField+`"]`, "X-Api-Keys 머리글"))
		}
	}

	var reqs []rpcRequest
	sse := false
	for _, m := range msgs {
		switch {
		case m.Method == "": // 클라이언트의 응답 — 무시
		case m.isNotification():
			h.s.handleNotification(&ss.inflight, m)
		default:
			reqs = append(reqs, m)
			if m.Method == "tools/call" && hasProgressToken(m.Params) {
				sse = true
			}
		}
	}
	if len(reqs) == 0 {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	sse = sse && acceptsSSE(r)

	var out *sseWriter
	if sse {
		out = newSSEWriter(w)
		if out == nil {
			sse = false
		}
	}
	var notify func(any)
	if sse {
		notify = func(m any) { out.event(h.s.encodeMsg(m, hdrKeys)) }
	}
	env := callEnv{ctx: r.Context(), notify: notify, hdrKeys: hdrKeys, hasHdr: hasHdr, inflight: &ss.inflight}

	responses := make([]map[string]any, len(reqs))
	var wg sync.WaitGroup
	for i, m := range reqs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			var resp map[string]any
			if hdrErr != nil && m.Method == "tools/call" {
				resp = resultMsg(m.ID, map[string]any{"content": []map[string]any{{"type": "text", "text": hdrErr.Error()}}, "isError": true})
			} else {
				resp = h.s.handleRequest(env, m)
			}
			if sse && resp != nil {
				out.event(h.s.encodeMsg(resp, hdrKeys))
			}
			responses[i] = resp
		}()
	}
	wg.Wait()
	if sse {
		return // 모든 응답 이벤트를 보냈다 — 스트림을 닫는다
	}
	var kept []map[string]any
	for _, resp := range responses {
		if resp != nil {
			kept = append(kept, resp)
		}
	}
	if len(kept) == 0 { // 모두 취소됨 — 답할 것이 없다
		w.WriteHeader(http.StatusAccepted)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if batch {
		parts := make([][]byte, len(kept))
		for i, k := range kept {
			parts[i] = h.s.encodeMsg(k, hdrKeys)
		}
		w.Write(append(append([]byte("["), bytes.Join(parts, []byte(","))...), ']'))
		return
	}
	w.Write(h.s.encodeMsg(kept[0], hdrKeys))
}

func hasProgressToken(params json.RawMessage) bool {
	var p struct {
		Meta map[string]json.RawMessage `json:"_meta"`
	}
	if json.Unmarshal(params, &p) != nil {
		return false
	}
	t, ok := p.Meta["progressToken"]
	return ok && string(t) != "null"
}

// get 세션의 서버 알림 SSE 스트림. 보낼 게 없으면 15초마다 ": ping".
func (h *httpServer) get(w http.ResponseWriter, r *http.Request) {
	if !acceptsSSE(r) {
		httpError(w, http.StatusNotAcceptable, "GET /mcp 는 Accept: text/event-stream 이어야 합니다")
		return
	}
	ss := h.session(w, r)
	if ss == nil {
		return
	}
	out := newSSEWriter(w)
	if out == nil {
		httpError(w, http.StatusInternalServerError, "스트리밍을 지원하지 않습니다")
		return
	}
	t := time.NewTicker(keepAlive)
	defer t.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ss.closed:
			return
		case ev := <-ss.events:
			out.event(ev)
		case <-t.C:
			out.comment("ping")
			ss.mu.Lock()
			ss.lastUsed = time.Now() // 열린 스트림이 있는 세션은 살아 있다
			ss.mu.Unlock()
		}
	}
}

// sseWriter SSE 이벤트를 차례로 쓰고 바로 내보낸다.
type sseWriter struct {
	mu sync.Mutex
	w  http.ResponseWriter
	f  http.Flusher
}

func newSSEWriter(w http.ResponseWriter) *sseWriter {
	f, ok := w.(http.Flusher)
	if !ok {
		return nil
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	f.Flush()
	return &sseWriter{w: w, f: f}
}

func (s *sseWriter) event(data []byte) {
	if data == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.w, "event: message\ndata: %s\n\n", data)
	s.f.Flush()
}

func (s *sseWriter) comment(c string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fmt.Fprintf(s.w, ": %s\n\n", c)
	s.f.Flush()
}
