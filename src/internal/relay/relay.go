// Package relay 는 PES 로컬 중계 모드를 맡는다 (규약: pes-bridge RELAY.md).
//
// 외부 AI(pes-bridge)가 kosis CLI 를 돌릴 때 CLI 는 API 키를 받지 않는다. 환경변수 PES_RELAY_URL 이 있으면
// 키 대신 자리표시(__PES_KEY_0__ …)를 쓰고, KOSIS 로 가는 모든 HTTP(S) 요청을
// {PES_RELAY_URL}/kosis/<scheme>/<host>/<path>?<query> 로 보낸다. 중계가 자리표시를 실제 키로 바꿔 기관에 보낸다.
//
// 중계 모드는 CLI(cmd.Execute)가 EnableCLI 를 불렀을 때만 켜진다 — kosis-mcp 서버는 부르지 않으므로
// PES_RELAY_URL 이 있어도 지금처럼 자기 키로 KOSIS 를 직접 부른다.
package relay

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
)

const (
	// Tool 중계 주소의 <tool> 자리.
	Tool = "kosis"

	envURL   = "PES_RELAY_URL"
	envToken = "PES_RELAY_TOKEN"
	envKeys  = "PES_RELAY_KOSIS_KEYS"

	tokenHeader = "X-PES-Relay-Token"
	errorHeader = "X-PES-Relay-Error"

	// NoKeyMessage 키 수가 0 이거나 변수가 없을 때 알리는 글 (규약 3번 그대로).
	NoKeyMessage = "PES 에 " + Tool + " 키가 없어요. PES 「조사 도구 · 그림 키」에서 키를 넣으세요."

	maxErrorBody = 64 << 10 // 중계 오류 본문은 짧은 JSON — 그 이상은 읽지 않는다
)

var (
	cliEnabled atomic.Bool

	failMu  sync.Mutex
	failErr error // 이 프로세스에서 처음 난 중계 오류 · 키 없음 (있으면 더 부르지 않고 0 이 아닌 코드로 끝낸다)
)

// ErrNoKey PES 에 등록된 KOSIS 키가 없다.
var ErrNoKey = errors.New(NoKeyMessage)

// Error 중계가 스스로 낸 오류(토큰 틀림 401 · 허용 안 된 호스트 403 · 키 없음 409). 기관 응답이 아니다.
// 다른 키로 다시 해 보지 않고 error 글을 그대로 보여 주고 끝낸다.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// IsError err 가 중계 오류(또는 PES 키 없음)인지.
func IsError(err error) bool {
	var re *Error
	return errors.As(err, &re) || errors.Is(err, ErrNoKey)
}

// Unwrap err 안에 중계 오류 · 키 없음이 있으면 그것만 꺼낸다 (주소가 붙은 *url.Error 껍질을 벗긴다).
func Unwrap(err error) (error, bool) {
	var re *Error
	if errors.As(err, &re) {
		return re, true
	}
	if errors.Is(err, ErrNoKey) {
		return ErrNoKey, true
	}
	return nil, false
}

// EnableCLI kosis CLI 가 시작할 때 한 번 부른다. 부르지 않은 프로그램(kosis-mcp)은 중계 모드가 되지 않는다.
func EnableCLI() { cliEnabled.Store(true) }

// Active 중계 모드인지 — CLI 이고 PES_RELAY_URL 이 있을 때.
func Active() bool {
	return cliEnabled.Load() && baseURL() != ""
}

func baseURL() string {
	return strings.TrimRight(strings.TrimSpace(os.Getenv(envURL)), "/")
}

// KeyCount PES 에 등록된 KOSIS 키 수. 변수가 없거나 0 이상의 정수가 아니면 0.
func KeyCount() int {
	n, err := strconv.Atoi(strings.TrimSpace(os.Getenv(envKeys)))
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// Keys 중계 모드의 키 풀 — 자리표시 n 개. 키가 없으면 ErrNoKey (기관을 부르지 않고 끝내야 한다).
func Keys() ([]string, error) {
	n := KeyCount()
	if n == 0 {
		fail(ErrNoKey)
		return nil, ErrNoKey
	}
	keys := make([]string, n)
	for i := range keys {
		keys[i] = fmt.Sprintf("__PES_KEY_%d__", i)
	}
	return keys, nil
}

func fail(err error) {
	failMu.Lock()
	if failErr == nil {
		failErr = err
	}
	failMu.Unlock()
}

// Failed 이 프로세스에서 중계 오류나 PES 키 없음이 있었으면 그 오류. CLI 는 이때 0 이 아닌 코드로 끝난다.
func Failed() error {
	failMu.Lock()
	defer failMu.Unlock()
	return failErr
}

// Wrap 중계 모드에서만 base 에 중계 길목을 씌운다. 중계 모드가 아니면 base 를 그대로 돌려준다.
func Wrap(base http.RoundTripper) http.RoundTripper {
	if !Active() {
		return base
	}
	return &transport{base: base}
}

// transport 기관으로 가는 요청을 중계 주소로 바꿔 보내는 한 길목.
// http.Client 는 원래(기관) 주소로 요청을 만든다고 알고 있으므로, 주소 넘김(3xx)의 Location 도 원래 주소 기준으로 풀리고
// 다음 요청이 다시 이 길목을 지나 중계 주소로 바뀐다 — 중계 밖 주소로 따라가는 일이 없다.
type transport struct {
	base http.RoundTripper
}

func (t *transport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := Failed(); err != nil { // 중계 오류 뒤에는 더 부르지 않는다
		return nil, err
	}
	if KeyCount() == 0 {
		fail(ErrNoKey)
		return nil, ErrNoKey
	}
	target, err := rewrite(baseURL(), req.URL)
	if err != nil {
		return nil, err
	}
	r2 := req.Clone(req.Context())
	r2.URL = target
	r2.Host = target.Host
	r2.Header.Set(tokenHeader, os.Getenv(envToken))

	resp, err := t.base.RoundTrip(r2)
	if err != nil {
		return nil, err
	}
	if resp.Header.Get(errorHeader) == "1" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
		resp.Body.Close()
		re := &Error{Status: resp.StatusCode, Message: errorText(resp.StatusCode, body)}
		fail(re)
		return nil, re
	}
	return resp, nil
}

func (t *transport) CloseIdleConnections() {
	if c, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		c.CloseIdleConnections()
	}
}

// rewrite <scheme>://<host>[:port]/<path>?<query> → {base}/kosis/<scheme>/<host>[:port]/<path>?<query>.
// 이미 중계 주소면 그대로 둔다.
func rewrite(base string, u *url.URL) (*url.URL, error) {
	b, err := url.Parse(base)
	if err != nil || b.Host == "" || (b.Scheme != "http" && b.Scheme != "https") {
		return nil, fmt.Errorf("%s 주소가 올바르지 않아요", envURL)
	}
	prefix := b.EscapedPath() + "/" + Tool + "/"
	if u.Scheme == b.Scheme && u.Host == b.Host && strings.HasPrefix(u.EscapedPath(), prefix) {
		return u, nil
	}
	raw := b.Scheme + "://" + b.Host + prefix + u.Scheme + "/" + u.Host + u.EscapedPath()
	if u.RawQuery != "" || u.ForceQuery {
		raw += "?" + u.RawQuery
	}
	return url.Parse(raw)
}

func errorText(status int, body []byte) string {
	var v struct {
		Error string `json:"error"`
	}
	if json.Unmarshal(body, &v) == nil && strings.TrimSpace(v.Error) != "" {
		return v.Error
	}
	return fmt.Sprintf("PES 중계 오류 (%d)", status)
}

// ResetForTesting 테스트용 — CLI 표시를 on 으로 맞추고 기억한 오류를 지운다.
func ResetForTesting(on bool) {
	cliEnabled.Store(on)
	failMu.Lock()
	failErr = nil
	failMu.Unlock()
}
