package nlp

import (
	"fmt"
	"regexp"
	"strings"
)

// MatchResult 자연어 입력을 파싱한 결과
type MatchResult struct {
	OrgID string // 기관 코드
	TblID string // 통계표 ID

	// RegionName 사용자가 말한 지역 "이름"("서울"). 코드로는 여기서 변환하지 않는다.
	// 통계표마다 지역 축(class1/class2)과 코드 체계가 전혀 다르므로,
	// 코드 변환은 매칭된 통계표의 RegionAxis가 유일한 출처다.
	RegionName string

	Period  string // 수록주기: Y, M, Q, H
	Start   string // 시작 시점
	End     string // 종료 시점
	Latest  int    // 최근 N개
	Periods string // 비연속 시점 (쉼표 구분)
	Output  string // 출력 파일 경로
	Keyword string // 검색에 사용할 남은 키워드
	Matched bool   // 바로가기 사전에서 매칭 여부

	// MatchedToken 바로가기를 발동시킨 원래 단어("GDP"). 재시도 안내 문구에 쓴다.
	MatchedToken string

	// Shortcut 매칭된 통계표 정보 (Matched일 때만 유효)
	Shortcut TableShortcut
}

// Regions 지역명 인식용 사전. 값(코드)은 "행정구역별" 분류를 쓰는 통계표
// (인구·주민등록인구)에만 유효하며, 다른 통계표는 각자의 RegionAxis.Codes를 쓴다.
// 새 통계표를 추가할 때 이 값을 그대로 가져다 쓰지 말 것 — meta로 실측할 것.
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

// cpiRegions 소비자물가지수(101 DT_1J22003) 시도별 코드. 실측 2026-07-20.
var cpiRegions = map[string]string{
	"전국": "T10", "서울": "T11", "부산": "T12", "대구": "T13", "인천": "T14",
	"광주": "T15", "대전": "T16", "울산": "T17", "세종": "T18", "경기": "T21",
	"강원": "T31", "충북": "T41", "충남": "T51", "전북": "T61", "전남": "T71",
	"경북": "T81", "경남": "T90", "제주": "T96",
}

// unsoldRegions 미분양주택현황(116 DT_MLTM_2086) "구분" 축 코드. 실측 2026-07-20.
// 표준 시도 순서와 다르다 — 울산 다음이 경기(0013), 그 다음이 세종(0014)이다.
var unsoldRegions = map[string]string{
	"전국": "13102871014B.0005", "서울": "13102871014B.0006", "부산": "13102871014B.0007",
	"대구": "13102871014B.0008", "인천": "13102871014B.0009", "광주": "13102871014B.0010",
	"대전": "13102871014B.0011", "울산": "13102871014B.0012", "경기": "13102871014B.0013",
	"세종": "13102871014B.0014", "강원": "13102871014B.0015", "충북": "13102871014B.0016",
	"충남": "13102871014B.0017", "전북": "13102871014B.0018", "전남": "13102871014B.0019",
	"경북": "13102871014B.0020", "경남": "13102871014B.0021", "제주": "13102871014B.0022",
}

// RegionAxis 이 통계표에서 "지역"이 어느 분류 축에 있고 코드가 무엇인지.
type RegionAxis struct {
	Flag  string            // "--class1" | "--class2" ... 지역이 놓이는 축
	Codes map[string]string // 지역명 → 이 통계표 전용 코드
}

// TableShortcut 통계표 하나를 조회하는 데 필요한 정보 일체.
// 여기 없는 값을 기본값으로 지어내지 않는다 — 조용한 오답보다 명시적 실패.
type TableShortcut struct {
	OrgID string
	TblID string

	// Fixed 항상 붙는 고정 축 (플래그명 → 값). 지역 축과 겹치면 안 된다.
	Fixed map[string]string

	// Region nil이면 이 통계표에는 지역 분류가 없다 (GDP, 실업률 등).
	Region *RegionAxis

	Item string // 항목 코드. 통계표에 항목이 하나뿐이면 그 값.

	// Periods 지원 주기. Periods[0]이 기본값이고, 여기 없는 주기는 거부한다.
	Periods []string

	Label    string // 사람이 읽는 통계표 이름 (실패 메시지·AI 프롬프트용)
	Verified string // 실측 검증일 (YYYY-MM-DD)
}

// Shortcuts 바로가기 사전 (주요 통계표).
// 모든 코드는 kosis meta로 실측하고 실제 데이터 반환까지 확인한 값이다.
// 수정 시 반드시 `make verify-shortcuts`로 재검증할 것.
var Shortcuts = map[string]TableShortcut{
	"미분양":   unsoldHousing,
	"미분양주택": unsoldHousing,

	"물가":      consumerPrice,
	"소비자물가":   consumerPrice,
	"소비자물가지수": consumerPrice,

	"GDP":   gdp,
	"gdp":   gdp,
	"국내총생산": gdp,

	"인구":  population,
	"총인구": population,

	"경제활동": economicActivity,
	"고용률":  economicActivity,
	"실업률":  economicActivity,

	"주민등록":   residentPopulation,
	"주민등록인구": residentPopulation,
}

var unsoldHousing = TableShortcut{
	OrgID: "116", TblID: "DT_MLTM_2086",
	Fixed:    map[string]string{"--class1": "13102871014A.0002"}, // 대분류=시도별미분양현황
	Region:   &RegionAxis{Flag: "--class2", Codes: unsoldRegions},
	Item:     "13103871014T1", // 미분양(12월기준) — 유일한 항목
	Periods:  []string{"Y"},   // 연간만 제공
	Label:    "미분양주택현황",
	Verified: "2026-07-20",
}

var consumerPrice = TableShortcut{
	OrgID: "101", TblID: "DT_1J22003",
	Region:   &RegionAxis{Flag: "--class1", Codes: cpiRegions},
	Item:     "T", // 소비자물가지수(총지수) — 유일한 항목
	Periods:  []string{"M", "Q", "Y"},
	Label:    "소비자물가지수(2020=100)",
	Verified: "2026-07-20",
}

var gdp = TableShortcut{
	OrgID: "301", TblID: "DT_200Y001",
	Fixed:    map[string]string{"--class1": "13102134474ACC_ITEM.10101"}, // 계정항목=국내총생산(명목)
	Region:   nil,                                                        // 계정항목별 분류라 지역 개념이 없다
	Item:     "13103134474999",                                           // 주요지표(연간지표) — 유일한 항목
	Periods:  []string{"Y"},
	Label:    "국민계정 주요지표(명목 GDP)",
	Verified: "2026-07-20",
}

var population = TableShortcut{
	OrgID: "101", TblID: "DT_1IN1502",
	Region:   &RegionAxis{Flag: "--class1", Codes: Regions}, // 행정구역별: 표준 코드 체계
	Item:     "T100",                                        // 총인구
	Periods:  []string{"Y"},
	Label:    "인구총조사 총인구",
	Verified: "2026-07-20",
}

var economicActivity = TableShortcut{
	OrgID: "101", TblID: "DT_1DA7002S",
	Fixed:    map[string]string{"--class1": "00"}, // 연령계층별=15세 이상 전체 (지역 아님!)
	Region:   nil,                                 // 분류가 연령계층별이라 지역 조회 불가
	Item:     "ALL",
	Periods:  []string{"M", "Q", "Y"},
	Label:    "경제활동인구조사(연령계층별)",
	Verified: "2026-07-20",
}

var residentPopulation = TableShortcut{
	OrgID: "101", TblID: "DT_1YL20651E",
	Region:   &RegionAxis{Flag: "--class1", Codes: Regions}, // 행정구역별: 표준 코드 체계
	Item:     "ALL",
	Periods:  []string{"Y"},
	Label:    "주민등록인구현황",
	Verified: "2026-07-20",
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

	// [1] 지역 추출 (첫 번째 매칭된 토큰).
	// 코드로 변환하지 않고 이름만 기록한다 — 어느 통계표가 매칭될지 아직 모르고,
	// 코드 체계는 통계표마다 다르기 때문이다.
	for i, token := range tokens {
		if _, exists := Regions[token]; exists {
			result.RegionName = token
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
			result.Shortcut = shortcut
			result.Matched = true
			foundShortcut = true
			matchedToken = token
			result.MatchedToken = token
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

// LookupTable org/tbl로 사전 항목을 찾습니다. AI가 생성한 명령 검증에 씁니다.
func LookupTable(orgID, tblID string) (TableShortcut, bool) {
	for _, sc := range Shortcuts {
		if sc.OrgID == orgID && sc.TblID == tblID {
			return sc, true
		}
	}
	return TableShortcut{}, false
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
