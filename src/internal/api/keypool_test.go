package api

import (
	"testing"
	"time"
)

// TestKeyPoolBorrowReturn borrow 한도(키 개수만큼만 동시 borrow) 및 return 해제를 검증합니다.
func TestKeyPoolBorrowReturn(t *testing.T) {
	c, err := NewClient([]string{"k0", "k1", "k2"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	// 키 3개를 모두 borrow → 서로 다른 인덱스여야 함
	got := map[int]bool{}
	for i := 0; i < 3; i++ {
		got[c.borrowKey()] = true
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 distinct key indices, got %d (%v)", len(got), got)
	}

	// 풀이 비었으므로 추가 borrow는 블록되어야 함
	select {
	case idx := <-c.keyPool:
		t.Fatalf("pool should be empty but yielded %d", idx)
	case <-time.After(50 * time.Millisecond):
		// 기대대로 블록됨
	}

	// 하나 반납하면 즉시 borrow 가능해야 함
	c.returnKey(1)
	select {
	case idx := <-c.keyPool:
		if idx != 1 {
			t.Fatalf("expected returned key 1, got %d", idx)
		}
	case <-time.After(50 * time.Millisecond):
		t.Fatal("returnKey did not make a key available")
	}
}

// TestKeyPoolCooldownReturn 쿨다운 반납이 지정 시간 후에만 키를 풀에 돌려놓는지 검증합니다.
func TestKeyPoolCooldownReturn(t *testing.T) {
	c, err := NewClient([]string{"k0"})
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	idx := c.borrowKey() // 유일 키 borrow → 풀 빔
	c.cooldownReturn(idx, 30*time.Millisecond)

	// 쿨다운 경과 전에는 비어 있어야 함
	select {
	case <-c.keyPool:
		t.Fatal("key was returned before cooldown elapsed")
	case <-time.After(10 * time.Millisecond):
		// 기대대로 아직 비어 있음
	}

	// 쿨다운 후에는 반납되어야 함
	select {
	case got := <-c.keyPool:
		if got != idx {
			t.Fatalf("expected cooled-down key %d, got %d", idx, got)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("cooldownReturn did not return key after cooldown")
	}
}

// TestKeyStartSpreads kosis 를 매번 새로 실행해도(= 클라이언트를 새로 만들어도) 0번 키만 쓰지 않아야 한다.
func TestKeyStartSpreads(t *testing.T) {
	keys := []string{"k0", "k1", "k2"}
	first := map[string]int{}
	for i := 0; i < 300; i++ {
		c, _ := NewClient(keys)
		k := c.getNextAPIKey()
		if p := keys[c.borrowKey()]; p != k {
			t.Fatalf("일반 요청(%s)과 키 풀(%s)의 시작 키가 다름", k, p)
		}
		first[k]++
	}
	for _, k := range keys {
		if first[k] < 50 {
			t.Fatalf("시작 키가 고르지 않음: %v", first)
		}
	}
}
