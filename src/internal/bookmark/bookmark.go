// Package bookmark은 KOSIS CLI의 즐겨찾기 관리를 담당합니다.
// 즐겨찾기 데이터는 ~/.kosis/bookmarks.yaml에 저장됩니다.
package bookmark

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/clazic/kosis/internal/config"
	"gopkg.in/yaml.v3"
)

// Bookmark는 저장된 즐겨찾기를 나타냅니다.
type Bookmark struct {
	Name    string `yaml:"name"`
	OrgID   string `yaml:"org_id"`
	TblID   string `yaml:"tbl_id"`
	AddedAt string `yaml:"added_at"`
}

// Bookmarks는 즐겨찾기 목록 전체를 나타냅니다.
type Bookmarks struct {
	Items []Bookmark `yaml:"bookmarks"`
}

// BookmarkFilePath는 즐겨찾기 파일의 경로를 반환합니다.
func BookmarkFilePath() string {
	return filepath.Join(config.ConfigDir(), "bookmarks.yaml")
}

// Load는 즐겨찾기 목록을 파일에서 로드합니다.
func Load() ([]Bookmark, error) {
	filePath := BookmarkFilePath()

	// 파일이 없으면 빈 목록 반환
	if _, err := os.Stat(filePath); errors.Is(err, os.ErrNotExist) {
		return []Bookmark{}, nil
	}

	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("즐겨찾기 파일 읽기 실패: %w", err)
	}

	if len(data) == 0 {
		return []Bookmark{}, nil
	}

	var bm Bookmarks
	if err := yaml.Unmarshal(data, &bm); err != nil {
		return nil, fmt.Errorf("즐겨찾기 파싱 실패: %w", err)
	}

	return bm.Items, nil
}

// Save는 즐겨찾기 목록을 파일에 저장합니다.
func Save(bookmarks []Bookmark) error {
	if err := config.EnsureConfigDir(); err != nil {
		return err
	}

	bm := Bookmarks{Items: bookmarks}

	data, err := yaml.Marshal(&bm)
	if err != nil {
		return fmt.Errorf("즐겨찾기 인코딩 실패: %w", err)
	}

	filePath := BookmarkFilePath()
	if err := os.WriteFile(filePath, data, 0o600); err != nil {
		return fmt.Errorf("즐겨찾기 파일 저장 실패: %w", err)
	}

	return nil
}

// Add는 새로운 즐겨찾기를 추가합니다.
// 같은 이름이 이미 존재하면 에러를 반환합니다.
// orgID와 tblID가 비어있으면 에러를 반환합니다.
func Add(orgID, tblID, name string) error {
	if orgID == "" || tblID == "" {
		return errors.New("기관 코드(orgID)와 통계표 ID(tblID)는 필수입니다")
	}

	if name == "" {
		// name이 없으면 "orgID_tblID" 형식으로 자동 생성
		name = fmt.Sprintf("%s_%s", orgID, tblID)
	}

	bookmarks, err := Load()
	if err != nil {
		return err
	}

	// 중복 확인
	for _, bm := range bookmarks {
		if bm.Name == name {
			return fmt.Errorf("이미 존재하는 즐겨찾기: %s", name)
		}
	}

	newBookmark := Bookmark{
		Name:    name,
		OrgID:   orgID,
		TblID:   tblID,
		AddedAt: time.Now().Format("2006-01-02"),
	}

	bookmarks = append(bookmarks, newBookmark)
	return Save(bookmarks)
}

// Remove는 주어진 이름 또는 인덱스로 즐겨찾기를 제거합니다.
// nameOrIndex가 숫자면 인덱스로 취급하고,
// 문자열이면 이름으로 취급합니다.
func Remove(nameOrIndex string) error {
	bookmarks, err := Load()
	if err != nil {
		return err
	}

	if len(bookmarks) == 0 {
		return errors.New("저장된 즐겨찾기가 없습니다")
	}

	var targetIndex int
	var found bool

	// 숫자 인덱스로 시도
	if idx, err := strconv.Atoi(nameOrIndex); err == nil {
		if idx >= 0 && idx < len(bookmarks) {
			targetIndex = idx
			found = true
		}
	} else {
		// 이름으로 검색
		for i, bm := range bookmarks {
			if bm.Name == nameOrIndex {
				targetIndex = i
				found = true
				break
			}
		}
	}

	if !found {
		return fmt.Errorf("즐겨찾기를 찾을 수 없습니다: %s", nameOrIndex)
	}

	bookmarks = append(bookmarks[:targetIndex], bookmarks[targetIndex+1:]...)
	return Save(bookmarks)
}

// List는 저장된 모든 즐겨찾기를 반환합니다.
func List() ([]Bookmark, error) {
	return Load()
}
