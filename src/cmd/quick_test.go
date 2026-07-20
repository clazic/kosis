package cmd

import (
	"strings"
	"testing"

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

// TestValidateGeneratedArgs AI가 만든 명령의 검증. 값을 고치지 않고 막기만 한다.
func TestValidateGeneratedArgs(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		wantErr string
	}{
		{
			name: "정상",
			args: []string{"101", "DT_1IN1502", "--class1", "11", "--item", "T100", "--period", "Y", "--latest", "5"},
		},
		{
			name: "= 형식도 인식",
			args: []string{"101", "DT_1IN1502", "--class1=11", "--item=T100", "--period=Y"},
		},
		{
			// 사용자가 보고한 실제 실패: AI가 GDP에 class1을 통째로 생략했다
			name:    "분류 코드 누락 — API 호출 전에 잡아야 함",
			args:    []string{"301", "DT_200Y001", "--item", "T01", "--period", "Y", "--latest", "5"},
			wantErr: "분류 코드",
		},
		{
			name:    "항목 누락",
			args:    []string{"101", "DT_1IN1502", "--class1", "11", "--period", "Y"},
			wantErr: "항목 코드",
		},
		{
			name:    "주기 누락",
			args:    []string{"101", "DT_1IN1502", "--class1", "11", "--item", "T100"},
			wantErr: "수록주기",
		},
		{
			name:    "알 수 없는 주기",
			args:    []string{"101", "DT_1IN1502", "--class1", "11", "--item", "T100", "--period", "Z"},
			wantErr: "알 수 없는 수록주기",
		},
		{
			name:    "사전에 있는 표의 미지원 주기는 대조해서 거부",
			args:    []string{"116", "DT_MLTM_2086", "--class1", "x", "--item", "y", "--period", "M"},
			wantErr: "주기를 제공하지 않습니다",
		},
		{
			name: "user-id 방식은 분류 없이도 통과",
			args: []string{"101", "DT_1IN1502", "--user-id", "abc", "--item", "T100", "--period", "Y"},
		},
		{
			name: "사전에 없는 표는 구조 검증만 하고 통과 (AI 자율성 보존)",
			args: []string{"999", "DT_UNKNOWN", "--class1", "x", "--item", "y", "--period", "Q"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGeneratedArgs(tc.args)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("통과를 기대했으나 에러: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("에러 %q를 기대했으나 통과함", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("에러 = %q, %q를 포함해야 함", err.Error(), tc.wantErr)
			}
		})
	}
}
