package api

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

// Meta retrieves metadata for a statistics table.
func (c *Client) Meta(orgID, tblID string, opts MetaOptions) ([]MetaResult, error) {
	if orgID == "" || tblID == "" {
		return nil, fmt.Errorf("orgId와 tblId는 필수입니다")
	}

	params := map[string]string{
		"orgId": orgID,
		"tblId": tblID,
	}

	// Add optional type parameter
	if opts.Type != "" {
		params["type"] = opts.Type
	}

	body, err := c.request("statisticsData.do?method=getMeta", params, false)
	if err != nil {
		return nil, err
	}

	// KOSIS API는 jsonVD=Y 없이 호출하면 키에 따옴표가 없는 비표준 JSON을 반환
	// 예: {OBJ_ID:"ITEM"} → {"OBJ_ID":"ITEM"}
	bodyStr := strings.TrimSpace(string(body))
	if len(bodyStr) > 0 {
		// 먼저 표준 JSON 파싱을 시도하고, 실패할 때만 regex 폴백 사용
		// 이렇게 하면 정상 JSON 내 문자열 값에서 regex가 잘못 매칭하는 것을 방지
		var jsonCheck json.RawMessage
		if json.Unmarshal([]byte(bodyStr), &jsonCheck) != nil {
			// 표준 JSON이 아닌 경우에만 따옴표 없는 키를 표준 JSON 키로 변환
			re := regexp.MustCompile(`([{,])\s*([A-Za-z_][A-Za-z0-9_]*)\s*:`)
			bodyStr = re.ReplaceAllString(bodyStr, `$1"$2":`)
		}
		body = []byte(bodyStr)
	}

	// 응답이 object 구조인지 array 구조인지 판별
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) > 0 && body[0] == '{' {
		// object 구조: {"CLASSIFICATIONS": [...], "ITEMS": [...], ...}
		var objResp struct {
			Classifications []MetaResult `json:"CLASSIFICATIONS"`
			Items           []MetaResult `json:"ITEMS"`
			Periods         []MetaResult `json:"PERIODS"`
		}
		if err := json.Unmarshal(body, &objResp); err != nil {
			return nil, fmt.Errorf("응답 파싱 실패: %w", err)
		}
		var results []MetaResult
		results = append(results, objResp.Classifications...)
		results = append(results, objResp.Items...)
		results = append(results, objResp.Periods...)
		return results, nil
	}

	// array 구조: [...]
	var results []MetaResult
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}

// MetaSummary retrieves and organizes metadata into classifications, items, and periods.
func (c *Client) MetaSummary(orgID, tblID string) (*MetaSummaryResult, error) {
	if orgID == "" || tblID == "" {
		return nil, fmt.Errorf("orgId와 tblId는 필수입니다")
	}

	summary := &MetaSummaryResult{
		Classifications: []MetaResult{},
		Items:           []MetaResult{},
		Periods:         []MetaResult{},
	}

	// ITM 타입: 분류(Classification)와 항목(Item) 정보
	itmResults, itmErr := c.Meta(orgID, tblID, MetaOptions{Type: "ITM"})
	if itmErr == nil {
		for _, r := range itmResults {
			if r.ObjID == "ITEM" {
				summary.Items = append(summary.Items, r)
			} else {
				summary.Classifications = append(summary.Classifications, r)
			}
		}
	}

	// PRD 타입: 수록정보 (주기, 기간)
	prdResults, prdErr := c.Meta(orgID, tblID, MetaOptions{Type: "PRD"})
	if prdErr == nil {
		summary.Periods = prdResults
	}

	// 모든 서브 쿼리가 실패하면 에러 반환
	if itmErr != nil && prdErr != nil {
		return nil, fmt.Errorf("메타 조회 실패: ITM=%v, PRD=%v", itmErr, prdErr)
	}

	// 일부만 실패하면 경고 로그 후 부분 결과 반환
	if itmErr != nil {
		fmt.Fprintf(os.Stderr, "경고: ITM 메타 조회 실패 (분류/항목 정보 누락): %v\n", itmErr)
	}
	if prdErr != nil {
		fmt.Fprintf(os.Stderr, "경고: PRD 메타 조회 실패 (수록정보 누락): %v\n", prdErr)
	}

	return summary, nil
}
