package api

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// yearPattern matches 4-digit years (1900-2099) in search keywords.
var yearPattern = regexp.MustCompile(`\b(19|20)\d{2}\b`)

// Search searches for statistics tables by keyword.
// If the search fails with error 30 and the keyword contains a year,
// it automatically retries without the year.
func (c *Client) Search(keyword string, opts SearchOptions) ([]SearchResult, error) {
	if keyword == "" {
		return nil, fmt.Errorf("검색어는 필수입니다")
	}

	results, err := c.doSearch(keyword, opts)
	if err != nil && strings.Contains(err.Error(), "API 오류 [30]") {
		// Check if keyword is purely numeric (stat code, not a keyword)
		if isNumericOnly(keyword) {
			return nil, fmt.Errorf("숫자 코드(%s)로는 검색할 수 없습니다.\n"+
				"  → 통계표 이름으로 검색하세요: kosis s \"인구\"\n"+
				"  → 코드로 찾으려면 목록 탐색: kosis ls", keyword)
		}

		// Check if keyword contains a year pattern
		if yearPattern.MatchString(keyword) {
			cleaned := strings.TrimSpace(yearPattern.ReplaceAllString(keyword, ""))
			// Remove extra spaces
			cleaned = regexp.MustCompile(`\s+`).ReplaceAllString(cleaned, " ")
			if cleaned != "" && cleaned != keyword {
				fmt.Fprintf(os.Stderr, "참고: 검색어에서 연도를 제거하고 재검색합니다 → \"%s\"\n", cleaned)
				return c.doSearch(cleaned, opts)
			}
		}
	}

	return results, err
}

func (c *Client) doSearch(keyword string, opts SearchOptions) ([]SearchResult, error) {
	params := map[string]string{
		"searchNm": keyword,
	}

	// Add optional parameters
	if opts.Sort != "" {
		params["sort"] = opts.Sort
	}
	if opts.StartCount > 0 {
		params["startCount"] = fmt.Sprintf("%d", opts.StartCount)
	}
	if opts.ResultCount > 0 {
		params["resultCount"] = fmt.Sprintf("%d", opts.ResultCount)
	} else {
		// Default result count
		params["resultCount"] = "20"
	}

	body, err := c.request("statisticsSearch.do?method=getList", params, false)
	if err != nil {
		return nil, err
	}

	var results []SearchResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}

// isNumericOnly checks if a string contains only digits.
func isNumericOnly(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
