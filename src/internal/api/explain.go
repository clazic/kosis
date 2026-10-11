package api

import (
	"encoding/json"
	"fmt"
	"net/url"
)

// Explain 통계표가 속한 통계조사의 설명(작성목적·조사대상·공표범위·승인번호 등)을 가져온다.
// 필드 이름은 API 그대로(statsNm, writingPurps, examinObjrange, confmNo …) — 조사마다 있는 필드가 다르다.
// method=getList·metaItm=All 이 없으면 API 가 설명 전체를 etcRefData 하나에 뭉쳐 돌려준다.
func (c *Client) Explain(orgID, tblID string) (map[string]string, error) {
	if orgID == "" || tblID == "" {
		return nil, fmt.Errorf("orgId와 tblId는 필수입니다")
	}
	body, err := c.request("statisticsExplData.do", map[string]string{
		"method": "getList", "orgId": orgID, "tblId": tblID, "metaItm": "All",
	}, false)
	if err != nil {
		return nil, err
	}
	var results []map[string]any
	if err := json.Unmarshal(body, &results); err != nil {
		return nil, fmt.Errorf("응답 파싱 실패: %w", err)
	}
	out := map[string]string{}
	for _, r := range results {
		for k, v := range r {
			if s := fmt.Sprint(v); v != nil && s != "" {
				out[k] = s
			}
		}
	}
	return out, nil
}

// TableURL KOSIS 원표 페이지.
func TableURL(orgID, tblID string) string {
	return "https://kosis.kr/statHtml/statHtml.do?orgId=" + url.QueryEscape(orgID) + "&tblId=" + url.QueryEscape(tblID)
}

// ExplainURL 국가통계 승인번호(confmNo)의 통계설명자료 페이지 (나라통계).
func ExplainURL(confmNo string) string {
	if confmNo == "" {
		return ""
	}
	return "https://www.narastat.kr/metasvc/index.do?confmNo=" + url.QueryEscape(confmNo)
}
