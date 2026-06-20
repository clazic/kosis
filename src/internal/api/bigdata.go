package api

import (
	"fmt"
)

// BigData retrieves large-scale statistics data (exceeding 40,000 cells).
// Returns raw response data in SDMX or XLS format based on type parameter.
func (c *Client) BigData(userStatsID string, opts BigDataOptions) ([]byte, error) {
	if userStatsID == "" {
		return nil, fmt.Errorf("userStatsId는 필수입니다")
	}

	params := map[string]string{
		"userStatsId": userStatsID,
	}

	// Add optional type parameter (SDMX, XLS)
	if opts.Type != "" {
		params["type"] = opts.Type
	} else {
		// Default to XLS
		params["type"] = "XLS"
	}

	// Make request and return raw bytes
	body, err := c.request("statisticsBigData.do?method=getList", params, true)
	if err != nil {
		return nil, err
	}

	return body, nil
}
