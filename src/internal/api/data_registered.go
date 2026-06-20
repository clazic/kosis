package api

import (
	"encoding/json"
	"fmt"
)

// DataRegistered retrieves data from user-registered statistics.
// Uses statisticsData.do?method=getList with userStatsId parameter.
func (c *Client) DataRegistered(userStatsID string, opts DataOptions) ([]DataRow, error) {
	if userStatsID == "" {
		return nil, fmt.Errorf("userStatsId는 필수입니다")
	}

	params := map[string]string{
		"userStatsId": userStatsID,
	}

	// Add period parameters
	if opts.StartPrdDe != "" {
		params["startPrdDe"] = opts.StartPrdDe
	}
	if opts.EndPrdDe != "" {
		params["endPrdDe"] = opts.EndPrdDe
	}
	if opts.NewEstPrdCnt != "" {
		params["newEstPrdCnt"] = opts.NewEstPrdCnt
	}
	if opts.PrdInterval != "" {
		params["prdInterval"] = opts.PrdInterval
	}
	if opts.PrdSe != "" {
		params["prdSe"] = normalizePrdSe(opts.PrdSe)
	}
	if opts.OutputFields != "" {
		params["outputFields"] = opts.OutputFields
	}

	body, err := c.request("statisticsData.do?method=getList", params, true)
	if err != nil {
		return nil, err
	}

	var results []DataRow
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}
