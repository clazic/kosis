package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/clazic/kosis/internal/history"
	"github.com/spf13/cobra"
)

var historyCmd = &cobra.Command{
	Use:     "history",
	Aliases: []string{"hi"},
	Short:   "조회 이력 관리",
	Long: `조회 이력 관리

이전에 실행한 CLI 명령어들의 이력을 조회하고, 이전 조회를 재실행합니다.
이력은 ~/.kosis/history.yaml에 저장되며, 최근 100개를 유지합니다.

사용법:
  kosis history [flags]
  kosis hi [flags]

플래그:
  -n, --limit <N>    표시할 최근 N개 (기본: 10)

하위 명령어:
  replay <ID>        특정 이력 항목 재실행
  clear              모든 이력 삭제

예제:
  # 최근 10개 이력 조회 (기본)
  kosis hi

  # 최근 20개 이력 조회
  kosis hi -n 20

  # 이력 항목 재실행
  kosis hi replay 3

  # 모든 이력 삭제
  kosis hi clear

관련 명령어:
  kosis <command> --help     각 명령어 상세 도움말`,
	Run: func(cmd *cobra.Command, args []string) {
		limit, _ := cmd.Flags().GetInt("limit")
		if limit <= 0 {
			limit = 10
		}

		entries, err := history.List(limit)
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		if len(entries) == 0 {
			fmt.Println("저장된 이력이 없습니다.")
			fmt.Println()
			fmt.Println("명령어를 실행하면 이력이 기록됩니다:")
			fmt.Println("  kosis search \"키워드\"")
			fmt.Println("  kosis data 101 DT_1IN1502 -c1 \"11\" -i T100 -p Y -l 5")
			return
		}

		// 역순으로 표시 (최근이 위에)
		displayEntries := entries
		for i := len(displayEntries)/2 - 1; i >= 0; i-- {
			opp := len(displayEntries) - 1 - i
			displayEntries[i], displayEntries[opp] = displayEntries[opp], displayEntries[i]
		}

		fmt.Println()
		fmt.Println("  ID │ 시간              │ 명령어")
		fmt.Println("  ───┼───────────────────┼──────────────────────────────────────")

		for _, entry := range displayEntries {
			// 시간을 "MM-DD HH:MM" 형식으로 표시
			t, err := time.Parse("2006-01-02T15:04:05", entry.Timestamp)
			displayTime := entry.Timestamp
			if err == nil {
				displayTime = t.Format("01-02 15:04")
			}

			// 명령어를 적당히 잘라서 표시
			displayCmd := entry.Command
			if len(displayCmd) > 40 {
				displayCmd = displayCmd[:37] + "..."
			}

			fmt.Printf("  %-2d │ %-17s │ %s\n", entry.ID, displayTime, displayCmd)
		}

		fmt.Println()
		fmt.Printf("총 %d개의 이력\n", len(entries))
	},
}

var historyReplayCmd = &cobra.Command{
	Use:   "replay <ID>",
	Short: "이전 조회 재실행",
	Long: `이전에 실행한 명령어를 다시 실행합니다.

ID는 history 명령어로 확인할 수 있습니다.
재실행 시 새 프로세스로 실행되어 현재 상태에 영향을 주지 않습니다.

사용법:
  kosis history replay <ID>
  kosis hi replay <ID>

예제:
  # 최근 이력 확인 후 재실행
  kosis hi
  kosis hi replay 3

  # 이전 데이터 조회를 다시 실행
  kosis hi replay 1

참고:
  args 가 저장된 기록(새 검색 기록)은 실행 인자 그대로 공백 든 검색어·필터까지 그대로 다시 실행합니다
  (셸을 거치지 않음). args 가 없는 옛 기록은 명령 문자열을 공백으로 나누므로 공백 든 검색어는 원래대로 되지 않습니다.
  "재실행: …" 안내는 stderr, stdout 은 다시 실행한 명령의 출력만. 없는 ID 는 종료 코드 1,
  다시 실행한 명령이 실패하면 그 종료 코드를 그대로 돌려줍니다.`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		var id int
		if _, err := fmt.Sscanf(args[0], "%d", &id); err != nil {
			return fmt.Errorf("ID는 숫자여야 합니다 (%v)", err)
		}
		cmd.SilenceUsage = true

		entry, err := history.GetByID(id)
		if err != nil {
			return err
		}

		// 안내는 stderr — stdout 은 다시 실행한 명령의 출력만 (예 -f json 파싱)
		fmt.Fprintf(os.Stderr, "재실행: kosis %s\n\n", entry.Command)

		// 저장된 인자 그대로 다시 실행 (셸을 거치지 않는다). 옛 항목은 Args 가 없어 공백으로 나눈다.
		cmdParts := entry.Args
		if len(cmdParts) == 0 {
			cmdParts = strings.Fields(entry.Command)
		}

		// 새 프로세스로 재실행하여 cobra 상태 오염 방지
		replayCmd := exec.Command(os.Args[0], cmdParts...)
		replayCmd.Stdin = os.Stdin
		replayCmd.Stdout = os.Stdout
		replayCmd.Stderr = os.Stderr
		if err := replayCmd.Run(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) { // 자식이 이미 오류를 stderr 에 썼다 — 종료 코드만 그대로
				os.Exit(exitErr.ExitCode())
			}
			return fmt.Errorf("명령어 실행 실패: %w", err)
		}
		return nil
	},
}

var historyClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "모든 이력 삭제",
	Long: `저장된 모든 조회 이력을 삭제합니다.

이 작업은 되돌릴 수 없습니다. 실행 시 확인 프롬프트가 표시됩니다.

사용법:
  kosis history clear
  kosis hi clear

예제:
  kosis hi clear`,
	Run: func(cmd *cobra.Command, args []string) {
		// 확인 프롬프트
		fmt.Print("모든 이력을 삭제하시겠습니까? (y/N): ")
		var response string
		fmt.Scanln(&response)

		if strings.ToLower(response) != "y" && strings.ToLower(response) != "yes" {
			fmt.Println("취소되었습니다.")
			return
		}

		if err := history.Clear(); err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		fmt.Println("✓ 모든 이력이 삭제되었습니다.")
	},
}

// RecordHistory는 실행된 명령어를 이력에 기록합니다.
// data 명령어 등에서 호출되어 이력을 저장합니다.
func RecordHistory(command string, resultCount int) error {
	return history.Add(command, resultCount)
}

// GetCommandAsHistory는 현재 실행된 명령어를 이력 형식으로 반환합니다.
// cobra의 명령어 정보를 사용합니다.
func GetCommandAsHistory(cmd *cobra.Command, args []string) string {
	// 전체 경로 구성 (예: search, data, indicator data 등)
	parts := []string{}
	c := cmd
	for c != nil && c != rootCmd {
		parts = append([]string{c.Name()}, parts...)
		c = c.Parent()
	}

	// 인자와 플래그 추가
	cmdStr := strings.Join(parts, " ")
	if len(args) > 0 {
		cmdStr += " " + strings.Join(args, " ")
	}

	return cmdStr
}

func init() {
	rootCmd.AddCommand(historyCmd)

	historyCmd.AddCommand(historyReplayCmd)
	historyCmd.AddCommand(historyClearCmd)

	// historyCmd 플래그
	historyCmd.Flags().IntP("limit", "n", 10, "표시할 최근 이력 개수. 이력은 최대 100개까지 보관됨")
}
