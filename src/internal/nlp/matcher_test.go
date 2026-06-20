package nlp

import (
	"testing"
)

func TestMatch(t *testing.T) {
	testCases := []struct {
		input          string
		expectedClass1 string
		expectedPeriod string
		expectedLatest int
		expectedMatched bool
		expectedOrgID  string
		expectedTblID  string
		expectedItem   string
	}{
		// [1] 지역 매칭 테스트
		{
			input:          "서울 미분양 최근 6개월",
			expectedClass1: "11",
			expectedPeriod: "M",
			expectedLatest: 6,
			expectedMatched: true,
			expectedOrgID:  "116",
			expectedTblID:  "DT_MLTM_2086",
			expectedItem:   "T10",
		},
		// [2] 기간 범위 테스트
		{
			input:          "GDP 2020~2024",
			expectedPeriod: "Y",
			expectedMatched: true,
			expectedOrgID:  "301",
			expectedTblID:  "DT_200Y001",
			expectedItem:   "T01",
		},
		// [3] 지역 없음 - 비어 있어야 함 (quick.go에서 기본값 설정)
		{
			input:          "물가 최근 3개월",
			expectedClass1: "", // 입력에 지역이 없으므로 비어있음
			expectedPeriod: "M",
			expectedLatest: 3,
			expectedMatched: true,
			expectedOrgID:  "101",
			expectedTblID:  "DT_1J20001",
			expectedItem:   "T10",
		},
		// [4] 비연속 시점
		{
			input:          "인구 2020,2022,2025",
			expectedPeriod: "Y",
			expectedMatched: true,
			expectedOrgID:  "101",
			expectedTblID:  "DT_1IN1502",
			expectedItem:   "T100",
		},
		// [5] 바로가기 없음 - 검색 필요
		{
			input:           "전국 실업자",
			expectedClass1:  "00",
			expectedMatched: false,
		},
	}

	for i, tc := range testCases {
		result := Match(tc.input)

		if result.Class1 != tc.expectedClass1 && tc.expectedClass1 != "" {
			t.Errorf("[Case %d] Class1: got %s, want %s", i+1, result.Class1, tc.expectedClass1)
		}

		if result.Period != tc.expectedPeriod && tc.expectedPeriod != "" {
			t.Errorf("[Case %d] Period: got %s, want %s", i+1, result.Period, tc.expectedPeriod)
		}

		if result.Latest != tc.expectedLatest && tc.expectedLatest != 0 {
			t.Errorf("[Case %d] Latest: got %d, want %d", i+1, result.Latest, tc.expectedLatest)
		}

		if result.Matched != tc.expectedMatched {
			t.Errorf("[Case %d] Matched: got %v, want %v", i+1, result.Matched, tc.expectedMatched)
		}

		if result.OrgID != tc.expectedOrgID && tc.expectedOrgID != "" {
			t.Errorf("[Case %d] OrgID: got %s, want %s", i+1, result.OrgID, tc.expectedOrgID)
		}

		if result.TblID != tc.expectedTblID && tc.expectedTblID != "" {
			t.Errorf("[Case %d] TblID: got %s, want %s", i+1, result.TblID, tc.expectedTblID)
		}

		if result.Item != tc.expectedItem && tc.expectedItem != "" {
			t.Errorf("[Case %d] Item: got %s, want %s", i+1, result.Item, tc.expectedItem)
		}
	}
}

func TestMatchPeriodPatterns(t *testing.T) {
	testCases := []struct {
		input          string
		expectedPeriod string
		expectedStart  string
		expectedEnd    string
		expectedLatest int
		expectedPeriods string
	}{
		{input: "최근6개월", expectedPeriod: "M", expectedLatest: 6},
		{input: "최근 5년", expectedPeriod: "Y", expectedLatest: 5},
		{input: "2020~2024", expectedPeriod: "Y", expectedStart: "2020", expectedEnd: "2024"},
		{input: "2020,2022,2025", expectedPeriod: "Y", expectedPeriods: "2020,2022,2025"},
		{input: "월별", expectedPeriod: "M"},
		{input: "연별", expectedPeriod: "Y"},
		{input: "분기별", expectedPeriod: "Q"},
	}

	for _, tc := range testCases {
		result := Match(tc.input)

		if tc.expectedPeriod != "" && result.Period != tc.expectedPeriod {
			t.Errorf("Input '%s': Period got %s, want %s", tc.input, result.Period, tc.expectedPeriod)
		}

		if tc.expectedStart != "" && result.Start != tc.expectedStart {
			t.Errorf("Input '%s': Start got %s, want %s", tc.input, result.Start, tc.expectedStart)
		}

		if tc.expectedEnd != "" && result.End != tc.expectedEnd {
			t.Errorf("Input '%s': End got %s, want %s", tc.input, result.End, tc.expectedEnd)
		}

		if tc.expectedLatest != 0 && result.Latest != tc.expectedLatest {
			t.Errorf("Input '%s': Latest got %d, want %d", tc.input, result.Latest, tc.expectedLatest)
		}

		if tc.expectedPeriods != "" && result.Periods != tc.expectedPeriods {
			t.Errorf("Input '%s': Periods got %s, want %s", tc.input, result.Periods, tc.expectedPeriods)
		}
	}
}

func TestRegionsShortcuts(t *testing.T) {
	// 지역 사전 확인
	expectedRegions := 18 // 17개 시도 + 전국
	if len(Regions) != expectedRegions {
		t.Errorf("Regions count: got %d, want %d", len(Regions), expectedRegions)
	}

	// 바로가기 사전 확인
	expectedShortcuts := 8
	if len(Shortcuts) < expectedShortcuts {
		t.Errorf("Shortcuts count: got %d, want at least %d", len(Shortcuts), expectedShortcuts)
	}

	// 특정 바로가기 확인
	if shortcut, exists := Shortcuts["미분양"]; !exists || shortcut.OrgID != "116" || shortcut.TblID != "DT_MLTM_2086" {
		t.Errorf("Shortcut '미분양' not correctly defined")
	}
}
