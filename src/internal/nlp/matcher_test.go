package nlp

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestMatch(t *testing.T) {
	testCases := []struct {
		input           string
		expectedRegion  string
		expectedPeriod  string
		expectedLatest  int
		expectedMatched bool
		expectedOrgID   string
		expectedTblID   string
		expectedItem    string
	}{
		// [1] 지역은 코드가 아니라 이름으로 넘어와야 한다 (코드 체계는 통계표마다 다름)
		{
			input:           "서울 미분양 최근 6개월",
			expectedRegion:  "서울",
			expectedPeriod:  "M",
			expectedLatest:  6,
			expectedMatched: true,
			expectedOrgID:   "116",
			expectedTblID:   "DT_MLTM_2086",
			expectedItem:    "13103871014T1",
		},
		// [2] 기간 범위 테스트
		{
			input:           "GDP 2020~2024",
			expectedPeriod:  "Y",
			expectedMatched: true,
			expectedOrgID:   "301",
			expectedTblID:   "DT_200Y001",
			expectedItem:    "13103134474999",
		},
		// [3] 지역 없음
		{
			input:           "물가 최근 3개월",
			expectedRegion:  "", // 입력에 지역이 없음
			expectedPeriod:  "M",
			expectedLatest:  3,
			expectedMatched: true,
			expectedOrgID:   "101",
			expectedTblID:   "DT_1J22003",
			expectedItem:    "T",
		},
		// [4] 비연속 시점
		{
			input:           "인구 2020,2022,2025",
			expectedPeriod:  "Y",
			expectedMatched: true,
			expectedOrgID:   "101",
			expectedTblID:   "DT_1IN1502",
			expectedItem:    "T100",
		},
		// [5] 바로가기 없음 - 검색 필요
		{
			input:           "전국 실업자",
			expectedRegion:  "전국",
			expectedMatched: false,
		},
	}

	for i, tc := range testCases {
		result := Match(tc.input)

		if result.RegionName != tc.expectedRegion {
			t.Errorf("[Case %d] RegionName: got %s, want %s", i+1, result.RegionName, tc.expectedRegion)
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

		if result.Shortcut.Item != tc.expectedItem && tc.expectedItem != "" {
			t.Errorf("[Case %d] Item: got %s, want %s", i+1, result.Shortcut.Item, tc.expectedItem)
		}
	}
}

func TestMatchPeriodPatterns(t *testing.T) {
	testCases := []struct {
		input           string
		expectedPeriod  string
		expectedStart   string
		expectedEnd     string
		expectedLatest  int
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

// TestShortcutIntegrity 사전 항목의 구조적 결함을 잡는다.
// 코드값이 KOSIS와 맞는지는 여기서 알 수 없지만(그건 livecheck 담당),
// 축 충돌·전국코드 누락·빈 항목 같은 결함은 전부 여기서 걸린다.
func TestShortcutIntegrity(t *testing.T) {
	validPeriods := map[string]bool{"Y": true, "M": true, "Q": true, "H": true, "D": true, "F": true}
	validFlags := map[string]bool{}
	for i := 1; i <= 8; i++ {
		validFlags[fmt.Sprintf("--class%d", i)] = true
	}

	for key, sc := range Shortcuts {
		if sc.OrgID == "" || sc.TblID == "" {
			t.Errorf("[%s] OrgID/TblID가 비어 있습니다", key)
		}
		if sc.Item == "" {
			t.Errorf("[%s] Item이 비어 있습니다 — quick이 항목 없이 조회를 시도하게 됩니다", key)
		}
		if sc.Label == "" {
			t.Errorf("[%s] Label이 비어 있습니다 — 실패 메시지가 불친절해집니다", key)
		}
		if len(sc.Periods) == 0 {
			t.Errorf("[%s] Periods가 비어 있습니다", key)
		}
		for _, p := range sc.Periods {
			if !validPeriods[p] {
				t.Errorf("[%s] 알 수 없는 주기: %s", key, p)
			}
		}
		for flag := range sc.Fixed {
			if !validFlags[flag] {
				t.Errorf("[%s] Fixed에 잘못된 플래그: %s", key, flag)
			}
		}
		if sc.Region != nil {
			if !validFlags[sc.Region.Flag] {
				t.Errorf("[%s] Region.Flag가 잘못됨: %s", key, sc.Region.Flag)
			}
			// 축 충돌: 같은 플래그를 Fixed와 Region이 동시에 쓰면 명령이 깨진다
			if _, dup := sc.Fixed[sc.Region.Flag]; dup {
				t.Errorf("[%s] Fixed와 Region이 같은 축(%s)을 씁니다", key, sc.Region.Flag)
			}
			// 지역 미지정 시 기본값으로 쓰이므로 반드시 있어야 한다
			if _, ok := sc.Region.Codes["전국"]; !ok {
				t.Errorf("[%s] Region.Codes에 '전국'이 없습니다 — 지역 미지정 조회가 실패합니다", key)
			}
		}
		if _, err := time.Parse("2006-01-02", sc.Verified); err != nil {
			t.Errorf("[%s] Verified 날짜 형식 오류: %q", key, sc.Verified)
		}
	}
}

// TestGetSKILLContentReflectsShortcuts AI 프롬프트가 사전에서 생성되는지 확인한다.
// 손으로 쓴 두 번째 사본이 생기면 반드시 어긋나므로, 사전 값이 그대로 나타나야 한다.
func TestGetSKILLContentReflectsShortcuts(t *testing.T) {
	content := GetSKILLContent()

	// 실제 항목 코드가 프롬프트에 있어야 한다 (예전엔 존재하지 않는 T01이 박혀 있었다)
	for _, must := range []string{
		"13103134474999",    // GDP 항목
		"DT_1J22003",        // 소비자물가 (구 DT_1J20001은 존재하지 않는 표)
		"13102871014B.0006", // 미분양 서울
		"지역별 분류 없음",         // GDP·경제활동에 붙는 경고
		"SEARCH:",           // 모르는 표를 지어내지 않게 하는 탈출구
	} {
		if !strings.Contains(content, must) {
			t.Errorf("AI 프롬프트에 %q가 없습니다", must)
		}
	}

	// 폐기된 잘못된 코드가 남아 있으면 안 된다
	for _, forbidden := range []string{"DT_1J20001", "T01=경상가격"} {
		if strings.Contains(content, forbidden) {
			t.Errorf("AI 프롬프트에 폐기된 값 %q가 남아 있습니다", forbidden)
		}
	}
}
