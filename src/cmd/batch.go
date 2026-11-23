package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	"github.com/spf13/cobra"
)

// kosis batch — 서로 독립인 kosis d 조회 여러 개를 한 번에 돌려 JSON 하나로 돌려준다.
// AI 가 조회마다 명령을 따로 내리며 왕복하는 시간을 줄인다 (2026-11-23 측정: 5단계 질문 시간의 2/3 가 모델 왕복).
var batchCmd = &cobra.Command{
	Use:   "batch <d 인자> [<d 인자> …]",
	Short: "여러 조회를 한 번에 (서로 독립인 kosis d 를 동시에)",
	Long: `서로 독립인 kosis d 조회 여러 개(최대 10)를 동시에 돌려 결과를 JSON 하나로 돌려줍니다.
인자 하나가 kosis d 에 주는 인자 한 묶음입니다 (따옴표로 묶는다: "ORG TBL -c1 … -i … -p … 시점 [계산 옵션]").
앞 결과로 다음 조회가 정해지는 경우는 묶지 말고 차례로 부릅니다.

출력 (stdout, JSON 배열 — 순서는 인자 순서):
  [{"index":1, "args":"…", "ok":true,  "rows":[… API 키 행 …], "notices":["…stderr 안내…"]},
   {"index":2, "args":"…", "ok":false, "error":"…", "notices":[…]}]
  각 조회는 kosis d … -f json --api-keys 와 같다 (조회 전 검사·규칙·경고 줄은 notices 로).
  한 조회가 실패해도 나머지는 나온다. 하나라도 실패하면 종료 코드 1.

d 인자에 넣을 수 없는 것: -f/--format, -o/--output, --chart, --pivot, --fields 는 빼고 씁니다
(결과 모양이 조회마다 같아야 한다). --sort/--top/--no-total/--change/--ratio 는 된다.`,
	Example: `  # 화성시 인구·세대수·화재 건수를 한 번에
  kosis batch "101 DT_1B040A3 -c1 41590 -i T20 -p Y -s 2025 -e 2025" \
              "101 DT_1B040B3 -c1 41590 -i T1 -p Y -s 2025 -e 2025" \
              "101 DT_1YL8601 -c1 31240 -i T10 -p Y -s 2025 -e 2025"`,
	Args: cobra.MinimumNArgs(1),
	RunE: runBatch,
}

func init() {
	rootCmd.AddCommand(batchCmd)
}

const maxBatchArgs = 10

type batchResult struct {
	Index   int              `json:"index"`
	Args    string           `json:"args"`
	OK      bool             `json:"ok"`
	Rows    []map[string]any `json:"rows,omitempty"`
	Error   string           `json:"error,omitempty"`
	Notices []string         `json:"notices,omitempty"`
}

func runBatch(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true
	if len(args) > maxBatchArgs {
		return fmt.Errorf("한 번에 %d개까지입니다 (받은 수: %d)", maxBatchArgs, len(args))
	}
	for i, a := range args {
		parts := splitArgs(a)
		if len(parts) < 2 {
			return fmt.Errorf("%d번째 인자: ORG TBL 로 시작하는 kosis d 인자여야 합니다 (받은 값: %q)", i+1, a)
		}
		for _, p := range parts {
			for _, bad := range []string{"-f", "--format", "-o", "--output", "--chart", "--chart-format", "--pivot", "--fields"} {
				if p == bad || strings.HasPrefix(p, bad+"=") {
					return fmt.Errorf("%d번째 인자: %s 는 batch 에서 쓰지 않습니다 (결과는 늘 JSON)", i+1, bad)
				}
			}
		}
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	results := make([]batchResult, len(args))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // KOSIS 에 한꺼번에 몰리지 않게
	for i, a := range args {
		wg.Add(1)
		go func(i int, a string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[i] = runOneD(self, i+1, a)
		}(i, a)
	}
	wg.Wait()
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(results); err != nil {
		return err
	}
	failed := 0
	for _, r := range results {
		if !r.OK {
			failed++
		}
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "오류: %d개 중 %d개 조회 실패 (각 항목의 error 참고)\n", len(results), failed)
		os.Exit(1)
	}
	return nil
}

// runOneD 이 실행 파일로 kosis d … -f json --api-keys 를 돌린다. stderr 줄은 notices 로.
func runOneD(self string, index int, argLine string) batchResult {
	r := batchResult{Index: index, Args: argLine}
	c := exec.Command(self, append(append([]string{"d"}, splitArgs(argLine)...), "-f", "json", "--api-keys")...)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	runErr := c.Run()
	var errLines []string
	for _, l := range strings.Split(strings.TrimSpace(stderr.String()), "\n") {
		l = strings.TrimSpace(l)
		if l == "" {
			continue
		}
		if strings.HasPrefix(l, "오류:") || strings.HasPrefix(l, "Error:") {
			errLines = append(errLines, strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(l, "오류:"), "Error:")))
			continue
		}
		r.Notices = append(r.Notices, strings.TrimSpace(strings.TrimPrefix(l, "ℹ")))
	}
	if runErr != nil {
		r.Error = strings.Join(errLines, " / ")
		if r.Error == "" {
			r.Error = runErr.Error()
		}
		return r
	}
	if err := json.Unmarshal(stdout.Bytes(), &r.Rows); err != nil {
		r.Error = "결과 JSON 을 읽지 못했습니다: " + err.Error()
		return r
	}
	if r.Rows == nil {
		r.Rows = []map[string]any{}
	}
	r.OK = true
	return r
}
