package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime/debug"
	"sync"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/cache"
)

const (
	protocolVersion = "2025-06-18"
	maxLineBytes    = 4 << 20 // 한 메시지 상한
	maxConcurrent   = 32      // 동시에 처리하는 tools/call 수
)

// supportedVersions 클라이언트가 이 중 하나를 보내면 그대로 돌려준다.
var supportedVersions = map[string]bool{"2025-06-18": true, "2025-03-26": true, "2024-11-05": true}

type server struct {
	out         io.Writer
	outMu       sync.Mutex
	version     string
	defaultKeys []string
	cache       *cache.Cache
	sem         chan struct{}
	wg          sync.WaitGroup

	// newClient 요청마다 그 요청의 키로 클라이언트를 만든다 (테스트에서 바꾼다).
	newClient func(keys []string) (*api.Client, error)
}

func newServer(out io.Writer, version string, defaultKeys []string, fc *cache.Cache) *server {
	s := &server{out: out, version: version, defaultKeys: defaultKeys, cache: fc, sem: make(chan struct{}, maxConcurrent)}
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

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

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
			s.writeError(json.RawMessage("null"), -32600, fmt.Sprintf("메시지가 너무 깁니다 (%d바이트 초과)", maxLineBytes))
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
			return nil, errTooLongOr(err)
		}
		if err != nil || !isPrefix {
			return buf, err
		}
	}
}

func errTooLongOr(err error) error {
	if err != nil {
		return err
	}
	return errTooLong
}

func (s *server) handleLine(line []byte) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		s.writeError(json.RawMessage("null"), -32600, "배치 요청은 지원하지 않습니다")
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(trimmed, &req); err != nil {
		s.writeError(json.RawMessage("null"), -32700, "JSON 파싱 실패")
		return
	}
	isNotification := len(req.ID) == 0
	if req.Method == "" { // 클라이언트가 보낸 응답 — 이 서버는 요청을 보내지 않으니 무시
		return
	}
	if isNotification {
		return // notifications/initialized, notifications/cancelled 등
	}
	switch req.Method {
	case "initialize":
		s.writeResult(req.ID, s.initialize(req.Params))
	case "ping":
		s.writeResult(req.ID, map[string]any{})
	case "tools/list":
		s.writeResult(req.ID, map[string]any{"tools": toolList()})
	case "tools/call":
		s.wg.Add(1)
		s.sem <- struct{}{}
		go func() {
			defer s.wg.Done()
			defer func() { <-s.sem }()
			s.toolsCall(req.ID, req.Params)
		}()
	default:
		s.writeError(req.ID, -32601, "지원하지 않는 메서드: "+req.Method)
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

func (s *server) toolsCall(id json.RawMessage, raw json.RawMessage) {
	var p callParams
	if err := json.Unmarshal(raw, &p); err != nil {
		s.writeError(id, -32602, "tools/call 인자 형식이 맞지 않습니다")
		return
	}
	t, ok := toolByName(p.Name)
	if !ok {
		s.writeError(id, -32602, "알 수 없는 도구: "+p.Name)
		return
	}
	keys, _, kerr := selectKeys(p.Meta, s.defaultKeys)
	text, isErr := s.runTool(t, p.Arguments, keys, kerr)
	s.writeResult(id, map[string]any{
		"content": []map[string]any{{"type": "text", "text": s.scrub(text, keys)}},
		"isError": isErr,
	})
}

// runTool 인자 검증 → 키 확인 → 클라이언트 → 실행. 패닉도 오류 결과로.
func (s *server) runTool(t *tool, args json.RawMessage, keys []string, keyErr error) (text string, isErr bool) {
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
		return "클라이언트를 만들지 못했습니다: " + err.Error(), true
	}
	defer client.CloseIdleConnections()
	out, err := t.run(client, in)
	if err != nil {
		return err.Error(), true
	}
	return out, false
}

// scrub 출력 직전: 이 요청의 키와 기본 키를 모두 가린다.
func (s *server) scrub(text string, reqKeys []string) string {
	return scrubKeys(text, append(append([]string(nil), reqKeys...), s.defaultKeys...))
}

func (s *server) writeResult(id json.RawMessage, result any) {
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func (s *server) writeError(id json.RawMessage, code int, msg string) {
	s.write(map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcError{Code: code, Message: s.scrub(msg, nil)}})
}

func (s *server) write(v any) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil { // Encode 가 끝에 \n 을 붙인다
		fmt.Fprintf(os.Stderr, "kosis-mcp: 응답 인코딩 실패: %v\n", err)
		return
	}
	s.outMu.Lock()
	defer s.outMu.Unlock()
	if _, err := s.out.Write(buf.Bytes()); err != nil {
		fmt.Fprintf(os.Stderr, "kosis-mcp: 응답 쓰기 실패: %v\n", err)
	}
}
