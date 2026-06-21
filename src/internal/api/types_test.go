package api

import "testing"

// TestGetFieldCodeKeys는 GetField가 분류·항목 코드 키(C1~C8, ITM_ID)를 반환하는지 검증한다.
// (지도/DB 결합용 코드 노출의 회귀 가드)
func TestGetFieldCodeKeys(t *testing.T) {
	row := DataRow{
		C1: "11010", C1NM: "종로구",
		C2: "C", C2NM: "제조업",
		C8: "Z9", ItmID: "T04", ItmNM: "출하액",
		OrgID: "101", TblID: "DT_1FS1101", DT: "1262246",
	}
	cases := map[string]string{
		"C1":     "11010",
		"C2":     "C",
		"C8":     "Z9",
		"ITM_ID": "T04",
		"C1_NM":  "종로구",
		"ITM_NM": "출하액",
		"DT":     "1262246",
		"ORG_ID": "101",
	}
	for key, want := range cases {
		if got := row.GetField(key); got != want {
			t.Errorf("GetField(%q) = %q, want %q", key, got, want)
		}
	}
	// 정의되지 않은 키는 빈 문자열
	if got := row.GetField("NOPE"); got != "" {
		t.Errorf("GetField(unknown) = %q, want empty", got)
	}
}

// TestBuildColumnMetaWithCode는 코드 포함/미포함 메타의 차이를 검증한다.
// - 기본(BuildColumnMeta)은 코드 컬럼을 포함하지 않아야 한다 (하위호환 가드).
// - WithCode는 각 분류축·항목에 코드 컬럼을 이름 컬럼 앞에 포함해야 한다.
func TestBuildColumnMetaWithCode(t *testing.T) {
	s := &MetaSummaryResult{
		Classifications: []MetaResult{
			{ObjID: "A", ObjNM: "시도별"},
			{ObjID: "A", ObjNM: "시도별"}, // 같은 ObjID 중복 → 1개로 취급
			{ObjID: "B", ObjNM: "산업별"},
		},
		Items: []MetaResult{{ObjNM: "항목"}},
	}

	has := func(cm *ColumnMeta, key string) bool {
		for _, c := range cm.Columns {
			if c.Key == key {
				return true
			}
		}
		return false
	}

	// 기본: 코드 컬럼 없음
	base := s.BuildColumnMeta()
	for _, k := range []string{"C1", "C2", "ITM_ID"} {
		if has(base, k) {
			t.Errorf("BuildColumnMeta()는 코드 컬럼 %q를 포함하면 안 됨 (하위호환 위반)", k)
		}
	}
	if !has(base, "C1_NM") || !has(base, "ITM_NM") {
		t.Error("BuildColumnMeta()에 이름 컬럼(C1_NM/ITM_NM)이 있어야 함")
	}

	// WithCode: 코드 + 이름 모두 포함
	wc := s.BuildColumnMetaWithCode()
	for _, k := range []string{"C1", "C2", "ITM_ID", "C1_NM", "C2_NM", "ITM_NM"} {
		if !has(wc, k) {
			t.Errorf("BuildColumnMetaWithCode()에 컬럼 %q가 있어야 함", k)
		}
	}

	// 코드 컬럼이 대응 이름 컬럼보다 앞에 와야 한다
	idx := func(cm *ColumnMeta, key string) int {
		for i, c := range cm.Columns {
			if c.Key == key {
				return i
			}
		}
		return -1
	}
	if idx(wc, "C1") > idx(wc, "C1_NM") {
		t.Error("코드 컬럼 C1은 이름 컬럼 C1_NM보다 앞에 와야 함")
	}
}
