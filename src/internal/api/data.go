package api

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// buildDataParams 데이터 조회 파라미터 구성 (공통 함수)
// orgID, tblID와 DataOptions를 받아 API 요청용 파라미터 맵을 구성합니다.
func buildDataParams(orgID, tblID string, opts DataOptions) map[string]string {
	params := map[string]string{
		"orgId": orgID,
		"tblId": tblID,
	}

	// Add optional classification parameters
	if opts.Class1 != "" {
		params["objL1"] = opts.Class1
	}
	if opts.Class2 != "" {
		params["objL2"] = opts.Class2
	}
	if opts.Class3 != "" {
		params["objL3"] = opts.Class3
	}
	if opts.Class4 != "" {
		params["objL4"] = opts.Class4
	}
	if opts.Class5 != "" {
		params["objL5"] = opts.Class5
	}
	if opts.Class6 != "" {
		params["objL6"] = opts.Class6
	}
	if opts.Class7 != "" {
		params["objL7"] = opts.Class7
	}
	if opts.Class8 != "" {
		params["objL8"] = opts.Class8
	}

	// Add item and period parameters
	if opts.Item != "" {
		params["itmId"] = opts.Item
	}
	if opts.PrdSe != "" {
		params["prdSe"] = normalizePrdSe(opts.PrdSe)
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
	if opts.OutputFields != "" {
		params["outputFields"] = opts.OutputFields
	}

	return params
}

// Data retrieves data from statisticsParameterData API with optional parameters.
func (c *Client) Data(orgID, tblID string, opts DataOptions) ([]DataRow, error) {
	if orgID == "" || tblID == "" {
		return nil, fmt.Errorf("orgId와 tblId는 필수입니다")
	}

	params := buildDataParams(orgID, tblID, opts)

	body, err := c.request("Param/statisticsParameterData.do?method=getList", params, true)
	if err != nil {
		return nil, err
	}

	// jsonVD=Y 사용 시 키가 한글로 반환됨 → 영문 키로 치환
	bodyStr := normalizeDataKeys(string(body))

	var results []DataRow
	if err := json.Unmarshal([]byte(bodyStr), &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}

// normalizeDataKeys는 jsonVD=Y로 인한 한글 JSON 키를 영문 키로 치환합니다.
func normalizeDataKeys(s string) string {
	replacer := strings.NewReplacer(
		`"분류값명1"`, `"C1_NM"`,
		`"분류값명2"`, `"C2_NM"`,
		`"분류값명3"`, `"C3_NM"`,
		`"분류값명4"`, `"C4_NM"`,
		`"분류값명5"`, `"C5_NM"`,
		`"분류값명6"`, `"C6_NM"`,
		`"분류값명7"`, `"C7_NM"`,
		`"분류값명8"`, `"C8_NM"`,
		`"분류값1"`, `"C1"`,
		`"분류값2"`, `"C2"`,
		`"분류값3"`, `"C3"`,
		`"분류값4"`, `"C4"`,
		`"분류값5"`, `"C5"`,
		`"분류값6"`, `"C6"`,
		`"분류값7"`, `"C7"`,
		`"분류값8"`, `"C8"`,
		`"항목명"`, `"ITM_NM"`,
		`"항목"`, `"ITM_ID"`,
		`"단위"`, `"UNIT_NM"`,
		`"단위ID"`, `"UNIT_ID"`,
		`"수록주기"`, `"PRD_SE"`,
		`"수록시점"`, `"PRD_DE"`,
		`"수치값"`, `"DT"`,
		`"비고"`, `"CMMT"`,
		`"최종수정일"`, `"LST_CHN_DE"`,
		`"기관코드"`, `"ORG_ID"`,
		`"통계표코드"`, `"TBL_ID"`,
		`"통계표명"`, `"TBL_NM"`,
	)
	return replacer.Replace(s)
}

// normalizePrdSe converts Korean/user-friendly period codes to KOSIS API codes.
// 메타 API는 한글(년, 월, 분기, 반기, 5년 등)로 반환하지만, 데이터 API는 영문 코드를 요구합니다.
func normalizePrdSe(prdSe string) string {
	switch strings.TrimSpace(prdSe) {
	case "년", "연", "Y", "y":
		return "Y"
	case "월", "M", "m":
		return "M"
	case "분기", "Q", "q":
		return "Q"
	case "반기", "H", "h":
		return "H"
	case "5년", "F", "f":
		return "F"
	case "A":
		// 일부 통계표에서 PRD_SE가 "A"로 반환되지만, 실제 API 코드는 다를 수 있음
		return "Y"
	default:
		return prdSe
	}
}

// DataWithPeriods handles non-continuous periods by making multiple requests.
func (c *Client) DataWithPeriods(orgID, tblID string, periods []string, opts DataOptions) ([]DataRow, error) {
	if orgID == "" || tblID == "" {
		return nil, fmt.Errorf("orgId와 tblId는 필수입니다")
	}

	if len(periods) == 0 {
		return nil, fmt.Errorf("기간은 필수입니다")
	}

	var allResults []DataRow

	// Request data for each period separately
	for _, period := range periods {
		// Make a copy of options to avoid mutation
		optsCopy := opts
		optsCopy.StartPrdDe = period
		optsCopy.EndPrdDe = period

		results, err := c.Data(orgID, tblID, optsCopy)
		if err != nil {
			// API 오류 30: 해당 시점에 데이터가 없음 → 건너뛰고 계속 진행
			if strings.Contains(err.Error(), "API 오류 [30]") {
				fmt.Fprintf(os.Stderr, "참고: 시점 %s에 데이터가 없어 건너뜁니다.\n", period)
				continue
			}
			return allResults, fmt.Errorf("기간 %s 조회 실패: %w", period, err)
		}

		allResults = append(allResults, results...)
	}

	return allResults, nil
}

// ParsePeriods parses period string in various formats.
// Supports: "2020,2022,2025" (non-continuous) or returns as is for continuous ranges.
func ParsePeriods(periodStr string) []string {
	periodStr = strings.TrimSpace(periodStr)
	if periodStr == "" {
		return []string{}
	}

	// Split by comma for non-continuous periods
	parts := strings.Split(periodStr, ",")
	var periods []string
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			periods = append(periods, trimmed)
		}
	}

	return periods
}

// IsNonContinuousPeriods checks if the period string contains multiple non-continuous values.
func IsNonContinuousPeriods(periodStr string) bool {
	parts := ParsePeriods(periodStr)
	return len(parts) > 1 && strings.Contains(periodStr, ",")
}

// dataWithSpecificKey retrieves data using a specific API key.
// keyIndex >= 0이면 해당 키를 사용, -1이면 라운드로빈 사용 (Data 함수와 동일).
// 설계서 8.5절: 멀티 API 키 병렬 조회에서 각 워커가 특정 키로 요청하도록 함.
func (c *Client) dataWithSpecificKey(orgID, tblID string, opts DataOptions, keyIndex int) ([]DataRow, error) {
	if orgID == "" || tblID == "" {
		return nil, fmt.Errorf("orgId와 tblId는 필수입니다")
	}

	// buildDataParams 함수로 파라미터 구성 (코드 중복 제거)
	params := buildDataParams(orgID, tblID, opts)

	// 데이터 응답은 캐시하지 않음 (설계서 8.5절)
	body, err := c.requestWithKey("Param/statisticsParameterData.do?method=getList", params, true, keyIndex)
	if err != nil {
		return nil, err
	}

	bodyStr := normalizeDataKeys(string(body))
	var results []DataRow
	if err := json.Unmarshal([]byte(bodyStr), &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}

	return results, nil
}
