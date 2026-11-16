package api

import (
	"net/url"
	"strings"
)

// maxRequestURLLen 데이터 요청 URL(https://kosis.kr/… 전체, apiKey 포함) 길이 상한 — 안전 여유 500.
// KOSIS 는 URL 이 4,000바이트를 넘으면 오류 [21] "호출하신 URL 길이가 4000Byte를 넘어서 요청하실 수 없습니다" 를 HTTP 200 으로,
// 약 7,950자부터는 앞단 서버가 413, 약 16,400자부터 414 를 돌려준다
// (2026-11-16 실측, 101/DT_1IN1502 objL1 코드 나열: 3,994자 정상 · 4,005자 [21] · 7,916자 [21] · 7,960자 413 · 16,454자 414).
// 4만 셀 분류 축 2차 분할이 읍면동 코드 수천 개를 + 로 나열하면 이 한도를 넘었다 (-c1 ALL 11년 → 414).
var maxRequestURLLen = 3500

const dataEndpoint = "Param/statisticsParameterData.do?method=getList"

// codeFields 코드를 + 로 나열할 수 있는 데이터 요청 인자 (objL1~8, itmId).
func codeFields(o *DataOptions) []*string {
	return []*string{&o.Class1, &o.Class2, &o.Class3, &o.Class4, &o.Class5, &o.Class6, &o.Class7, &o.Class8, &o.Item}
}

// maxEncodedKeyLen 이 클라이언트 키 중 URL 인코딩했을 때 가장 긴 길이 (URL 길이 계산용).
func (c *Client) maxEncodedKeyLen() int {
	n := 0
	for _, k := range c.apiKeys {
		if l := len(url.QueryEscape(k)); l > n {
			n = l
		}
	}
	return n
}

// dataURLLen 이 조회의 데이터 요청 URL 길이 (가장 긴 키 기준 — 어떤 키로 보내도 이 이하).
func (c *Client) dataURLLen(orgID, tblID string, opts DataOptions) int {
	params := buildDataParams(orgID, tblID, opts)
	v := url.Values{}
	for k, val := range params {
		v.Set(k, val)
	}
	v.Set("apiKey", strings.Repeat("x", c.maxEncodedKeyLen()))
	v.Set("format", defaultFormat)
	v.Set("jsonVD", defaultJSONVD)
	v.Set("jsonMVD", defaultJSONMVD)
	return len(c.baseURL) + 1 + len(dataEndpoint) + 1 + len(v.Encode())
}

// splitForURL URL 이 maxRequestURLLen 을 넘으면 + 로 나열한 코드가 가장 긴 인자를 여러 묶음으로 나눈다
// (묶음마다 다시 검사하므로 여러 인자가 길어도 모두 한도 안). 순서는 원래 코드 순서. 나눌 수 없으면(코드 하나가 너무 길면) 그대로.
func (c *Client) splitForURL(orgID, tblID string, opts DataOptions) []DataOptions {
	total := c.dataURLLen(orgID, tblID, opts)
	if total <= maxRequestURLLen {
		return []DataOptions{opts}
	}
	o := opts
	fields := codeFields(&o)
	best, bestLen := -1, 0
	for i, f := range fields {
		if strings.Contains(*f, "+") {
			if l := len(url.QueryEscape(*f)); l > bestLen {
				best, bestLen = i, l
			}
		}
	}
	if best < 0 {
		return []DataOptions{opts}
	}
	codes := strings.Split(*fields[best], "+")
	budget := maxRequestURLLen - (total - bestLen) // 이 인자 값에 쓸 수 있는 길이
	longest := 0
	for _, code := range codes {
		longest = max(longest, len(url.QueryEscape(code)))
	}
	var groups [][]string
	if budget < longest { // 다른 인자도 길어 이 인자만으로는 못 맞춘다 — 반으로 나누고 묶음마다 다시 (다음엔 다른 인자를 나눈다)
		groups = [][]string{codes[:len(codes)/2], codes[len(codes)/2:]}
		budget = -1
	}
	var cur []string
	curLen := 0
	for _, code := range codes {
		if budget < 0 {
			break
		}
		l := len(url.QueryEscape(code))
		if len(cur) > 0 {
			l += len(url.QueryEscape("+"))
		}
		if len(cur) > 0 && curLen+l > budget {
			groups = append(groups, cur)
			cur, curLen = nil, 0
			l = len(url.QueryEscape(code))
		}
		cur = append(cur, code)
		curLen += l
	}
	if len(cur) > 0 {
		groups = append(groups, cur)
	}
	if len(groups) <= 1 {
		return []DataOptions{opts}
	}
	var out []DataOptions
	for _, g := range groups {
		so := opts
		*codeFields(&so)[best] = strings.Join(g, "+")
		out = append(out, c.splitForURL(orgID, tblID, so)...)
	}
	return out
}

// splitAllForURL 여러 조회 조건을 각각 URL 한도 안으로 나눠 이어 붙인다.
func (c *Client) splitAllForURL(orgID, tblID string, subs []DataOptions) []DataOptions {
	var out []DataOptions
	for _, so := range subs {
		out = append(out, c.splitForURL(orgID, tblID, so)...)
	}
	return out
}

// dataBounded Data 와 같되 URL 이 길면 나눠 부르고 이어 붙인다 (원래 코드 순서).
func (c *Client) dataBounded(orgID, tblID string, opts DataOptions) ([]DataRow, error) {
	subs := c.splitForURL(orgID, tblID, opts)
	if len(subs) == 1 {
		return c.Data(orgID, tblID, opts)
	}
	var all []DataRow
	var noData error
	for _, so := range subs {
		rows, err := c.Data(orgID, tblID, so)
		if err != nil && strings.Contains(err.Error(), "API 오류 [30]") { // 이 묶음의 코드엔 값이 없다 — 다른 묶음은 있을 수 있다
			noData = err
			continue
		}
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	if len(all) == 0 && noData != nil {
		return nil, noData // 모든 묶음이 [30]
	}
	return deduplicateRows(all), nil
}
