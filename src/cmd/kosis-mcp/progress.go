package main

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// progressInterval 중간 진행 알림 사이 최소 간격 (1초에 여러 번 보내지 않는다 — 사이 것은 건너뛴다).
const progressInterval = time.Second

// progress 요청 하나의 notifications/progress 보내기. progressToken 이 없으면(nil) 아무것도 보내지 않는다.
// progress 값은 늘기만 한다: 시작 0 → 조각 n (전체 total) → 끝.
type progress struct {
	token  json.RawMessage
	notify func(msg any)

	mu    sync.Mutex
	last  float64
	total int
	sent  time.Time
	tool  string
}

func newProgress(token json.RawMessage, notify func(any)) *progress {
	if len(token) == 0 || string(token) == "null" || notify == nil {
		return nil
	}
	return &progress{token: token, notify: notify}
}

func (p *progress) send(value float64, total int, msg string) {
	params := map[string]any{"progressToken": p.token, "progress": value, "message": msg}
	if total > 0 {
		params["total"] = total
	}
	p.notify(map[string]any{"jsonrpc": "2.0", "method": "notifications/progress", "params": params})
	p.last, p.sent = value, time.Now()
}

// start 도구 실행 시작.
func (p *progress) start(tool string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.tool = tool
	p.send(0, 0, tool+" 시작")
}

// step 조각 done/total 을 받았다 (1초 안에 또 오면 건너뛴다). nil 이면 아무 일도 안 한다.
func (p *progress) step(done, total int, what string) {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.total = total
	if float64(done) <= p.last || time.Since(p.sent) < progressInterval {
		return
	}
	p.send(float64(done), total, fmt.Sprintf("%s %d/%d", what, done, total))
}

// finish 도구 실행 끝 (항상 보낸다).
func (p *progress) finish() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	v, total := p.last+1, 0
	if p.total > 0 && float64(p.total) > p.last {
		v, total = float64(p.total), p.total
	} else if p.total > 0 { // 이미 마지막 조각을 보냈다 — 같은 값을 다시 보내지 않는다
		return
	}
	p.send(v, total, p.tool+" 끝")
}
