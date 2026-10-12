package api

import (
	"encoding/json"
	"fmt"
)

// List retrieves statistics list for tree navigation.
func (c *Client) List(opts ListOptions) ([]StatList, error) {
	params := map[string]string{}

	// Add optional parameters
	if opts.VwCd != "" {
		params["vwCd"] = opts.VwCd
	}
	// parentListId 는 기관별(MT_OTITLE)만 무시된다 — 주제별·북한·국제 등은 하위 목록으로 내려간다
	// (2026-10-13 실측: MT_BUKHAN --parent 101_001 → 영토/인구·보건 …).
	if opts.ParentID != "" {
		if opts.VwCd != "MT_OTITLE" {
			params["parentListId"] = opts.ParentID
		}
	}

	body, err := c.request("statisticsList.do?method=getList", params, false)
	if err != nil {
		return nil, err
	}

	var results []StatList
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}
