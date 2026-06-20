package cache

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Cache는 파일 기반 캐시를 관리합니다.
type Cache struct {
	dir   string        // 캐시 저장 디렉토리 (예: ~/.kosis/cache/)
	ttl   time.Duration // 캐시 유효 기간
	mu    sync.RWMutex  // 동시성 보호
}

// New는 새로운 Cache 인스턴스를 생성합니다.
// dir: 캐시 디렉토리 경로
// ttlHours: 캐시 유효 기간 (시간)
func New(dir string, ttlHours int) (*Cache, error) {
	// 캐시 디렉토리 생성
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("캐시 디렉토리 생성 실패 (%s): %w", dir, err)
	}

	return &Cache{
		dir: dir,
		ttl: time.Duration(ttlHours) * time.Hour,
	}, nil
}

// cacheKeyToFilePath는 캐시 키를 파일 경로로 변환합니다.
// 파일명은 key의 SHA256 해시 + ".json"
func (c *Cache) cacheKeyToFilePath(key string) string {
	hash := sha256.Sum256([]byte(key))
	hashStr := fmt.Sprintf("%x", hash)
	return filepath.Join(c.dir, hashStr+".json")
}

// Get은 캐시에서 데이터를 조회합니다.
// 만료되었으면 false를 반환하고, 자동으로 만료된 파일을 삭제합니다.
func (c *Cache) Get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	filePath := c.cacheKeyToFilePath(key)

	// 파일 존재 여부 확인
	info, err := os.Stat(filePath)
	if err != nil {
		return nil, false
	}

	// 파일 수정 시간 확인 (ModTime으로 만료 판단)
	modTime := info.ModTime()
	if time.Since(modTime) > c.ttl {
		// 만료된 캐시는 동기적으로 삭제 (에러는 무시)
		os.Remove(filePath)
		return nil, false
	}

	// 파일 내용 읽기
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, false
	}

	return data, true
}

// Set은 데이터를 캐시에 저장합니다.
func (c *Cache) Set(key string, data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	filePath := c.cacheKeyToFilePath(key)

	// 임시 파일에 먼저 쓰기 (원자성)
	tmpFile := filePath + ".tmp"
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return fmt.Errorf("캐시 파일 쓰기 실패 (%s): %w", tmpFile, err)
	}

	// 임시 파일을 최종 파일로 이동
	if err := os.Rename(tmpFile, filePath); err != nil {
		os.Remove(tmpFile) // 실패 시 임시 파일 삭제
		return fmt.Errorf("캐시 파일 이동 실패 (%s -> %s): %w", tmpFile, filePath, err)
	}

	return nil
}

// Clear는 모든 캐시를 삭제합니다.
func (c *Cache) Clear() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 캐시 디렉토리의 모든 파일 제거
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("캐시 디렉토리 읽기 실패: %w", err)
	}

	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			filePath := filepath.Join(c.dir, entry.Name())
			if err := os.Remove(filePath); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("캐시 삭제 중 오류 발생: %v", errs[0])
	}

	return nil
}

// Size는 캐시가 차지하는 바이트 크기를 반환합니다.
func (c *Cache) Size() (int64, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var totalSize int64

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return 0, fmt.Errorf("캐시 디렉토리 읽기 실패: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			totalSize += info.Size()
		}
	}

	return totalSize, nil
}

// GetExpiredCount는 만료된 캐시 파일의 개수를 반환합니다.
func (c *Cache) GetExpiredCount() (int, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var expiredCount int

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return 0, fmt.Errorf("캐시 디렉토리 읽기 실패: %w", err)
	}

	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) > c.ttl {
				expiredCount++
			}
		}
	}

	return expiredCount, nil
}

// CleanExpired는 만료된 캐시 파일을 모두 삭제합니다.
func (c *Cache) CleanExpired() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return fmt.Errorf("캐시 디렉토리 읽기 실패: %w", err)
	}

	now := time.Now()
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			if now.Sub(info.ModTime()) > c.ttl {
				filePath := filepath.Join(c.dir, entry.Name())
				if err := os.Remove(filePath); err != nil {
					errs = append(errs, err)
				}
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("캐시 정리 중 오류 발생: %v", errs[0])
	}

	return nil
}
