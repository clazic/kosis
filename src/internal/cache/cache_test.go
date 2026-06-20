package cache

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCacheSetAndGet(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 24)
	if err != nil {
		t.Fatalf("캐시 생성 실패: %v", err)
	}

	key := "test_key"
	value := []byte("test_value")

	// Set 테스트
	if err := c.Set(key, value); err != nil {
		t.Fatalf("캐시 저장 실패: %v", err)
	}

	// Get 테스트
	data, found := c.Get(key)
	if !found {
		t.Fatal("캐시 조회 실패: 데이터를 찾을 수 없습니다")
	}

	if string(data) != string(value) {
		t.Fatalf("캐시 데이터 불일치: 예상=%s, 실제=%s", value, data)
	}
}

func TestCacheExpiration(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 0) // TTL 0시간 (즉시 만료)
	if err != nil {
		t.Fatalf("캐시 생성 실패: %v", err)
	}

	key := "test_key"
	value := []byte("test_value")

	if err := c.Set(key, value); err != nil {
		t.Fatalf("캐시 저장 실패: %v", err)
	}

	// 만료되도록 대기
	time.Sleep(100 * time.Millisecond)

	// Get 테스트 (만료되어 false 반환)
	_, found := c.Get(key)
	if found {
		t.Fatal("만료된 캐시가 반환되었습니다")
	}
}

func TestCacheClear(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 24)
	if err != nil {
		t.Fatalf("캐시 생성 실패: %v", err)
	}

	// 여러 캐시 항목 저장
	for i := 0; i < 3; i++ {
		key := "key_" + string(rune(i))
		value := []byte("value_" + string(rune(i)))
		if err := c.Set(key, value); err != nil {
			t.Fatalf("캐시 저장 실패: %v", err)
		}
	}

	// Clear 테스트
	if err := c.Clear(); err != nil {
		t.Fatalf("캐시 삭제 실패: %v", err)
	}

	// 모든 캐시가 삭제되었는지 확인
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("디렉토리 읽기 실패: %v", err)
	}

	if len(entries) > 0 {
		t.Fatalf("캐시 삭제 실패: %d개 파일이 남아있습니다", len(entries))
	}
}

func TestCacheSize(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 24)
	if err != nil {
		t.Fatalf("캐시 생성 실패: %v", err)
	}

	key := "test_key"
	value := []byte("test_value_12345")

	if err := c.Set(key, value); err != nil {
		t.Fatalf("캐시 저장 실패: %v", err)
	}

	size, err := c.Size()
	if err != nil {
		t.Fatalf("캐시 크기 조회 실패: %v", err)
	}

	expectedSize := int64(len(value))
	if size < expectedSize {
		t.Fatalf("캐시 크기 불일치: 예상=%d, 실제=%d", expectedSize, size)
	}
}

func TestCacheMultipleKeys(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 24)
	if err != nil {
		t.Fatalf("캐시 생성 실패: %v", err)
	}

	// 여러 개의 서로 다른 키로 데이터 저장
	testData := map[string]string{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	for k, v := range testData {
		if err := c.Set(k, []byte(v)); err != nil {
			t.Fatalf("캐시 저장 실패 (key=%s): %v", k, err)
		}
	}

	// 각 키의 데이터가 올바르게 저장되었는지 확인
	for k, expectedV := range testData {
		data, found := c.Get(k)
		if !found {
			t.Fatalf("캐시 조회 실패 (key=%s)", k)
		}
		if string(data) != expectedV {
			t.Fatalf("캐시 데이터 불일치 (key=%s): 예상=%s, 실제=%s", k, expectedV, string(data))
		}
	}
}

func TestCacheCleanExpired(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 0) // TTL 0시간 (즉시 만료)
	if err != nil {
		t.Fatalf("캐시 생성 실패: %v", err)
	}

	// 캐시 항목 저장
	for i := 0; i < 3; i++ {
		key := "key_" + string(rune(i))
		value := []byte("value_" + string(rune(i)))
		if err := c.Set(key, value); err != nil {
			t.Fatalf("캐시 저장 실패: %v", err)
		}
	}

	// 만료되도록 대기
	time.Sleep(100 * time.Millisecond)

	// CleanExpired 테스트
	if err := c.CleanExpired(); err != nil {
		t.Fatalf("캐시 정리 실패: %v", err)
	}

	// 모든 만료된 캐시가 삭제되었는지 확인
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatalf("디렉토리 읽기 실패: %v", err)
	}

	if len(entries) > 0 {
		t.Fatalf("캐시 정리 실패: %d개 파일이 남아있습니다", len(entries))
	}
}

func TestCacheDirCreation(t *testing.T) {
	tmpDir := filepath.Join(t.TempDir(), "nested", "cache", "dir")
	c, err := New(tmpDir, 24)
	if err != nil {
		t.Fatalf("캐시 생성 실패: %v", err)
	}

	// 디렉토리가 생성되었는지 확인
	if _, err := os.Stat(tmpDir); os.IsNotExist(err) {
		t.Fatal("캐시 디렉토리가 생성되지 않았습니다")
	}

	// 정상적으로 데이터를 저장할 수 있는지 확인
	if err := c.Set("test", []byte("data")); err != nil {
		t.Fatalf("캐시 저장 실패: %v", err)
	}
}

func TestCacheGetNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 24)
	if err != nil {
		t.Fatalf("캐시 생성 실패: %v", err)
	}

	// 존재하지 않는 키 조회
	_, found := c.Get("nonexistent_key")
	if found {
		t.Fatal("존재하지 않는 키가 반환되었습니다")
	}
}

func TestCacheGetExpiredCount(t *testing.T) {
	tmpDir := t.TempDir()
	c, err := New(tmpDir, 0) // TTL 0시간
	if err != nil {
		t.Fatalf("캐시 생성 실패: %v", err)
	}

	// 캐시 항목 저장
	for i := 0; i < 3; i++ {
		key := "key_" + string(rune(i))
		if err := c.Set(key, []byte("value")); err != nil {
			t.Fatalf("캐시 저장 실패: %v", err)
		}
	}

	// 만료되도록 대기
	time.Sleep(100 * time.Millisecond)

	// 만료된 캐시 개수 확인
	count, err := c.GetExpiredCount()
	if err != nil {
		t.Fatalf("만료된 캐시 개수 조회 실패: %v", err)
	}

	if count != 3 {
		t.Fatalf("만료된 캐시 개수 불일치: 예상=3, 실제=%d", count)
	}
}
