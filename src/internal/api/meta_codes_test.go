package api

import "testing"

func TestCodes(t *testing.T) {
	s := &MetaSummaryResult{
		Classifications: []MetaResult{
			{ObjID: "A", ObjNM: "시도별", ItmID: "26", ItmNM: "울  산"},
			{ObjID: "A", ObjNM: "시도별", ItmID: "26510", ItmNM: "울주군", UpItmID: "26"},
			{ObjID: "B", ObjNM: "산업별", ItmID: "C", ItmNM: "제조업"},
		},
		Items: []MetaResult{{ObjID: "ITEM", ItmID: "T10", ItmNM: "생산액", UnitNM: "백만원"}},
	}
	rows := s.Codes()
	if len(rows) != 4 {
		t.Fatalf("%+v", rows)
	}
	r := rows[1]
	if r.Flag != "--class1" || r.Level != 2 || r.ParentName != "울 산" || r.Path != "울 산 > 울주군" {
		t.Errorf("하위 단계: %+v", r)
	}
	if rows[2].Flag != "--class2" || rows[3].Flag != "--item" || rows[3].Unit != "백만원" {
		t.Errorf("플래그·단위: %+v %+v", rows[2], rows[3])
	}
}

// TestCodesKeepsObjSN 축 하나만 받아도(objId) 원래 축 번호(OBJ_ID_SN)를 지킨다.
func TestCodesKeepsObjSN(t *testing.T) {
	s := &MetaSummaryResult{Classifications: []MetaResult{
		{ObjID: "B", ObjNM: "산업별", ObjIDSN: "2", ItmID: "C", ItmNM: "제조업"},
		{ObjID: "C", ObjNM: "규모별", ObjIDSN: "3", ItmID: "S1", ItmNM: "1~4인"},
	}}
	rows := s.Codes()
	if rows[0].Flag != "--class2" || rows[1].Flag != "--class3" {
		t.Errorf("OBJ_ID_SN 무시: %+v", rows)
	}
}

// RSI R16: 상위가 빠진 신설 구(23125)는 같은 체계의 최상위(23) 아래로
func TestInferOrphanParents(t *testing.T) {
	rows := []MetaResult{
		{ObjID: "A", ItmID: "00"}, {ObjID: "A", ItmID: "23"}, {ObjID: "A", ItmID: "23010", UpItmID: "23"},
		{ObjID: "A", ItmID: "23125"}, {ObjID: "A", ItmID: "2"}, {ObjID: "A", ItmID: "999"},
	}
	got := inferOrphanParents(rows)
	if got[3].UpItmID != "23" {
		t.Errorf("23125 상위: %q", got[3].UpItmID)
	}
	if got[1].UpItmID != "" || got[4].UpItmID != "" || got[5].UpItmID != "" {
		t.Errorf("하위 없는 2·999·23 은 그대로: %+v", got)
	}
}
