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
	// parentListId는 MT_ZTITLE(주제별)에서만 의미가 있음.
	// 다른 뷰(MT_OTITLE 등)에서는 parentListId를 보내면 에러 30 발생.
	if opts.ParentID != "" {
		if opts.VwCd == "" || opts.VwCd == "MT_ZTITLE" {
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
