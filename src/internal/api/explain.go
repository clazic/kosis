package api

import (
	"encoding/json"
	"fmt"
)

// Explain retrieves explanation data for a statistics table.
// Provides survey methodology, purpose, and other metadata.
func (c *Client) Explain(orgID, tblID string) ([]ExplainResult, error) {
	if orgID == "" || tblID == "" {
		return nil, fmt.Errorf("orgId와 tblId는 필수입니다")
	}

	params := map[string]string{
		"orgId": orgID,
		"tblId": tblID,
	}

	body, err := c.request("statisticsExplData.do", params, false)
	if err != nil {
		return nil, err
	}

	var results []ExplainResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}
