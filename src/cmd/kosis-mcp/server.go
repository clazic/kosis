package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/cache"
)

const (
	protocolVersion = "2025-06-18"
	maxLineBytes    = 4 << 20 // stdio 한 메시지 상한
	maxConcurrent   = 32      // stdio 에서 동시에 처리하는 tools/call 수
)

// supportedVersions 클라이언트가 이 중 하나를 보내면 그대로 돌려준다 (HTTP 의 MCP-Protocol-Version 머리글도 이 안이어야 한다).
var supportedVersions = map[string]bool{"2025-06-18": true, "2025-03-26": true, "2024-11-05": true}

// server stdio·HTTP 가 함께 쓰는 핵심: 메서드 처리, 도구 실행, 키 선택, 진행 알림, 키 지우기.
type server struct {
	version     string
	defaultKeys []string
	cache       *cache.Cache
	tools       []*tool

	// newClient 요청마다 그 요청의 키로 클라이언트를 만든다 (테스트에서 바꾼다).
	newClient func(keys []string) (*api.Client, error)

	// stdio
	out      io.Writer
	outMu    sync.Mutex
	sem      chan struct{}
	wg       sync.WaitGroup
	inflight inflightSet
}

func newServer(out io.Writer, version string, defaultKeys []string, fc *cache.Cache) *server {
	s := &server{out: out, version: version, defaultKeys: defaultKeys, cache: fc, tools: tools,
		sem: make(chan struct{}, maxConcurrent)}
	s.newClient = func(keys []string) (*api.Client, error) {
		c, err := api.NewClient(keys)
		if err != nil {
			return nil, err
		}
		if s.cache != nil {
			c.SetCache(s.cache)
		}
		return c, nil
	}
	return s
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

func (r rpcRequest) isNotification() bool { return len(r.ID) == 0 }

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func resultMsg(id json.RawMessage, result any) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "result": result}
}

func errorMsg(id json.RawMessage, code int, msg string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcError{Code: code, Message: msg}}
}

// idKey 요청 id 를 취소 표에 쓸 문자열로 (3 과 "3" 은 다르다).
func idKey(id json.RawMessage) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, id); err != nil {
		return string(id)
	}
	return buf.String()
}

// inflightSet 진행 중 tools/call 의 취소 함수 (id → cancel).
type inflightSet struct {
	mu sync.Mutex
	m  map[string]context.CancelFunc
}

func (f *inflightSet) add(id string, cancel context.CancelFunc) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.m == nil {
		f.m = map[string]context.CancelFunc{}
	}
	f.m[id] = cancel
}

func (f *inflightSet) remove(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, id)
}

// cancel notifications/cancelled 의 params 를 받아 그 요청을 멈춘다.
func (f *inflightSet) cancel(params json.RawMessage) {
	var p struct {
		RequestID json.RawMessage `json:"requestId"`
	}
	if json.Unmarshal(params, &p) != nil || len(p.RequestID) == 0 {
		return
	}
	f.mu.Lock()
	c := f.m[idKey(p.RequestID)]
	f.mu.Unlock()
	if c != nil {
		c()
	}
}

func (f *inflightSet) cancelAll() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, c := range f.m {
		c()
	}
}

// callEnv 요청 하나를 처리할 때의 환경 (전송 방식마다 다르다).
type callEnv struct {
	ctx      context.Context
	notify   func(msg any) // 이 요청에 묶인 알림(진행) 보내기
	hdrKeys  []string      // HTTP X-Api-Keys (있으면 _meta 가 없는 tools/call 에 요청별 키로)
	hasHdr   bool
	inflight *inflightSet
}

// handleRequest id 가 있는 요청 하나 → 응답. nil 이면 답하지 않는다(취소됨).
func (s *server) handleRequest(env callEnv, req rpcRequest) map[string]any {
	switch req.Method {
	case "initialize":
		return resultMsg(req.ID, s.initialize(req.Params))
	case "ping":
		return resultMsg(req.ID, map[string]any{})
	case "tools/list":
		return resultMsg(req.ID, map[string]any{"tools": s.toolList()})
	case "tools/call":
		return s.toolsCall(env, req.ID, req.Params)
	default:
		return errorMsg(req.ID, -32601, "지원하지 않는 메서드: "+req.Method)
	}
}

// handleNotification 클라이언트 알림: 취소만 처리하고 나머지(initialized 등)는 무시.
func (s *server) handleNotification(set *inflightSet, req rpcRequest) {
	if req.Method == "notifications/cancelled" {
		set.cancel(req.Params)
	}
}

func (s *server) initialize(params json.RawMessage) map[string]any {
	var p struct {
		ProtocolVersion string `json:"protocolVersion"`
	}
	_ = json.Unmarshal(params, &p)
	v := protocolVersion
	if supportedVersions[p.ProtocolVersion] {
		v = p.ProtocolVersion
	}
	return map[string]any{
		"protocolVersion": v,
		"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}},
		"serverInfo":      map[string]any{"name": "kosis-mcp", "version": s.version},
		"instructions": "KOSIS(국가통계포털) 읽기 전용 조회. 순서: kosis_search(표 찾기) → kosis_meta(분류·항목 코드와 주기 확인) → kosis_data(값 조회). " +
			"코드는 짐작하지 말고 kosis_meta 결과를 그대로 쓴다. kosis_data 의 notices(결과에 없는 코드·잠정치·전망 시점·단위)는 답에 반영한다.",
	}
}

type callParams struct {
	Name      string                     `json:"name"`
	Arguments json.RawMessage            `json:"arguments"`
	Meta      map[string]json.RawMessage `json:"_meta"`
}

// callKeys tools/call 의 키: _meta["kr.pes/keys"] → HTTP X-Api-Keys → 기본 키. 요청별 키가 있으면 기본 키로 넘어가지 않는다.
func (s *server) callKeys(env callEnv, meta map[string]json.RawMessage) ([]string, error) {
	if _, ok := meta[metaKeysField]; ok || !env.hasHdr {
		keys, _, err := selectKeys(meta, s.defaultKeys)
		return keys, err
	}
	return env.hdrKeys, nil
}

func (s *server) toolsCall(env callEnv, id json.RawMessage, raw json.RawMessage) map[string]any {
	var p callParams
	if err := json.Unmarshal(raw, &p); err != nil {
		return errorMsg(id, -32602, "tools/call 인자 형식이 맞지 않습니다")
	}
	t, ok := s.toolByName(p.Name)
	if !ok {
		return errorMsg(id, -32602, "알 수 없는 도구: "+p.Name)
	}
	ctx, cancel := context.WithCancel(env.ctx)
	defer cancel()
	if env.inflight != nil {
		k := idKey(id)
		env.inflight.add(k, cancel)
		defer env.inflight.remove(k)
	}
	keys, kerr := s.callKeys(env, p.Meta)
	prog := newProgress(p.Meta["progressToken"], env.notify)
	start := time.Now()
	text, isErr := s.runTool(ctx, t, p.Arguments, keys, kerr, prog)
	if ctx.Err() != nil { // 취소됨 (notifications/cancelled · 연결 끊김) — 규격대로 답하지 않는다
		accessLog("tools/call", t.Name, start, "취소")
		return nil
	}
	accessLog("tools/call", t.Name, start, "")
	return resultMsg(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
		"isError": isErr,
	})
}

// accessLog stderr 접근 기록: 메서드·도구 이름·걸린 시간만 (인자·키·토큰 없음).
func accessLog(method, tool string, start time.Time, note string) {
	if note != "" {
		note = " " + note
	}
	fmt.Fprintf(os.Stderr, "kosis-mcp: %s %s %s%s\n", method, tool, time.Since(start).Round(time.Millisecond), note)
}

// runTool 인자 검증 → 키 확인 → 클라이언트 → 실행 (시작·끝 진행 알림). 패닉도 오류 결과로.
func (s *server) runTool(ctx context.Context, t *tool, args json.RawMessage, keys []string, keyErr error, prog *progress) (text string, isErr bool) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Fprintf(os.Stderr, "kosis-mcp: %s 처리 중 패닉: %s\n", t.Name, s.scrub(fmt.Sprintf("%v\n%s", r, debug.Stack()), keys))
			text, isErr = "내부 오류가 났습니다 — 인자를 바꿔 다시 시도하세요", true
		}
	}()
	in, err := validateArgs(t, args)
	if err != nil {
		return "인자 오류: " + err.Error(), true
	}
	if keyErr != nil {
		return keyErr.Error(), true
	}
	client, err := s.newClient(keys)
	if err != nil {
		return s.scrub("클라이언트를 만들지 못했습니다: "+err.Error(), keys), true
	}
	client.SetContext(ctx)
	defer client.CloseIdleConnections()
	prog.start(t.Name)
	out, err := t.run(ctx, client, in, prog)
	prog.finish()
	if err != nil {
		return s.scrub(err.Error(), keys), true
	}
	return s.scrub(out, keys), false
}

// scrub 출력 직전: 이 요청의 키와 기본 키를 모두 가린다.
func (s *server) scrub(text string, reqKeys []string) string {
	return scrubKeys(text, append(append([]string(nil), reqKeys...), s.defaultKeys...))
}

// encodeMsg JSON-RPC 메시지 한 줄 (키를 지운 뒤).
func (s *server) encodeMsg(v any, keys []string) []byte {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		fmt.Fprintf(os.Stderr, "kosis-mcp: 응답 인코딩 실패: %v\n", err)
		return nil
	}
	return []byte(s.scrub(strings.TrimRight(buf.String(), "\r\n"), keys))
}

// ── stdio ─────────────────────────────────────────────────────

// serve stdin 을 줄 단위로 읽어 처리한다. EOF 면 처리 중인 요청을 마치고 끝낸다.
func (s *server) serve(in io.Reader) error {
	r := bufio.NewReaderSize(in, 64<<10)
	defer s.wg.Wait()
	for {
		line, err := readLine(r)
		if len(bytes.TrimSpace(line)) > 0 {
			s.handleLine(line)
		}
		if err == io.EOF {
			return nil
		}
		if errors.Is(err, errTooLong) {
			s.write(errorMsg(json.RawMessage("null"), -32600, fmt.Sprintf("메시지가 너무 깁니다 (%d바이트 초과)", maxLineBytes)))
			continue
		}
		if err != nil {
			return err
		}
	}
}

var errTooLong = errors.New("메시지가 너무 깁니다")

func readLine(r *bufio.Reader) ([]byte, error) {
	var buf []byte
	for {
		chunk, isPrefix, err := r.ReadLine()
		buf = append(buf, chunk...)
		if len(buf) > maxLineBytes {
			for isPrefix && err == nil { // 나머지는 버린다
				_, isPrefix, err = r.ReadLine()
			}
			if err != nil {
				return nil, err
			}
			return nil, errTooLong
		}
		if err != nil || !isPrefix {
			return buf, err
		}
	}
}

func (s *server) handleLine(line []byte) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		s.write(errorMsg(json.RawMessage("null"), -32600, "stdio 에서는 배치 요청을 지원하지 않습니다"))
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(trimmed, &req); err != nil {
		s.write(errorMsg(json.RawMessage("null"), -32700, "JSON 파싱 실패"))
		return
	}
	if req.Method == "" { // 클라이언트가 보낸 응답 — 이 서버는 요청을 보내지 않으니 무시
		return
	}
	if req.isNotification() {
		s.handleNotification(&s.inflight, req)
		return
	}
	if req.Method != "tools/call" {
		s.write(s.handleRequest(callEnv{ctx: context.Background()}, req))
		return
	}
	s.wg.Add(1)
	s.sem <- struct{}{}
	go func() {
		defer s.wg.Done()
		defer func() { <-s.sem }()
		env := callEnv{ctx: context.Background(), notify: s.write, inflight: &s.inflight}
		if resp := s.handleRequest(env, req); resp != nil {
			s.write(resp)
		}
	}()
}

// write stdout 에 JSON-RPC 한 줄 (기본 키를 지운 뒤 — 도구 결과는 runTool 에서 요청 키까지 지웠다).
func (s *server) write(v any) {
	b := s.encodeMsg(v, nil)
	if b == nil {
		return
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	if _, err := s.out.Write(append(b, '\n')); err != nil {
		fmt.Fprintf(os.Stderr, "kosis-mcp: 응답 쓰기 실패: %v\n", err)
	}
}
