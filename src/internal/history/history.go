// Package history는 KOSIS CLI의 조회 이력 관리를 담당합니다.
// 이력 데이터는 ~/.kosis/history.yaml에 저장됩니다.
// 최대 100개 이력을 유지하며, 초과 시 가장 오래된 것부터 삭제됩니다.
package history

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/clazic/kosis/internal/config"
	"gopkg.in/yaml.v3"
)

const maxHistoryEntries = 100

// HistoryEntry는 저장된 조회 이력 항목을 나타냅니다.
type HistoryEntry struct {
	ID          int    `yaml:"id"`
	Command     string `yaml:"command"`
	Timestamp   string `yaml:"timestamp"`
	ResultCount int    `yaml:"result_count"`
}

// History는 이력 항목들의 전체 목록을 나타냅니다.
type History struct {
	Items []HistoryEntry `yaml:"history"`
}

// HistoryFilePath는 이력 파일의 경로를 반환합니다.
func HistoryFilePath() string {
	return filepath.Join(config.ConfigDir(), "history.yaml")
}

// Load는 이력 목록을 파일에서 로드합니다.
func Load() ([]HistoryEntry, error) {
	filePath := HistoryFilePath()

	// 파일이 없으면 빈 목록 반환
	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		return []HistoryEntry{}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("이력 파일 읽기 실패: %w", err)
	}

	if len(data) == 0 {
		return []HistoryEntry{}, nil
	}

	var hist History
	if err := yaml.Unmarshal(data, &hist); err != nil {
		return nil, fmt.Errorf("이력 파싱 실패: %w", err)
	}

	return hist.Items, nil
}

// Save는 이력 목록을 파일에 저장합니다.
func Save(entries []HistoryEntry) error {
	if err := config.EnsureConfigDir(); err != nil {
		return err
	}

	hist := History{Items: entries}

	data, err := yaml.Marshal(&hist)
	if err != nil {
		return fmt.Errorf("이력 인코딩 실패: %w", err)
	}

	filePath := HistoryFilePath()
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("이력 파일 저장 실패: %w", err)
	}

	return nil
}

// Add는 새로운 조회 이력을 추가합니다.
// ID는 자동으로 할당되고, Timestamp는 현재 시각으로 설정됩니다.
// 이력 수가 maxHistoryEntries(100)을 초과하면 가장 오래된 항목을 삭제합니다.
func Add(command string, resultCount int) error {
	if command == "" {
		return errors.New("명령어는 비워둘 수 없습니다")
	}

	entries, err := Load()
	if err != nil {
		return err
	}

	// 새 ID 계산 (가장 큰 ID + 1)
	newID := 1
	if len(entries) > 0 {
		newID = entries[len(entries)-1].ID + 1
	}

	newEntry := HistoryEntry{
		ID:          newID,
		Command:     command,
		Timestamp:   time.Now().Format("2006-01-02T15:04:05"),
		ResultCount: resultCount,
	}

	entries = append(entries, newEntry)

	// 최대 개수 초과 시 가장 오래된 것 삭제
	if len(entries) > maxHistoryEntries {
		entries = entries[len(entries)-maxHistoryEntries:]
	}

	return Save(entries)
}

// List는 최근 limit개의 이력을 반환합니다.
// limit이 0 이하면 모든 이력을 반환합니다.
func List(limit int) ([]HistoryEntry, error) {
	entries, err := Load()
	if err != nil {
		return nil, err
	}

	if limit <= 0 {
		return entries, nil
	}

	if len(entries) <= limit {
		return entries, nil
	}

	// 가장 최근 limit개 반환
	return entries[len(entries)-limit:], nil
}

// GetByID는 주어진 ID의 이력 항목을 반환합니다.
// 항목을 찾지 못하면 nil과 에러를 반환합니다.
func GetByID(id int) (*HistoryEntry, error) {
	entries, err := Load()
	if err != nil {
		return nil, err
	}

	for _, entry := range entries {
		if entry.ID == id {
			return &entry, nil
		}
	}

	return nil, fmt.Errorf("이력을 찾을 수 없습니다: ID %d", id)
}

// Clear는 모든 이력을 삭제합니다.
func Clear() error {
	return Save([]HistoryEntry{})
}
