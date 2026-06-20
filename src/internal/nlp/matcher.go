package nlp

import (
	"fmt"
	"regexp"
	"strings"
)

// MatchResult 자연어 입력을 파싱한 결과
type MatchResult struct {
	OrgID   string // 기관 코드
	TblID   string // 통계표 ID
	Class1  string // 분류1 (지역) 코드
	Item    string // 항목 코드
	Period  string // 수록주기: Y, M, Q, H
	Start   string // 시작 시점
	End     string // 종료 시점
	Latest  int    // 최근 N개
	Periods string // 비연속 시점 (쉼표 구분)
	Output  string // 출력 파일 경로
	Keyword string // 검색에 사용할 남은 키워드
	Matched bool   // 바로가기 사전에서 매칭 여부
}

// Regions 내장 지역 사전 (17개 시도)
var Regions = map[string]string{
	"전국": "00",
	"서울": "11",
	"부산": "21",
	"대구": "22",
	"인천": "23",
	"광주": "24",
	"대전": "25",
	"울산": "26",
	"세종": "29",
	"경기": "31",
	"강원": "32",
	"충북": "33",
	"충남": "34",
	"전북": "35",
	"전남": "36",
	"경북": "37",
	"경남": "38",
	"제주": "39",
}

// TableShortcut 바로가기 사전 항목
type TableShortcut struct {
	OrgID       string
	TblID       string
	DefaultItem string
}

// Shortcuts 바로가기 사전 (주요 통계표)
var Shortcuts = map[string]TableShortcut{
	"미분양":     {OrgID: "116", TblID: "DT_MLTM_2086", DefaultItem: "T10"},
	"미분양주택":   {OrgID: "116", TblID: "DT_MLTM_2086", DefaultItem: "T10"},
	"물가":      {OrgID: "101", TblID: "DT_1J20001", DefaultItem: "T10"},
	"소비자물가":   {OrgID: "101", TblID: "DT_1J20001", DefaultItem: "T10"},
	"소비자물가지수": {OrgID: "101", TblID: "DT_1J20001", DefaultItem: "T10"},
	"GDP":     {OrgID: "301", TblID: "DT_200Y001", DefaultItem: "T01"},
	"gdp":     {OrgID: "301", TblID: "DT_200Y001", DefaultItem: "T01"},
	"국내총생산":   {OrgID: "301", TblID: "DT_200Y001", DefaultItem: "T01"},
	"인구":      {OrgID: "101", TblID: "DT_1IN1502", DefaultItem: "T100"},
	"총인구":     {OrgID: "101", TblID: "DT_1IN1502", DefaultItem: "T100"},
	"경제활동":    {OrgID: "101", TblID: "DT_1DA7002S", DefaultItem: "ALL"},
	"고용률":     {OrgID: "101", TblID: "DT_1DA7002S", DefaultItem: "ALL"},
	"실업률":     {OrgID: "101", TblID: "DT_1DA7002S", DefaultItem: "ALL"},
	"주민등록":    {OrgID: "101", TblID: "DT_1YL20651E", DefaultItem: "ALL"},
	"주민등록인구":  {OrgID: "101", TblID: "DT_1YL20651E", DefaultItem: "ALL"},
}

// Match 자연어 입력을 파싱하여 MatchResult 반환
func Match(input string) *MatchResult {
	if input == "" {
		return &MatchResult{Matched: false}
	}

	result := &MatchResult{
		Latest:  0, // 초기값 0 (나중에 기본값 설정)
		Matched: false,
	}

	// "2020, 2022, 2025" 처럼 공백이 섞인 비연속 시점을 우선 정규화
	if normalizedPeriods, ok := extractSpacedCommaPeriods(input); ok {
		result.Periods = normalizedPeriods
		result.Period = inferPeriodFromPeriods(normalizedPeriods)
	}

	// 토큰 분리
	tokens := strings.Fields(input)
	if len(tokens) == 0 {
		return result
	}

	// 처리된 토큰을 기록하기 위한 인덱스 집합
	usedTokens := make(map[int]bool)

	// [1] 지역 추출 (첫 번째 매칭된 토큰)
	for i, token := range tokens {
		if code, exists := Regions[token]; exists {
			result.Class1 = code
			usedTokens[i] = true
			break
		}
	}

	// [2] 기간 패턴 추출
	extractPeriods(tokens, usedTokens, result)

	// [3] 나머지 토큰으로 바로가기 사전 매칭
	remainingTokens := []string{}
	for i, token := range tokens {
		if !usedTokens[i] {
			remainingTokens = append(remainingTokens, token)
		}
	}

	// 바로가기 사전에서 첫 번째 매칭되는 단어 찾기
	foundShortcut := false
	matchedToken := ""
	for _, token := range remainingTokens {
		if shortcut, exists := resolveShortcutToken(token); exists {
			result.OrgID = shortcut.OrgID
			result.TblID = shortcut.TblID
			result.Item = shortcut.DefaultItem
			result.Matched = true
			foundShortcut = true
			matchedToken = token
			break
		}
	}

	// 검색 fallback용 키워드 구성
	if foundShortcut {
		var leftover []string
		for _, token := range remainingTokens {
			if token != matchedToken {
				leftover = append(leftover, token)
			}
		}
		result.Keyword = strings.Join(leftover, " ")
	} else {
		result.Keyword = strings.Join(remainingTokens, " ")
	}

	return result
}

func resolveShortcutToken(token string) (TableShortcut, bool) {
	if shortcut, exists := Shortcuts[token]; exists {
		return shortcut, true
	}

	trimmed := strings.Trim(token, ".,!?\"'()[]{}")
	if shortcut, exists := Shortcuts[trimmed]; exists {
		return shortcut, true
	}

	upper := strings.ToUpper(trimmed)
	if shortcut, exists := Shortcuts[upper]; exists {
		return shortcut, true
	}

	lower := strings.ToLower(trimmed)
	if shortcut, exists := Shortcuts[lower]; exists {
		return shortcut, true
	}

	return TableShortcut{}, false
}

// extractPeriods 기간 패턴을 정규식으로 매칭
func extractPeriods(tokens []string, usedTokens map[int]bool, result *MatchResult) {
	// 모든 토큰의 조합을 확인
	for i, token := range tokens {
		if usedTokens[i] {
			continue
		}

		// "최근 N개월/년" 패턴 (연속된 토큰 "최근" + "N개월" 또는 단일 토큰)
		if matchRecentPattern(token, result) {
			usedTokens[i] = true
			return
		}

		// "최근" 토큰 + 다음 토큰 조합 매칭
		if token == "최근" && i+1 < len(tokens) && !usedTokens[i+1] {
			if matchRecentWithNext(tokens[i+1], result) {
				usedTokens[i] = true
				usedTokens[i+1] = true
				return
			}
		}

		// "YYYY~YYYY" 범위 패턴
		if matchRangePattern(token, result) {
			usedTokens[i] = true
			return
		}

		// "YYYY,YYYY,YYYY" 비연속 패턴
		if matchPeriodsPattern(token, result) {
			usedTokens[i] = true
			return
		}

		// "월별/연별/분기별" 주기 패턴
		if matchFrequencyPattern(token, result) {
			usedTokens[i] = true
		}
	}
}

// matchRecentWithNext "최근" 토큰 다음의 "N개월", "N개년" 등을 매칭
func matchRecentWithNext(nextToken string, result *MatchResult) bool {
	// 패턴: N개월, N개년, 6개월, 5년 등
	re := regexp.MustCompile(`^(\d+)개?(월|년)$`)
	matches := re.FindStringSubmatch(nextToken)
	if matches != nil && len(matches) == 3 {
		count := 0
		fmt.Sscanf(matches[1], "%d", &count)
		result.Latest = count
		unit := matches[2]
		if unit == "월" {
			result.Period = "M"
		} else if unit == "년" {
			result.Period = "Y"
		}
		return true
	}
	return false
}

// matchRecentPattern "최근 6개월", "최근 5년" 패턴 매칭
func matchRecentPattern(token string, result *MatchResult) bool {
	// 패턴: 최근 N개월/년
	recentRe := regexp.MustCompile(`^최근\s*(\d+)\s*개(월|년)$`)
	matches := recentRe.FindStringSubmatch(token)
	if matches != nil && len(matches) == 3 {
		count := 0
		fmt.Sscanf(matches[1], "%d", &count)
		result.Latest = count
		unit := matches[2]
		if unit == "월" {
			result.Period = "M"
		} else if unit == "년" {
			result.Period = "Y"
		}
		return true
	}

	// 더 유연한 패턴: "최근6개월", "최근5년" 등
	flexRe := regexp.MustCompile(`^최근(\d+)개(월|년)$`)
	matches = flexRe.FindStringSubmatch(token)
	if matches != nil && len(matches) == 3 {
		count := 0
		fmt.Sscanf(matches[1], "%d", &count)
		result.Latest = count
		unit := matches[2]
		if unit == "월" {
			result.Period = "M"
		} else if unit == "년" {
			result.Period = "Y"
		}
		return true
	}

	return false
}

// matchRangePattern "2020~2024" 형식 매칭
func matchRangePattern(token string, result *MatchResult) bool {
	// 패턴: YYYY~YYYY, YYYY-YYYY
	rangeRe := regexp.MustCompile(`^(\d{4})[~-](\d{4})$`)
	matches := rangeRe.FindStringSubmatch(token)
	if matches != nil && len(matches) == 3 {
		result.Start = matches[1]
		result.End = matches[2]
		// Period 미지정시 기본값 Y (연별)
		if result.Period == "" {
			result.Period = "Y"
		}
		return true
	}

	// YYYYMM~YYYYMM, YYYYMM-YYYYMM 형식도 지원
	rangeMonthRe := regexp.MustCompile(`^(\d{6})[~-](\d{6})$`)
	matches = rangeMonthRe.FindStringSubmatch(token)
	if matches != nil && len(matches) == 3 {
		result.Start = matches[1]
		result.End = matches[2]
		if result.Period == "" {
			result.Period = "M"
		}
		return true
	}

	return false
}

// matchPeriodsPattern "2020,2022,2025" 형식 매칭
func matchPeriodsPattern(token string, result *MatchResult) bool {
	// 패턴: YYYY,YYYY,YYYY
	periodsRe := regexp.MustCompile(`^(\d{4,6})(?:,\d{4,6})+$`)
	if periodsRe.MatchString(token) {
		result.Periods = token
		// Period 미지정시 기본값 Y
		if result.Period == "" {
			result.Period = "Y"
		}
		return true
	}
	return false
}

// extractSpacedCommaPeriods normalizes period lists like "2020, 2022, 2025".
func extractSpacedCommaPeriods(input string) (string, bool) {
	re := regexp.MustCompile(`\b(\d{4,6}(?:\s*,\s*\d{4,6})+)\b`)
	matches := re.FindStringSubmatch(input)
	if matches == nil || len(matches) < 2 {
		return "", false
	}

	parts := strings.Split(matches[1], ",")
	normalized := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		normalized = append(normalized, part)
	}

	if len(normalized) < 2 {
		return "", false
	}
	return strings.Join(normalized, ","), true
}

func inferPeriodFromPeriods(periods string) string {
	parts := strings.Split(periods, ",")
	if len(parts) == 0 {
		return "Y"
	}
	for _, part := range parts {
		if len(strings.TrimSpace(part)) == 6 {
			return "M"
		}
	}
	return "Y"
}

// matchFrequencyPattern "월별", "연별", "분기별" 등 주기 패턴
func matchFrequencyPattern(token string, result *MatchResult) bool {
	switch token {
	case "월별":
		result.Period = "M"
		return true
	case "연별", "년별", "연간":
		result.Period = "Y"
		return true
	case "분기별", "분기":
		result.Period = "Q"
		return true
	case "반기별", "반기":
		result.Period = "H"
		return true
	}
	return false
}
