package api

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestRunParallelChunks_Concurrency1Key 키 1개여도 concurrency>1이면 동시 요청이 발생하고,
// 모든 청크가 처리되며 순서 보존·중복 제거가 동작함을 검증합니다 (인수조건 1·5).
func TestRunParallelChunks_Concurrency1Key(t *testing.T) {
	c, err := NewClient([]string{"k0"}) // 키 1개
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	chunks := []PeriodChunk{
		{Start: "2016", End: "2016"}, {Start: "2017", End: "2017"},
		{Start: "2018", End: "2018"}, {Start: "2019", End: "2019"},
		{Start: "2020", End: "2020"}, {Start: "2021", End: "2021"},
	}

	var mu sync.Mutex
	fetchedStarts := map[string]int{}
	cur, maxConcurrent := 0, 0

	fetch := func(keyIdx int, chunk PeriodChunk) ([]DataRow, error) {
		mu.Lock()
		cur++
		if cur > maxConcurrent {
			maxConcurrent = cur
		}
		mu.Unlock()

		time.Sleep(25 * time.Millisecond) // 동시성 관찰용

		mu.Lock()
		cur--
		fetchedStarts[chunk.Start]++
		mu.Unlock()

		// 청크당 동일 행 2개 반환 → 중복 제거 검증
		row := DataRow{C1: "00", ItmID: "T10", PrdDe: chunk.Start}
		return []DataRow{row, row}, nil
	}

	rows, err := c.runParallelChunks(chunks, 4, nil, fetch) // concurrency 4, 키 1개
	if err != nil {
		t.Fatalf("runParallelChunks: %v", err)
	}

	// 동시성: 키 1개여도 4 워커로 동시 실행되어야 함
	if maxConcurrent < 2 {
		t.Fatalf("expected concurrent execution (>1) with 1 key, got max=%d", maxConcurrent)
	}
	// 모든 청크가 정확히 한 번씩 처리
	if len(fetchedStarts) != len(chunks) {
		t.Fatalf("expected %d distinct chunks fetched, got %d", len(chunks), len(fetchedStarts))
	}
	for _, ch := range chunks {
		if fetchedStarts[ch.Start] != 1 {
			t.Fatalf("chunk %s fetched %d times, want 1", ch.Start, fetchedStarts[ch.Start])
		}
	}
	// 중복 제거: 청크당 2행 반환했지만 dedup으로 1행씩 → 총 len(chunks)
	if len(rows) != len(chunks) {
		t.Fatalf("expected %d rows after dedup, got %d", len(chunks), len(rows))
	}
	// 순서 보존: PrdDe 오름차순
	for i := 1; i < len(rows); i++ {
		if rows[i-1].PrdDe > rows[i].PrdDe {
			t.Fatalf("rows not sorted by period: %s before %s", rows[i-1].PrdDe, rows[i].PrdDe)
		}
	}
}

// TestRunParallelChunks_Failover 키 하나가 429를 반환해도 나머지 청크가 가용 키로 완료됨을 검증합니다 (인수조건 2).
func TestRunParallelChunks_Failover(t *testing.T) {
	c, err := NewClient([]string{"k0", "k1"}) // 키 2개
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	chunks := []PeriodChunk{
		{Start: "2020", End: "2020"}, {Start: "2021", End: "2021"},
		{Start: "2022", End: "2022"}, {Start: "2023", End: "2023"},
	}

	var mu sync.Mutex
	key0Calls := 0

	fetch := func(keyIdx int, chunk PeriodChunk) ([]DataRow, error) {
		if keyIdx == 0 {
			mu.Lock()
			key0Calls++
			mu.Unlock()
			return nil, fmt.Errorf("HTTP 429 Too Many Requests")
		}
		return []DataRow{{C1: "00", ItmID: "T10", PrdDe: chunk.Start}}, nil
	}

	rows, err := c.runParallelChunks(chunks, 2, nil, fetch)
	if err != nil {
		t.Fatalf("runParallelChunks should not fail (failover via key1): %v", err)
	}
	// 키 0이 429를 반환했지만 모든 청크가 키 1로 완료되어야 함
	if len(rows) != len(chunks) {
		t.Fatalf("expected all %d chunks completed via failover, got %d rows", len(chunks), len(rows))
	}
	if key0Calls == 0 {
		t.Fatal("expected key0 to have been tried (and 429'd) at least once")
	}
}

// TestRunParallelChunks_429RequeueBounded 지속 429 시 splitter 레벨 재투입이 곱연산이 아니라
// 상한(초기 1회 + maxRequeue)으로 제한됨을 검증합니다 (설계서 Step 4: 재시도 단일화).
func TestRunParallelChunks_429RequeueBounded(t *testing.T) {
	c, err := NewClient([]string{"k0", "k1"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}
	chunks := []PeriodChunk{{Start: "2020", End: "2020"}}

	var mu sync.Mutex
	calls := 0
	fetch := func(keyIdx int, chunk PeriodChunk) ([]DataRow, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		return nil, fmt.Errorf("HTTP 429 Too Many Requests")
	}

	rows, err := c.runParallelChunks(chunks, 2, nil, fetch)
	if err != nil {
		t.Fatalf("persistent 429 should warn-continue, not fatal: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("expected 0 rows for persistent 429, got %d", len(rows))
	}
	// splitter 레벨 fetch 호출 = 초기 1 + maxRequeue(2) = 3. (이전엔 워커 3회 × requestWithKey 3회 = 곱연산)
	if calls != 3 {
		t.Fatalf("expected 3 splitter-level fetch calls (1 + maxRequeue 2), got %d", calls)
	}
}
