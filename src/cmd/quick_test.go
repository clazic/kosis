package cmd

import (
	"fmt"
	"github.com/clazic/kosis/internal/learn"
	"strings"
	"testing"
	"time"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/nlp"
)

// TestBuildDataArgsFromMatch 자연어 → data 인자 변환의 골든 테스트.
// 에러를 기대하는 케이스가 정상 케이스만큼 중요하다 —
// 사전에 없는 값을 지어내면 조용한 오답(엉뚱한 분류의 수치)이 나가기 때문이다.
func TestBuildDataArgsFromMatch(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		want    []string // 기대 인자 (nil이면 에러 기대)
		wantErr string   // 에러 메시지에 포함돼야 할 문구
	}{
		{
			name:  "미분양은 지역이 class2에 들어가고 class1은 고정값",
			input: "서울 미분양 최근 3년",
			want: []string{"116", "DT_MLTM_2086",
				"--class1", "13102871014A.0002",
				"--class2", "13102871014B.0006",
				"--item", "13103871014T1",
				"--period", "Y", "--latest", "3"},
		},
		{
			name:  "지역 미지정이면 전국 코드를 쓴다",
			input: "미분양 최근 2년",
			want: []string{"116", "DT_MLTM_2086",
				"--class1", "13102871014A.0002",
				"--class2", "13102871014B.0005",
				"--item", "13103871014T1",
				"--period", "Y", "--latest", "2"},
		},
		{
			name:    "미분양은 연간만 제공 — 월별 요청은 거부",
			input:   "미분양 월별",
			wantErr: "주기를 제공하지 않습니다",
		},
		{
			name:    "GDP에 지역을 요청하면 거부 (계정항목별 분류라 지역이 없음)",
			input:   "서울 GDP 최근 5년",
			wantErr: "지역별 분류를 제공하지 않습니다",
		},
		{
			name:  "'전국 GDP'의 전국은 지역 필터가 아닌 수사이므로 통과",
			input: "전국 GDP 최근 5년",
			want: []string{"301", "DT_200Y001",
				"--class1", "13102134474ACC_ITEM.10101",
				"--item", "13103134474999",
				"--period", "Y", "--latest", "5"},
		},
		{
			name:  "소비자물가는 지역이 class1이고 T 계열 코드",
			input: "소비자물가 월별",
			want: []string{"101", "DT_1J22003",
				"--class1", "T10",
				"--item", "T",
				"--period", "M", "--latest", "1"},
		},
		{
			name:  "소비자물가 서울",
			input: "서울 소비자물가 최근 6개월",
			want: []string{"101", "DT_1J22003",
				"--class1", "T11",
				"--item", "T",
				"--period", "M", "--latest", "6"},
		},
		{
			name:    "실업률은 분류가 연령계층별이라 지역 조회 불가",
			input:   "서울 실업률 최근 3개월",
			wantErr: "지역별 분류를 제공하지 않습니다",
		},
		{
			name:  "인구는 표준 행정구역 코드를 그대로 쓴다",
			input: "서울 인구 최근 3년",
			want: []string{"101", "DT_1IN1502",
				"--class1", "11",
				"--item", "T100",
				"--period", "Y", "--latest", "3"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			match := nlp.Match(tc.input)
			if !match.Matched {
				t.Fatalf("바로가기 매칭 실패: %q", tc.input)
			}

			got, err := buildDataArgsFromMatch(match, "", "")

			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("에러를 기대했으나 통과함: %v", got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("에러 메시지 = %q, %q를 포함해야 함", err.Error(), tc.wantErr)
				}
				return
			}

			if err != nil {
				t.Fatalf("예상치 못한 에러: %v", err)
			}
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("\ngot:  %v\nwant: %v", got, tc.want)
			}
		})
	}
}

func TestConfirmExecutionYes(t *testing.T) {
	t.Cleanup(func() { quickAssumeYes = false })

	if err := quickCmd.Flags().Parse([]string{"--yes"}); err != nil {
		t.Fatalf("--yes 파싱 실패: %v", err)
	}
	if !quickAssumeYes {
		t.Fatal("--yes 가 quickAssumeYes 에 반영되지 않음")
	}
	if !confirmExecution() {
		t.Fatal("--yes 인데 실행이 거부됨")
	}

	quickAssumeYes = false
	if f := quickCmd.Flags().ShorthandLookup("y"); f == nil || f.Name != "yes" {
		t.Fatal("-y 단축 플래그가 --yes 에 연결되지 않음")
	}
}

func TestNormalizeQuickAIFlag(t *testing.T) {
	config.SetConfigDirForTesting(t.TempDir())
	defer config.SetConfigDirForTesting("")

	got := normalizeQuickAIFlag([]string{"q", "GDP", "--ai", "hermes", "-y"})
	if strings.Join(got, " ") != "q GDP --ai=hermes -y" {
		t.Errorf("등록 공급자는 합쳐야 함: %q", got)
	}
	got = normalizeQuickAIFlag([]string{"q", "--ai", "GDP 최근 5년"})
	if strings.Join(got, "|") != "q|--ai|GDP 최근 5년" {
		t.Errorf("미등록 값은 그대로 둬야 함: %q", got)
	}
	got = normalizeQuickAIFlag([]string{"data", "-a", "hermes"})
	if strings.Join(got, "|") != "data|-a|hermes" {
		t.Errorf("quick 외 명령은 건드리지 않아야 함: %q", got)
	}
}

func TestPeriodArgs(t *testing.T) {
	now := time.Date(2026, 10, 10, 0, 0, 0, 0, time.UTC)
	cases := []struct {
		ps   nlp.PeriodSpec
		unit string
		want string
	}{
		{nlp.PeriodSpec{Kind: "range", Start: "2021", End: "2025"}, "Y", "--start 2021 --end 2025"},
		{nlp.PeriodSpec{Kind: "range", Start: "2021", End: "2022"}, "M", "--start 202101 --end 202212"},
		{nlp.PeriodSpec{Kind: "range", Start: "2021", End: "2022"}, "Q", "--start 20211 --end 20224"},
		{nlp.PeriodSpec{Kind: "range", Start: "202304", End: "202308"}, "Q", "--start 20232 --end 20233"}, // 월→분기
		{nlp.PeriodSpec{Kind: "range", Start: "202308", End: "202402"}, "H", "--start 20232 --end 20241"},
		{nlp.PeriodSpec{Kind: "range", Start: "2020"}, "Y", "--start 2020 --end 2026"}, // "2020년 이후"
		{nlp.PeriodSpec{Kind: "latest", Latest: 5, Unit: "Y"}, "M", "--latest 60"},
		{nlp.PeriodSpec{Kind: "latest", Latest: 24, Unit: "M"}, "Y", "--latest 2"},
		{nlp.PeriodSpec{Kind: "latest", Latest: 18, Unit: "M"}, "Y", "--latest 2"}, // 올림
		{nlp.PeriodSpec{Kind: "latest", Latest: 6, Unit: "M"}, "M", "--latest 6"},
		{nlp.PeriodSpec{Kind: "latest", Latest: 3}, "Q", "--latest 3"}, // 단위 모르면 환산하지 않음
		{nlp.PeriodSpec{Kind: "list", List: []string{"2020", "2022"}}, "Y", "--periods 2020,2022"},
		{nlp.PeriodSpec{Kind: "none"}, "Y", "--latest 1"},
	}
	for _, c := range cases {
		if got := strings.Join(periodArgs(c.ps, c.unit, now), " "); got != c.want {
			t.Errorf("periodArgs(%+v, %s) = %q, want %q", c.ps, c.unit, got, c.want)
		}
	}
}

func TestBuildClassAxes(t *testing.T) {
	rows := []api.MetaResult{
		{ObjID: "B", ObjNM: "성별", ObjIDSN: "10"}, // 숫자 정렬: 10 > 2
		{ObjID: "A", ObjNM: "지역", ObjIDSN: "2"},
		{ObjID: "C", ObjNM: "연령"}, // SN 없음 → 맨 뒤
		{ObjID: "A", ObjNM: "지역", ObjIDSN: "2"},
	}
	axes, err := buildClassAxes(rows)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, a := range axes {
		got = append(got, a.id+a.flag)
	}
	if strings.Join(got, " ") != "A--class1 B--class2 C--class3" {
		t.Errorf("축 매핑 = %v", got)
	}
	if len(axes[0].rows) != 2 {
		t.Errorf("같은 축 행 묶기 실패: %d", len(axes[0].rows))
	}

	var many []api.MetaResult
	for i := 1; i <= 9; i++ {
		many = append(many, api.MetaResult{ObjID: fmt.Sprintf("X%d", i), ObjIDSN: fmt.Sprint(i)})
	}
	if _, err := buildClassAxes(many); err == nil {
		t.Error("분류축 9개 이상은 --class1~8 한계로 오류여야 함")
	}
}

func TestLearnFailureOnlyTableErrors(t *testing.T) {
	config.SetConfigDirForTesting(t.TempDir())
	defer config.SetConfigDirForTesting("")
	store := learn.Load()
	plan := &aiPlan{Topic: "출생아수|울산", Table: "101/A"}

	for _, msg := range []string{"API 오류 [30]: 데이터 없음", "API 오류 [31]: 4만 셀 초과", "dial tcp: timeout", "파일 저장 실패"} {
		if learnFailure(store, plan, fmt.Errorf("%s", msg)) {
			t.Errorf("%q 는 표 탓이 아니므로 학습하면 안 됨", msg)
		}
	}
	if len(store.AvoidTables(plan.Topic)) != 0 {
		t.Fatal("회피 기록이 생기면 안 됨")
	}
	if !learnFailure(store, plan, fmt.Errorf("API 오류 [21]: 잘못된 요청 변수")) {
		t.Error("[21] 은 학습해야 함")
	}
	if _, bad := store.AvoidTables(plan.Topic)["101/A"]; !bad {
		t.Error("[21] 후 101/A 회피 기록 없음")
	}
}

func TestCountDataRows(t *testing.T) {
	table := "+--+\n│ 지역 │ 값 │\n+--+\n│ 울산 │ 1 │\n│ 서울 │ 2 │\n+--+\n"
	empty := "+--+\n│ 지역 │ 값 │\n+--+\n+--+\n"
	cases := []struct {
		out, format, output string
		want                int
	}{
		{table, "table", "", 2},
		{empty, "table", "", 0},
		{"참고: 시점 2031에 데이터가 없어 건너뜁니다.\n[]\n", "json", "", 0},
		{`[{"a":1},{"a":2},{"a":3}]`, "json", "", 3},
		{"\ufeff지역,값\n울산,1\n", "csv", "", 1},
		{"\ufeff지역,값\n", "csv", "", 0},
		{"| 지역 | 값 |\n|---|---|\n| 울산 | 1 |\n", "md", "", 1},
		{"✓ 저장되었습니다", "table", "x.xlsx", -1},
	}
	for _, c := range cases {
		if got := countDataRows(strings.ReplaceAll(c.out, "\\n", "\n"), c.format, c.output); got != c.want {
			t.Errorf("countDataRows(%q, %s) = %d, want %d", c.out, c.format, got, c.want)
		}
	}
}

func TestTrusted(t *testing.T) {
	if trusted(srcJev, 0.2) || !trusted(srcJev, 0.5) || trusted(srcOnly, 1) || trusted(srcRequested, 1) || !trusted(srcCorrected, 1) {
		t.Error("학습 대상 판정이 잘못됨")
	}
}

func TestAxisOptionsCapsAndPrioritizes(t *testing.T) {
	var rows []api.MetaResult
	rows = append(rows, api.MetaResult{ItmID: "00", ItmNM: "전국"})
	for i := 0; i < 300; i++ {
		rows = append(rows, api.MetaResult{ItmID: fmt.Sprintf("x%03d", i), ItmNM: fmt.Sprintf("구%03d", i), UpItmID: "00"})
	}
	rows = append(rows, api.MetaResult{ItmID: "26", ItmNM: "울산광역시"}, api.MetaResult{ItmID: "26010", ItmNM: "울산광역시 중구", UpItmID: "26"})

	opts := axisOptions(rows, []string{"울산"}, map[string]bool{"x000": true})
	if len(opts) != nlp.MaxChoiceOptions {
		t.Fatalf("255개로 줄여야 함: %d", len(opts))
	}
	for _, must := range []string{"26", "26010", "00"} {
		if _, ok := opts[must]; !ok {
			t.Errorf("조건 포함·최상위 값 %s가 빠짐", must)
		}
	}
	if _, ok := opts["x000"]; ok {
		t.Error("학습으로 피한 코드는 제외돼야 함")
	}
}
