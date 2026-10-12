package api

import (
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
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
	if opts.ObjID != "" {
		params["objId"] = opts.ObjID
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

	if opts.raw != nil {
		return nil, unmarshalMetaRaw(body, opts.raw)
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

// MetaRaw 메타를 API 필드 그대로 돌려준다. 유형마다 필드가 다르다
// (CMMT: CMMT_NM·CMMT_DC, SOURCE: JOSA_NM·DEPT_NM·DEPT_PHONE, ITM: OBJ_ID·ITM_ID·ITM_NM·UP_ITM_ID·UNIT_NM …).
func (c *Client) MetaRaw(orgID, tblID, metaType string) ([]map[string]any, error) {
	return c.MetaRawObj(orgID, tblID, metaType, "")
}

// MetaRawObj MetaRaw 에 objId(type=ITM 에서 분류 OBJ_ID 또는 ITEM 하나만)를 더한 것.
func (c *Client) MetaRawObj(orgID, tblID, metaType, objID string) ([]map[string]any, error) {
	var out []map[string]any
	_, err := c.Meta(orgID, tblID, MetaOptions{Type: metaType, ObjID: objID, raw: &out})
	return out, err
}

// MetaCodes 분류·항목 코드만 (type=ITM 1번 호출 — 수록정보 PRD 는 부르지 않는다).
// objID 를 주면 그 분류(OBJ_ID) 또는 항목(ITEM)만 받는다.
func (c *Client) MetaCodes(orgID, tblID, objID string) ([]CodeRow, error) {
	rows, err := c.Meta(orgID, tblID, MetaOptions{Type: "ITM", ObjID: objID})
	if err != nil {
		return nil, err
	}
	s := &MetaSummaryResult{}
	for _, r := range rows {
		if r.ObjID == "ITEM" {
			s.Items = append(s.Items, r)
		} else {
			s.Classifications = append(s.Classifications, r)
		}
	}
	return s.Codes(), nil
}

func unmarshalMetaRaw(body []byte, out *[]map[string]any) error {
	body = []byte(strings.TrimSpace(string(body)))
	if len(body) > 0 && body[0] == '{' { // {"CLASSIFICATIONS":[...], "ITEMS":[...]} → 한 목록으로
		var apiErr struct {
			Err    string `json:"err"`
			ErrMsg string `json:"errMsg"`
		}
		if json.Unmarshal(body, &apiErr) == nil && apiErr.Err != "" {
			return fmt.Errorf("API 오류 [%s]: %s", apiErr.Err, apiErr.ErrMsg)
		}
		var obj map[string][]map[string]any
		if err := json.Unmarshal(body, &obj); err != nil {
			return fmt.Errorf("응답 파싱 실패: %w", err)
		}
		for _, k := range []string{"CLASSIFICATIONS", "ITEMS", "PERIODS"} {
			*out = append(*out, obj[k]...)
		}
		return nil
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("응답 파싱 실패: %w", err)
	}
	return nil
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

// CodeRow 분류·항목 코드 한 줄 (m --find 용). Flag 는 kosis d 에 넘길 플래그.
type CodeRow struct {
	Flag       string `json:"flag"`  // --class1 … --class8, --item
	Axis       string `json:"axis"`  // 분류 이름 (시도별, 산업별 …)
	Code       string `json:"code"`  // d 에 넘길 코드
	Name       string `json:"name"`  // 이름 (공백 정리)
	Level      int    `json:"level"` // 1 = 최상위
	ParentCode string `json:"parent_code,omitempty"`
	ParentName string `json:"parent_name,omitempty"`
	Path       string `json:"path"` // 최상위부터 이름 (A > B > C)
	Unit       string `json:"unit,omitempty"`
}

// Codes 분류·항목의 모든 코드를 단계·상위 코드·경로와 함께 펼친다.
// 분류 순서(OBJ_ID 첫 등장 순)가 --class1, --class2 … 에 대응한다.
func (s *MetaSummaryResult) Codes() []CodeRow {
	var out []CodeRow
	add := func(rows []MetaResult, flagOf func(MetaResult) string) {
		type key struct{ obj, code string }
		byCode := map[key]MetaResult{}
		for _, r := range rows {
			byCode[key{r.ObjID, r.ItmID}] = r
		}
		for _, r := range rows {
			names := []string{cleanName(r.ItmNM)}
			level, up := 1, r.UpItmID
			for seen := 0; up != "" && seen < 20; seen++ { // 20: 순환 방지
				p, ok := byCode[key{r.ObjID, up}]
				if !ok {
					break
				}
				names = append([]string{cleanName(p.ItmNM)}, names...)
				level++
				up = p.UpItmID
			}
			row := CodeRow{Flag: flagOf(r), Axis: r.ObjNM, Code: r.ItmID, Name: cleanName(r.ItmNM),
				Level: level, ParentCode: r.UpItmID, Path: strings.Join(names, " > "), Unit: r.UnitNM}
			if p, ok := byCode[key{r.ObjID, r.UpItmID}]; ok {
				row.ParentName = cleanName(p.ItmNM)
			}
			out = append(out, row)
		}
	}
	// 축 번호는 메타의 OBJ_ID_SN (= objL 번호). 없으면 나온 순서.
	// 축 하나만 받은 경우(objId)에도 원래 번호를 지키려면 순서로 매기면 안 된다.
	idx := map[string]int{}
	add(s.Classifications, func(r MetaResult) string {
		if _, ok := idx[r.ObjID]; !ok {
			if n, err := strconv.Atoi(strings.TrimSpace(r.ObjIDSN)); err == nil && n > 0 {
				idx[r.ObjID] = n
			} else {
				idx[r.ObjID] = len(idx) + 1
			}
		}
		return fmt.Sprintf("--class%d", idx[r.ObjID])
	})
	add(s.Items, func(MetaResult) string { return "--item" })
	return out
}

// cleanName "울  산" 처럼 이름 안의 겹친 공백을 하나로.
func cleanName(s string) string { return strings.Join(strings.Fields(s), " ") }
