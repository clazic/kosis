package api

import (
	"encoding/json"
	"fmt"
)

// IndicatorSearchByID searches indicators by indicator ID.
// Uses pkNumberService.do?method=getList&service=1&serviceDetail=pkNotion
func (c *Client) IndicatorSearchByID(jipyoID string, opts PaginationOptions) ([]IndicatorResult, error) {
	if jipyoID == "" {
		return nil, fmt.Errorf("지표 ID는 필수입니다")
	}

	params := map[string]string{
		"jipyoId":       jipyoID,
		"service":       "1",
		"serviceDetail": "pkNotion",
	}

	// Add pagination parameters
	if opts.PageNo > 0 {
		params["pageNo"] = fmt.Sprintf("%d", opts.PageNo)
	}
	if opts.NumOfRows > 0 {
		params["numOfRows"] = fmt.Sprintf("%d", opts.NumOfRows)
	}

	body, err := c.request("pkNumberService.do?method=getList", params, false)
	if err != nil {
		return nil, err
	}

	var results []IndicatorResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}

// IndicatorSearchByName searches indicators by indicator name.
// Uses indExpService.do?method=getList&service=2&serviceDetail=indNotion
func (c *Client) IndicatorSearchByName(jipyoNm string, opts PaginationOptions) ([]IndicatorResult, error) {
	if jipyoNm == "" {
		return nil, fmt.Errorf("지표명은 필수입니다")
	}

	params := map[string]string{
		"jipyoNm":       jipyoNm,
		"service":       "2",
		"serviceDetail": "indNotion",
	}

	// Add pagination parameters
	if opts.PageNo > 0 {
		params["pageNo"] = fmt.Sprintf("%d", opts.PageNo)
	}
	if opts.NumOfRows > 0 {
		params["numOfRows"] = fmt.Sprintf("%d", opts.NumOfRows)
	}

	body, err := c.request("indExpService.do?method=getList", params, false)
	if err != nil {
		return nil, err
	}

	var results []IndicatorResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}

// IndicatorList retrieves indicators from an indicator list.
// Uses indiListService.do?method=getList&service=3
func (c *Client) IndicatorList(listID string, opts PaginationOptions) ([]IndicatorResult, error) {
	// API might handle empty listID as root request.

	params := map[string]string{
		"listId":  listID,
		"service": "3",
	}

	// Add pagination parameters
	if opts.PageNo > 0 {
		params["pageNo"] = fmt.Sprintf("%d", opts.PageNo)
	}
	if opts.NumOfRows > 0 {
		params["numOfRows"] = fmt.Sprintf("%d", opts.NumOfRows)
	}

	body, err := c.request("indiListService.do?method=getList", params, false)
	if err != nil {
		return nil, err
	}

	var results []IndicatorResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}

// IndicatorData retrieves indicator data by indicator name.
// Uses indListSearchRequest.do?method=getList&service=4&serviceDetail=indList
func (c *Client) IndicatorData(jipyoNm string, opts PaginationOptions) ([]IndicatorResult, error) {
	if jipyoNm == "" {
		return nil, fmt.Errorf("지표명은 필수입니다")
	}

	params := map[string]string{
		"jipyoNm":       jipyoNm,
		"service":       "4",
		"serviceDetail": "indList",
	}

	// Add pagination parameters
	if opts.PageNo > 0 {
		params["pageNo"] = fmt.Sprintf("%d", opts.PageNo)
	}
	if opts.NumOfRows > 0 {
		params["numOfRows"] = fmt.Sprintf("%d", opts.NumOfRows)
	}

	body, err := c.request("indListSearchRequest.do?method=getList", params, false)
	if err != nil {
		return nil, err
	}

	var results []IndicatorResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}
