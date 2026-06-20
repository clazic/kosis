package cmd

import (
	"fmt"
	"strings"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/history"
	"github.com/clazic/kosis/internal/output"
	"github.com/spf13/cobra"
)

// 부모 명령어: kosis indicator (alias: ind)
var indicatorCmd = &cobra.Command{
	Use:     "indicator",
	Aliases: []string{"ind"},
	Short:   "통계주요지표 검색/조회",
	Long: `KOSIS 통계주요지표

1,473개 핵심 통계지표를 간편하게 검색하고 조회합니다.
통계표(kosis data)의 28만개 테이블과는 별도로 사전 컴파일된 지표 세트입니다.
분류/항목 코드 없이 지표명만으로 바로 수치를 확인할 수 있습니다.

사용법:
  kosis indicator <subcommand> [flags]
  kosis ind <subcommand>

하위 명령어:
  search (s)     지표명으로 검색
  info           지표 상세 설명 (개념, 산식, 출처)
  data   (d)     지표 수치 데이터 조회
  list   (ls)    지표 목록 트리 탐색

예제:
  # GDP 지표 검색
  kosis ind s "GDP"

  # 지표 설명 확인
  kosis ind info 160

  # 지표 수치 데이터 조회
  kosis ind d "GDP"

  # 엑셀로 저장
  kosis ind d "GDP" -o gdp.xlsx

  # 지표 목록 탐색
  kosis ind ls
  kosis ind ls I01

통계표(data)와의 차이:
  kosis data     28만개 통계표, 분류/항목/시점 세밀 조회 (전문가용)
  kosis ind      1,473개 핵심 지표, 지표명으로 간편 조회 (빠른 확인용)

다음 단계:
  kosis ind s "키워드"       지표 후보 검색
  kosis ind info <지표ID>    지표 정의/산식 확인
  kosis ind d "지표명"       시계열 수치 조회`,
	Run: func(cmd *cobra.Command, args []string) {
		cmd.Help()
	},
}

// 하위 명령어 1: kosis ind search <지표명>
var indicatorSearchCmd = &cobra.Command{
	Use:     "search <지표명>",
	Aliases: []string{"s"},
	Short:   "지표명으로 검색",
	Long: `지표명으로 통계주요지표를 검색합니다.

검색 결과에서 지표ID를 확인하여 info, data 명령어에 사용합니다.

사용법:
  kosis indicator search <지표명> [flags]
  kosis ind s <지표명>

파라미터:
  <지표명>                 검색할 지표 이름 (필수)

플래그:
  -n, --limit <N>          결과 수 (기본: 10)
  -f, --format <type>      출력 형식: table(기본), json

예제:
  # GDP 지표 검색
  kosis ind s "GDP"

  # 고용률 검색 (결과 20개)
  kosis ind s "고용률" -n 20

  # JSON 형식으로 출력
  kosis ind s "인플레이션" -f json

다음 단계:
  검색 결과에서 지표ID를 확인한 후:
  kosis ind info <지표ID>    지표 설명 확인
  kosis ind d <지표명>       지표 데이터 조회`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		jipyoNm := args[0]
		limit, _ := cmd.Flags().GetInt("limit")
		format, _ := cmd.Flags().GetString("format")

		// Get API keys from config
		keys, err := config.GetAPIKeys()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		if len(keys) == 0 {
			fmt.Printf("오류: API 키가 설정되지 않았습니다.\n")
			fmt.Println()
			fmt.Println(config.NoAPIKeyMessage())
			return
		}

		// Create API client
		client, err := api.NewClient(keys)
		if err != nil {
			fmt.Printf("클라이언트 생성 실패: %v\n", err)
			return
		}

		// Execute search by name (API 7-B)
		results, err := client.IndicatorSearchByName(jipyoNm, api.PaginationOptions{
			NumOfRows: limit,
		})
		if err != nil {
			fmt.Printf("검색 실패: %v\n", err)
			return
		}

		if len(results) == 0 {
			fmt.Printf("'%s' 지표 검색 결과가 없습니다.\n", jipyoNm)
			return
		}

		// Convert results to []map[string]interface{}
		data := make([]map[string]interface{}, len(results))
		for i, result := range results {
			data[i] = map[string]interface{}{
				"JIPYO_ID":    result.StatJipyoID,
				"JIPYO_NM":    result.StatJipyoNM,
				"JIPYO_DSC":   result.JipyoExplan,
				"UNIT":        result.UnitNM,
				"LAST_UPDATE": result.LstUpdate,
			}
		}

		// Format and output
		formatter, err := output.NewFormatter(format)
		if err != nil {
			fmt.Printf("포맷터 생성 실패: %v\n", err)
			return
		}

		opts := output.FormatOptions{
			Columns: []string{"JIPYO_ID", "JIPYO_NM", "JIPYO_DSC", "UNIT", "LAST_UPDATE"},
		}

		if err := formatter.Format(data, opts); err != nil {
			fmt.Printf("출력 실패: %v\n", err)
			return
		}

		fmt.Printf("\n검색 결과: %d개\n", len(results))

		// Record history
		history.Add("indicator search "+jipyoNm, len(results))
	},
}

// 하위 명령어 2: kosis ind info <지표ID>
var indicatorInfoCmd = &cobra.Command{
	Use:   "info <지표ID>",
	Short: "지표 상세 설명 (개념, 산식, 출처)",
	Long: `통계주요지표의 상세 설명을 조회합니다.

지표의 개념 정의, 산출 산식, 작성기관, 출처 등을 확인합니다.

사용법:
  kosis indicator info <지표ID> [flags]
  kosis ind info <지표ID>

파라미터:
  <지표ID>                 지표 ID (ind search 결과에서 확인)

플래그:
  -f, --format <type>      출력 형식: table(기본), json

예제:
  # GDP 지표 설명
  kosis ind info 160

  # JSON 형식
  kosis ind info 160 -f json

관련 명령어:
  kosis ind s "키워드"       지표 검색 (ID 확인)
  kosis ind d "지표명"       지표 수치 데이터 조회`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		jipyoID := args[0]
		format, _ := cmd.Flags().GetString("format")

		// Get API keys from config
		keys, err := config.GetAPIKeys()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		if len(keys) == 0 {
			fmt.Printf("오류: API 키가 설정되지 않았습니다.\n")
			fmt.Println()
			fmt.Println(config.NoAPIKeyMessage())
			return
		}

		// Create API client
		client, err := api.NewClient(keys)
		if err != nil {
			fmt.Printf("클라이언트 생성 실패: %v\n", err)
			return
		}

		// Execute search by ID (API 7-A)
		results, err := client.IndicatorSearchByID(jipyoID, api.PaginationOptions{})
		if err != nil {
			fmt.Printf("조회 실패: %v\n", err)
			return
		}

		if len(results) == 0 {
			fmt.Printf("지표ID '%s'을 찾을 수 없습니다.\n", jipyoID)
			return
		}

		// Convert results to []map[string]interface{}
		data := make([]map[string]interface{}, len(results))
		for i, result := range results {
			data[i] = map[string]interface{}{
				"JIPYO_ID":    result.StatJipyoID,
				"JIPYO_NM":    result.StatJipyoNM,
				"JIPYO_DSC":   result.JipyoExplan,
				"DATA_VALUE":  result.DataValue,
				"DATA_DE":     result.DataDE,
				"UNIT":        result.UnitNM,
				"LAST_UPDATE": result.LstUpdate,
			}
		}

		// Format and output
		formatter, err := output.NewFormatter(format)
		if err != nil {
			fmt.Printf("포맷터 생성 실패: %v\n", err)
			return
		}

		opts := output.FormatOptions{
			Columns: []string{"JIPYO_ID", "JIPYO_NM", "JIPYO_DSC", "DATA_VALUE", "DATA_DE", "UNIT", "LAST_UPDATE"},
		}

		if err := formatter.Format(data, opts); err != nil {
			fmt.Printf("출력 실패: %v\n", err)
			return
		}
	},
}

// 하위 명령어 3: kosis ind data <지표명>
var indicatorDataCmd = &cobra.Command{
	Use:     "data <지표명>",
	Aliases: []string{"d"},
	Short:   "지표 수치 데이터 조회",
	Long: `통계주요지표 수치 데이터를 조회합니다.

지표명이나 지표ID로 시계열 수치를 조회합니다.

사용법:
  kosis indicator data <지표명|ID> [flags]
  kosis ind d <지표명|ID>

파라미터:
  <지표명|ID>              지표 이름 또는 지표 ID

플래그:
  -n, --limit <N>          결과 수 (기본: 10)
  -f, --format <type>      출력 형식: table(기본), json, csv, md
  -o, --output <파일>      파일 저장 (.csv/.xlsx/.json/.db/.sqlite/.parquet)

예제:
  # GDP 지표 데이터 조회
  kosis ind d "GDP"

  # 엑셀로 저장
  kosis ind d "GDP" -o gdp.xlsx

  # 최근 10개 시점
  kosis ind d "실업률" -n 10

  # Markdown 출력
  kosis ind d "소비자물가" -f md

  # JSON 출력 (파이프용)
  kosis ind d "인플레이션" -f json

관련 명령어:
  kosis ind s "키워드"       먼저 검색하여 정확한 지표명 확인
  kosis ind info <ID>        지표 개념/산식 확인`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		jipyoNm := args[0]
		limit, _ := cmd.Flags().GetInt("limit")
		format, _ := cmd.Flags().GetString("format")
		outputFile, _ := cmd.Flags().GetString("output")

		// Validate output extension early so users can verify output contracts
		// without requiring API connectivity.
		if outputFile != "" {
			detected := output.DetectFormat(outputFile)
			if detected == "table" {
				if !strings.Contains(outputFile, ".") {
					fmt.Println("파일 저장 실패: 확장자가 없습니다. 지원 형식: .csv, .json, .xlsx, .db, .sqlite, .parquet")
				} else {
					fmt.Println("파일 저장 실패: 지원하지 않는 확장자입니다. 지원 형식: .csv, .json, .xlsx, .db, .sqlite, .parquet")
				}
				return
			}
		}

		// Get API keys from config
		keys, err := config.GetAPIKeys()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		if len(keys) == 0 {
			fmt.Printf("오류: API 키가 설정되지 않았습니다.\n")
			fmt.Println()
			fmt.Println(config.NoAPIKeyMessage())
			return
		}

		// Create API client
		client, err := api.NewClient(keys)
		if err != nil {
			fmt.Printf("클라이언트 생성 실패: %v\n", err)
			return
		}

		// Execute data query (API 7-D)
		results, err := client.IndicatorData(jipyoNm, api.PaginationOptions{
			NumOfRows: limit,
		})
		if err != nil {
			fmt.Printf("조회 실패: %v\n", err)
			return
		}

		if len(results) == 0 {
			fmt.Printf("'%s' 지표의 데이터가 없습니다.\n", jipyoNm)
			return
		}

		// Convert results to []map[string]interface{}
		data := make([]map[string]interface{}, len(results))
		for i, result := range results {
			data[i] = map[string]interface{}{
				"JIPYO_ID":    result.StatJipyoID,
				"JIPYO_NM":    result.StatJipyoNM,
				"DATA_VALUE":  result.DataValue,
				"DATA_DE":     result.DataDE,
				"UNIT":        result.UnitNM,
				"LAST_UPDATE": result.LstUpdate,
			}
		}

		opts := output.FormatOptions{
			Columns: []string{"JIPYO_ID", "JIPYO_NM", "DATA_VALUE", "DATA_DE", "UNIT", "LAST_UPDATE"},
		}

		// Output file path uses extension-driven formatter and writes to disk.
		if outputFile != "" {
			detected := output.DetectFormat(outputFile)

			if err := output.WriteToFile(data, outputFile, opts); err != nil {
				fmt.Printf("파일 저장 실패: %v\n", err)
				return
			}
			fmt.Printf("저장 완료: %s (%s)\n", outputFile, detected)
			fmt.Printf("\n조회 결과: %d개\n", len(results))
			history.Add("indicator data "+jipyoNm, len(results))
			return
		}

		formatter, err := output.NewFormatter(format)
		if err != nil {
			fmt.Printf("포맷터 생성 실패: %v\n", err)
			return
		}

		if err := formatter.Format(data, opts); err != nil {
			fmt.Printf("출력 실패: %v\n", err)
			return
		}

		fmt.Printf("\n조회 결과: %d개\n", len(results))
	},
}

// 하위 명령어 4: kosis ind list [목록ID]
var indicatorListCmd = &cobra.Command{
	Use:     "list [목록ID]",
	Aliases: []string{"ls"},
	Short:   "지표 목록 트리 탐색",
	Long: `통계주요지표 목록을 트리 구조로 탐색합니다.

사용법:
  kosis indicator list [상위코드]
  kosis ind ls [상위코드]

파라미터:
  [상위코드]               상위 분류 코드 (선택, 기본: 최상위)

플래그:
  -f, --format <type>      출력 형식: table(기본), json

예제:
  # 최상위 분류 목록
  kosis ind ls

  # I01 하위 목록 탐색
  kosis ind ls I01

  # JSON 형식으로 출력
  kosis ind ls -f json

관련 명령어:
  kosis ind s "키워드"       키워드로 직접 검색`,
	Run: func(cmd *cobra.Command, args []string) {
		listID := ""
		if len(args) > 0 {
			listID = args[0]
		}

		format, _ := cmd.Flags().GetString("format")

		// Get API keys from config
		keys, err := config.GetAPIKeys()
		if err != nil {
			fmt.Printf("오류: %v\n", err)
			return
		}

		if len(keys) == 0 {
			fmt.Printf("오류: API 키가 설정되지 않았습니다.\n")
			fmt.Println()
			fmt.Println(config.NoAPIKeyMessage())
			return
		}

		// Create API client
		client, err := api.NewClient(keys)
		if err != nil {
			fmt.Printf("클라이언트 생성 실패: %v\n", err)
			return
		}

		// If no listID provided, use empty string (API will handle it)
		// Execute list query (API 7-C)
		results, err := client.IndicatorList(listID, api.PaginationOptions{})
		if err != nil {
			fmt.Printf("조회 실패: %v\n", err)
			return
		}

		if len(results) == 0 {
			if listID != "" {
				fmt.Printf("목록ID '%s'에 해당하는 지표가 없습니다.\n", listID)
			} else {
				fmt.Printf("지표 목록을 찾을 수 없습니다.\n")
			}
			return
		}

		// Convert results to []map[string]interface{}
		data := make([]map[string]interface{}, len(results))
		for i, result := range results {
			data[i] = map[string]interface{}{
				"LIST_ID":  result.ListID,
				"JIPYO_ID": result.StatJipyoID,
				"JIPYO_NM": result.StatJipyoNM,
				"LEVEL":    0,
				"UNIT":     result.UnitNM,
			}
		}

		// Format and output
		formatter, err := output.NewFormatter(format)
		if err != nil {
			fmt.Printf("포맷터 생성 실패: %v\n", err)
			return
		}

		opts := output.FormatOptions{
			Columns: []string{"LIST_ID", "JIPYO_ID", "JIPYO_NM", "LEVEL", "UNIT"},
		}

		if err := formatter.Format(data, opts); err != nil {
			fmt.Printf("출력 실패: %v\n", err)
			return
		}

		fmt.Printf("\n조회 결과: %d개\n", len(results))
	},
}

func init() {
	rootCmd.AddCommand(indicatorCmd)

	// Add subcommands to indicator
	indicatorCmd.AddCommand(indicatorSearchCmd)
	indicatorCmd.AddCommand(indicatorInfoCmd)
	indicatorCmd.AddCommand(indicatorDataCmd)
	indicatorCmd.AddCommand(indicatorListCmd)

	// Flags for indicatorSearchCmd
	indicatorSearchCmd.Flags().IntP("limit", "n", 10, "결과 수")
	indicatorSearchCmd.Flags().StringP("format", "f", "table", "출력 형식 (table, json)")

	// Flags for indicatorInfoCmd
	indicatorInfoCmd.Flags().StringP("format", "f", "table", "출력 형식 (table, json)")

	// Flags for indicatorDataCmd
	indicatorDataCmd.Flags().IntP("limit", "n", 10, "결과 수")
	indicatorDataCmd.Flags().StringP("format", "f", "table", "출력 형식 (table, json)")
	indicatorDataCmd.Flags().StringP("output", "o", "", "파일 저장 (.csv/.xlsx/.json/.db/.sqlite/.parquet)")

	// Flags for indicatorListCmd
	indicatorListCmd.Flags().StringP("format", "f", "table", "출력 형식 (table, json)")
}
