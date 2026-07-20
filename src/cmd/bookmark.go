package cmd

import (
	"fmt"

	"github.com/clazic/kosis/internal/bookmark"
	"github.com/spf13/cobra"
)

var bookmarkCmd = &cobra.Command{
	Use:     "bookmark",
	Aliases: []string{"bm"},
	Short:   "즐겨찾기 관리",
	Long: `즐겨찾기 관리

자주 사용하는 통계표를 즐겨찾기로 저장하여 빠르게 접근합니다.
즐겨찾기는 ~/.kosis/bookmarks.yaml에 저장됩니다.

사용법:
  kosis bookmark <subcommand>
  kosis bm <subcommand>

하위 명령어:
  add <ORG_ID> <TBL_ID>   즐겨찾기 추가
  list                     즐겨찾기 목록 조회
  remove <이름|인덱스>     즐겨찾기 제거

예제:
  # 즐겨찾기 추가 (이름 자동 생성)
  kosis bm add 101 DT_1IN1502

  # 즐겨찾기 추가 (사용자 정의 이름)
  kosis bm add 101 DT_1IN1502 --name "인구통계"

  # 즐겨찾기 목록 조회
  kosis bm list

  # 이름으로 제거
  kosis bm remove "인구통계"

  # 인덱스로 제거
  kosis bm remove 0

관련 명령어:
  kosis data <ORG_ID> <TBL_ID>    즐겨찾기의 데이터 조회`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

var bookmarkAddCmd = &cobra.Command{
	Use:   "add <ORG_ID> <TBL_ID>",
	Short: "즐겨찾기 추가",
	Long: `즐겨찾기에 통계표를 추가합니다.

기관 코드(ORG_ID)와 통계표 ID(TBL_ID)는 필수입니다.
--name 플래그로 사용자 정의 이름을 지정할 수 있습니다.
이름을 지정하지 않으면 "ORG_ID_TBL_ID" 형식으로 자동 생성됩니다.

사용법:
  kosis bookmark add <ORG_ID> <TBL_ID> [--name <이름>]
  kosis bm add <ORG_ID> <TBL_ID> [--name <이름>]

플래그:
  --name <이름>   즐겨찾기 이름 (선택사항)

예제:
  # 자동 이름 생성
  kosis bm add 101 DT_1IN1502

  # 사용자 정의 이름
  kosis bm add 101 DT_1IN1502 --name "인구통계"

  # 미분양 통계표
  kosis bm add 116 DT_MLTM_2086 --name "미분양"

다음 단계:
  kosis bm list                     즐겨찾기 목록 확인
  kosis data <ORG_ID> <TBL_ID>     즐겨찾기 데이터 조회`,
	Args: cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		orgID := args[0]
		tblID := args[1]
		name, _ := cmd.Flags().GetString("name")

		if err := bookmark.Add(orgID, tblID, name); err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		if name == "" {
			name = fmt.Sprintf("%s_%s", orgID, tblID)
		}

		fmt.Printf("✓ 즐겨찾기가 추가되었습니다: %s\n", name)
	},
}

var bookmarkListCmd = &cobra.Command{
	Use:     "list",
	Aliases: []string{"ls"},
	Short:   "즐겨찾기 목록 조회",
	Long: `저장된 모든 즐겨찾기를 조회합니다.

인덱스, 이름, 기관 코드, 통계표 ID, 추가 날짜를 표시합니다.

사용법:
  kosis bookmark list
  kosis bm ls

예제:
  # 즐겨찾기 목록 조회
  kosis bm list
  kosis bm ls

다음 단계:
  kosis data <ORG_ID> <TBL_ID>     즐겨찾기 데이터 조회
  kosis bm remove <이름|인덱스>    즐겨찾기 제거`,
	Run: func(cmd *cobra.Command, args []string) {
		bookmarks, err := bookmark.List()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		if len(bookmarks) == 0 {
			fmt.Println("저장된 즐겨찾기가 없습니다.")
			fmt.Println()
			fmt.Println("처음 사용자라면:")
			fmt.Println("  kosis bm add 101 DT_1IN1502 --name \"인구통계\"")
			return
		}

		fmt.Println()
		fmt.Println("  인덱스 │ 이름                   │ ORG_ID │ TBL_ID         │ 추가 날짜")
		fmt.Println("  ──────┼────────────────────────┼────────┼────────────────┼──────────")

		for i, bm := range bookmarks {
			// 이름을 20자로 제한
			displayName := bm.Name
			if len(displayName) > 20 {
				displayName = displayName[:17] + "..."
			}

			fmt.Printf("  %-6d │ %-22s │ %-6s │ %-14s │ %s\n",
				i, displayName, bm.OrgID, bm.TblID, bm.AddedAt)
		}

		fmt.Println()
		fmt.Printf("총 %d개의 즐겨찾기\n", len(bookmarks))
	},
}

var bookmarkRemoveCmd = &cobra.Command{
	Use:   "remove <이름|인덱스>",
	Short: "즐겨찾기 제거",
	Long: `저장된 즐겨찾기를 제거합니다.

이름 또는 인덱스로 제거할 즐겨찾기를 지정합니다.

사용법:
  kosis bookmark remove <이름|인덱스>
  kosis bm remove <이름|인덱스>

예제:
  # 이름으로 제거
  kosis bm remove "인구통계"

  # 인덱스로 제거 (list 명령어로 인덱스 확인)
  kosis bm remove 0

  # 자동 생성된 이름으로 제거
  kosis bm remove "101_DT_1IN1502"

다음 단계:
  kosis bm list     현재 즐겨찾기 목록 확인`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		nameOrIndex := args[0]

		if err := bookmark.Remove(nameOrIndex); err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		fmt.Printf("✓ 즐겨찾기가 제거되었습니다: %s\n", nameOrIndex)
	},
}

func init() {
	rootCmd.AddCommand(bookmarkCmd)

	bookmarkCmd.AddCommand(bookmarkAddCmd)
	bookmarkCmd.AddCommand(bookmarkListCmd)
	bookmarkCmd.AddCommand(bookmarkRemoveCmd)

	// bookmarkAddCmd 플래그
	bookmarkAddCmd.Flags().StringP("name", "", "", "즐겨찾기에 붙일 이름. remove 시 이 이름으로 지정 가능. 미지정 시 \"ORG_ID_TBL_ID\" 형식으로 자동 생성")
}
