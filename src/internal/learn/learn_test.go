package learn

import (
	"testing"

	"github.com/clazic/kosis/internal/config"
)

func TestLearnRoundTrip(t *testing.T) {
	config.SetConfigDirForTesting(t.TempDir())
	defer config.SetConfigDirForTesting("")

	s := Load()
	s.TableBad("출생 아수", "101/A", "사용자: 통계표 틀림")
	s.TableOK("출생아수", "101/B")
	s.AxisBad("101/B", "A", CondKey([]string{"울산"}), "21", "사용자: 분류 틀림")
	s.AxisOK("101/B", "A", CondKey([]string{"울산"}), "26")
	if err := s.Save(); err != nil {
		t.Fatal(err)
	}

	s = Load()
	if got := s.PreferTable("출생아수"); got != "101/B" {
		t.Errorf("prefer = %q (공백 무시 키여야 함)", got)
	}
	if _, bad := s.AvoidTables("출생아수")["101/A"]; !bad {
		t.Error("101/A가 avoid에 있어야 함")
	}
	if code, ok := s.AxisPick("101/B", "A", "울산"); !ok || code != "26" {
		t.Errorf("axis pick = %q %v", code, ok)
	}
	if !s.AvoidCodes("101/B", "A", "울산")["21"] {
		t.Error("21은 avoid여야 함")
	}

	// 교정: 틀렸던 표가 나중에 맞다고 확정되면 avoid에서 빠지고, prefer였던 표가 틀리면 prefer 해제
	s.TableOK("출생아수", "101/A")
	s.TableBad("출생아수", "101/A", "실행 실패")
	if s.PreferTable("출생아수") != "" {
		t.Error("틀린 표는 prefer에서 해제돼야 함")
	}
	if len(s.Keywords["출생아수"].Avoid) != 1 {
		t.Errorf("avoid 중복 기록: %+v", s.Keywords["출생아수"].Avoid)
	}
}

func TestCondKeyOrderInsensitive(t *testing.T) {
	if CondKey([]string{"남자", " 울산 "}) != CondKey([]string{"울산", "남자"}) {
		t.Error("조건 순서와 공백은 키에 영향이 없어야 함")
	}
}
