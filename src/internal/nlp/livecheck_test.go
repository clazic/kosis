//go:build livecheck

// 이 파일은 `livecheck` 태그가 있을 때만 컴파일된다.
// 기본 `go test ./...`에는 포함되지 않으므로 CI에 API 키가 없어도 안전하다.
//
//	실행: make verify-shortcuts
//
// 목적: Shortcuts 사전의 코드가 실제 KOSIS와 여전히 맞는지 확인한다.
// 통계표는 개편되므로(항목 코드 변경, 통계표 폐지) 사전은 시간이 지나면 썩는다.
// 구조적 결함은 TestShortcutIntegrity가 잡지만, "코드가 실제로 유효한가"는
// 실제로 호출해 보는 수밖에 없다.
package nlp

import (
	"testing"
	"time"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
)

// staleAfter 이 기간이 지난 항목은 재검증 대상으로 보고한다.
const staleAfter = 180 * 24 * time.Hour

func TestShortcutsAgainstLiveAPI(t *testing.T) {
	keys, err := config.GetAPIKeys()
	if err != nil || len(keys) == 0 {
		t.Skip("API 키가 없어 건너뜁니다. kosis config set-key <KEY> 후 다시 실행하세요.")
	}
	client, err := api.NewClient(keys)
	if err != nil {
		t.Fatalf("클라이언트 생성 실패: %v", err)
	}

	for _, sc := range uniqueShortcuts() {
		sc := sc
		t.Run(sc.Label, func(t *testing.T) {
			// 지역이 있는 표는 "전국"으로, 없는 표는 고정 축만으로 조회한다.
			opts := api.DataOptions{
				Item:         sc.Item,
				PrdSe:        sc.Periods[0],
				NewEstPrdCnt: "1",
			}
			for flag, val := range sc.Fixed {
				setClassByFlag(t, &opts, flag, val)
			}
			if sc.Region != nil {
				setClassByFlag(t, &opts, sc.Region.Flag, sc.Region.Codes["전국"])
			}

			rows, err := client.Data(sc.OrgID, sc.TblID, opts)
			if err != nil {
				t.Errorf("%s (%s %s) 조회 실패: %v\n  → kosis meta %s %s 로 코드를 다시 확인하고 Shortcuts를 갱신하세요.",
					sc.Label, sc.OrgID, sc.TblID, err, sc.OrgID, sc.TblID)
				return
			}
			if len(rows) == 0 {
				t.Errorf("%s (%s %s) 결과가 0건입니다 — 코드는 유효하지만 데이터가 없습니다.",
					sc.Label, sc.OrgID, sc.TblID)
				return
			}

			if v, perr := time.Parse("2006-01-02", sc.Verified); perr == nil && time.Since(v) > staleAfter {
				t.Logf("재검증 권장: %s의 Verified가 %s로 6개월 이상 지났습니다.", sc.Label, sc.Verified)
			}
		})
	}
}

// TestShortcutRegionCodesAgainstLiveAPI 지역 코드가 실제로 유효한지 표본 검사한다.
// 17개 시도를 전부 도는 것은 호출 수가 과하므로 서울만 확인한다 —
// 코드 체계가 어긋나면 서울 하나로도 충분히 드러난다.
func TestShortcutRegionCodesAgainstLiveAPI(t *testing.T) {
	keys, err := config.GetAPIKeys()
	if err != nil || len(keys) == 0 {
		t.Skip("API 키가 없어 건너뜁니다.")
	}
	client, err := api.NewClient(keys)
	if err != nil {
		t.Fatalf("클라이언트 생성 실패: %v", err)
	}

	for _, sc := range uniqueShortcuts() {
		if sc.Region == nil {
			continue
		}
		sc := sc
		t.Run(sc.Label+"/서울", func(t *testing.T) {
			opts := api.DataOptions{Item: sc.Item, PrdSe: sc.Periods[0], NewEstPrdCnt: "1"}
			for flag, val := range sc.Fixed {
				setClassByFlag(t, &opts, flag, val)
			}
			setClassByFlag(t, &opts, sc.Region.Flag, sc.Region.Codes["서울"])

			rows, err := client.Data(sc.OrgID, sc.TblID, opts)
			if err != nil {
				t.Errorf("%s 서울 조회 실패: %v\n  → Region.Codes 갱신 필요", sc.Label, err)
				return
			}
			if len(rows) == 0 {
				t.Errorf("%s 서울 결과가 0건입니다 — 지역 코드가 바뀌었을 수 있습니다.", sc.Label)
			}
		})
	}
}

// setClassByFlag "--class2" 같은 플래그명을 DataOptions의 해당 필드에 매핑한다.
func setClassByFlag(t *testing.T, opts *api.DataOptions, flag, val string) {
	t.Helper()
	switch flag {
	case "--class1":
		opts.Class1 = val
	case "--class2":
		opts.Class2 = val
	case "--class3":
		opts.Class3 = val
	case "--class4":
		opts.Class4 = val
	case "--class5":
		opts.Class5 = val
	case "--class6":
		opts.Class6 = val
	case "--class7":
		opts.Class7 = val
	case "--class8":
		opts.Class8 = val
	default:
		t.Fatalf("알 수 없는 분류 플래그: %s", flag)
	}
}
