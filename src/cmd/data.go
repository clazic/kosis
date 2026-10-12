package cmd

import (
	"fmt"
	"math"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/chart"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/history"
	"github.com/clazic/kosis/internal/interactive"
	"github.com/clazic/kosis/internal/output"
	"github.com/spf13/cobra"
)

var dataCmd = &cobra.Command{
	Use:     "data <ORG_ID> <TBL_ID> [flags]",
	Aliases: []string{"d"},
	Short:   "통계표 데이터 조회",
	Long: `KOSIS 통계표 데이터 조회

통계표의 실제 데이터(수치값)를 조회합니다.
분류, 항목, 시점을 지정하여 원하는 데이터를 가져옵니다.
인자 없이 실행하면 대화형 모드로 단계별 안내합니다.

주의: 조회 전 반드시 kosis meta로 분류/항목 코드를 확인하세요.
      4만 셀 초과 시 자동으로 분할 조회합니다.

사용법:
  kosis data <ORG_ID> <TBL_ID> [flags]
  kosis d <ORG_ID> <TBL_ID> [flags]
  kosis data                             대화형 모드

파라미터:
  <ORG_ID>                 기관 코드 (예: 101, 116, 301)
  <TBL_ID>                 통계표 ID (예: DT_1IN1502, DT_MLTM_2086)

필수 플래그 (--user-id 미사용 시):
  -c1, --class1 <값>      분류1 코드 (ALL, "00+11", "11*", level:1)
  -i,  --item <값>         항목 코드 (ALL, "T10+T20")
  -p,  --period <코드>     수록주기

추가 분류 플래그:
  -c2 ~ -c8               분류2~8 코드 (통계표에 분류가 여러 개일 때)

시점 플래그 (택1, 미지정시 최근 1개):
  -s, --start <시점>      시작 시점 (예: 2020, 202401)
  -e, --end <시점>         종료 시점 (예: 2024, 202412)
  -l, --latest <N>         최근 N개 시점
  --periods <시점들>       비연속 시점 (쉼표 구분, 예: "2020,2022,2025")

출력 플래그:
  -f, --format <type>      출력 형식: table(기본), json, csv, md
  -o, --output <파일>      파일 저장 (.csv/.xlsx/.json/.db/.parquet)
  --fields <필드목록>      출력 필드 선택 ("C1_NM,ITM_NM,PRD_DE,DT")
  --pivot <축>             그 축의 값을 열로 펼친 비교표 (C1~C8, ITM, PRD 또는 분류 이름)
                           예: --pivot C1 → 지역이 열, --pivot PRD → 시점이 열
                           --pivot ITM → 열 이름이 "항목명(단위)" (항목마다 단위가 달라도 한 행)
                           -o *.parquet 와 함께 쓸 수 없음 (parquet 열은 API 키로 고정)
  --api-keys               열 이름을 API 키(C1, C1_NM, ITM_NM, PRD_DE, DT …)로 — 표마다 같은 이름

차트 플래그:
  --chart <타입>           차트 생성: line, bar, pie
  --chart-format <포맷>    차트 포맷: terminal(기본), png, svg, pdf, html, excel, mermaid
  --title <제목>           차트 제목 (상단 중앙 표시)
  --subtitle <부제>        차트 부제목
  --source <출처>          출처 표시
  --note <주석>            주석 표시
  --template <이름>        HTML 템플릿 (line-chart, comparison, bar-rank, pie-share 등)
  --open                   생성 후 자동 열기

기타 플래그:
  --user-id <ID>           자료등록 방식 (userStatsId로 조회, 필수 플래그 생략 가능)
  --no-auto-split          4만 셀 초과 시 자동 분할 비활성화
  --concurrency <N>        동시 조회 워커 수 (0=자동: max(키 개수, 2)). 분할 조회 속도 제어

수록주기(-p) 코드표:
  meta 표시    -p 값        시점 형식       예시
  ─────────────────────────────────────────────────
  prdSe=년     Y 또는 "년"  2024            -p Y -l 5
  prdSe=월     M 또는 "월"  202401          -p M -s 202401 -e 202412
  prdSe=분기   Q 또는 "분기" 202401         -p Q -s 202401 -e 202404  (YYYYQQ)
  prdSe=반기   H·S 또는 "반기" 202401       -p H -s 202401 -e 202402  (YYYYHH)
  (분기·반기에 20241 처럼 5자리를 넣어도 6자리로 바꿔 보냅니다)
  prdSe=2·3·5년 등 F 또는 "3년" 2015          -p F -l 3   (여러 해 주기는 모두 F)

값 지정 규칙:
  ALL                      전체 선택
  "00+11+21"               복수 선택 (+로 구분)
  "11*"                    하위 전체 (* 접미사)

시점 플래그 조합 규칙:
  --start + --end          연속 범위 (예: 2020~2024)
  --latest N               최근 N개 시점
  --periods                원하는 시점만 골라서
  미지정                   최근 1개 시점
  (세 종류를 혼합 사용 불가)`,

	Example: `  # 서울 미분양 최근 6개월
  kosis d 116 DT_MLTM_2086 -c1 "11" -i T10 -p M -l 6

  # 전국+서울 총인구 2020~2024 (연별)
  kosis d 101 DT_1IN1502 -c1 "00+11" -i T100 -p Y -s 2020 -e 2024

  # 특정 연도만 골라서 조회 (비연속)
  kosis d 101 DT_1IN1502 -c1 "00" -i T100 -p Y --periods "2020,2022,2025"

  # 특정 월만 골라서 조회
  kosis d 101 DT_1J20001 -c1 "0" -i T10 -p M --periods "202401,202407,202501"

  # GDP 최근 5년, Markdown 테이블 출력
  kosis d 301 DT_200Y001 -c1 "10100" -i T01 -p Y -l 5 -f md

  # GDP 최근 5년, 엑셀로 저장
  kosis d 301 DT_200Y001 -c1 "10100" -i T01 -p Y -l 5 -o gdp.xlsx

  # 소비자물가 월별 JSON → jq 파이프
  kosis d 101 DT_1J20001 -c1 "0" -i T10 -p M -l 12 -f json | jq '.[].DT'

  # 대용량: 전국 인구 전체 → SQLite
  kosis d 101 DT_1IN1502 -c1 ALL -i ALL -p Y -s 2015 -e 2024 -o 인구.db

  # 차트: 울산 인구추이 라인차트
  kosis d 101 DT_1IN1502 -c1 26 -i T100 -p Y -l 10 --chart line --title "울산 인구추이"

  # 차트: HTML 인터랙티브
  kosis d 101 DT_1IN1502 -c1 26 -i T100 -p Y -l 10 --chart line --chart-format html -o pop.html --open

  # 대화형 모드 (단계별 안내)
  kosis data

주의 (에러 방지):
  - 분류/항목에 한글 이름 사용 금지 → 코드만 사용 (에러 21)
  - 검색어에 연도 포함 금지 (search에서)
  - 존재하지 않는 시점 요청 시 에러 30 → -l 5로 먼저 가용 시점 확인
  - prdSe=2년·3년·4년·5년 통계표에 -p Y 사용 시 에러 30 → -p F 사용
  - ALL 사용 시 4만 셀 초과 가능 → 자동 분할 처리됨
  - 코드는 kosis m --find/--under/--level 결과를 그대로 (짐작 금지 — 에러 21). + 나열 개수 제한은 확인되지 않음 (220개 실측 성공, level:2)
  - 분류 축이 N개인 표는 --class1 ~ --classN 모두 필요 (빠지면 빠진 플래그를 알려 줌)

자주 쓰는 통계표:
  인구          kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 5
  미분양        kosis d 116 DT_MLTM_2086 -c1 00 -i T10 -p M -l 6
  소비자물가    kosis d 101 DT_1J20001 -c1 0 -i T10 -p M -l 12
  GDP           kosis d 301 DT_200Y001 -c1 10100 -i T01 -p Y -l 5
  경제활동인구  kosis d 101 DT_1DA7002S -c1 00 -i ALL -p M -l 6

관련 명령어:
  kosis search <키워드>            통계표 검색 (ORG_ID, TBL_ID 확인)
  kosis meta <ORG_ID> <TBL_ID>    메타데이터 확인 (분류/항목 코드 확인)
  kosis bulk <userStatsId>         대용량 SDMX/XLS 다운로드`,

	Run: func(cmd *cobra.Command, args []string) {
		// 대화형 모드: 인자가 없으면 단계별 입력
		if len(args) == 0 {
			runDataInteractive()
			return
		}

		// CLI 모드: 모든 인자가 제공되어야 함
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "오류: ORG_ID와 TBL_ID는 필수입니다")
			fmt.Fprintln(os.Stderr, "  사용법: kosis d --help") // 도움말 전체는 원인을 묻는다 — 한 줄만
			os.Exit(1)
		}

		// 사용자가 --format을 명시하지 않았으면 config의 default_format 적용
		if !cmd.Flags().Changed("format") {
			if cfg, err := config.Load(); err == nil && cfg.DefaultFormat != "" {
				formatFlag = cfg.DefaultFormat
			}
		}

		orgID := args[0]
		tblID := args[1]

		periodsValue := strings.TrimSpace(periodsFlag)
		hasRange := startFlag != "" || endFlag != ""
		hasLatest := cmd.Flags().Changed("latest")
		hasPeriods := periodsValue != ""

		// 여러 코드를 쉼표로 적은 경우(-i T1,T2) KOSIS 는 [21] — '+' 로 바꾼다
		for _, f := range []*string{&class1Flag, &class2Flag, &class3Flag, &class4Flag, &class5Flag, &class6Flag, &class7Flag, &class8Flag, &itemFlag} {
			if fixed := joinCodes(*f); fixed != *f && strings.Contains(*f, ",") {
				fmt.Fprintf(os.Stderr, "ℹ 여러 코드는 '+' 로 잇습니다 — %q 를 %q 로 바꿔 조회합니다\n", *f, fixed)
				*f = fixed
			}
		}
		if sortFlag != "" && sortFlag != "desc" && sortFlag != "asc" {
			fmt.Fprintf(os.Stderr, "오류: --sort 는 desc 또는 asc 입니다 (받은 값: %q)\n", sortFlag)
			os.Exit(1)
		}
		if sortByFlag != "" && (sortFlag == "" || !map[string]bool{"value": true, "change": changeFlag != "", "pct": changeFlag != "", "ratio": ratioFlag != ""}[sortByFlag]) {
			fmt.Fprintf(os.Stderr, "오류: --sort-by 는 --sort 와 함께 value|change|pct|ratio 중 하나 (change·pct 는 --change, ratio 는 --ratio 필요). 받은 값: %q\n", sortByFlag)
			os.Exit(1)
		}
		if topFlag < 0 || (topFlag > 0 && sortFlag == "") {
			fmt.Fprintln(os.Stderr, "오류: --top N 은 1 이상이고 --sort desc|asc 와 함께 씁니다")
			os.Exit(1)
		}
		if changeFlag != "" && changeFlag != "yoy" && changeFlag != "prev" {
			fmt.Fprintf(os.Stderr, "오류: --change 는 yoy 또는 prev 입니다 (받은 값: %q)\n", changeFlag)
			os.Exit(1)
		}
		if rn, rd, _ := strings.Cut(ratioFlag, "/"); ratioFlag != "" && (strings.Count(ratioFlag, "/") != 1 || strings.TrimSpace(rn) == "" || strings.TrimSpace(rd) == "" || pivotFlag != "") {
			fmt.Fprintln(os.Stderr, "오류: --ratio 는 분자코드/분모코드 하나 (예 T2/T1), --pivot 과 함께 못 씁니다")
			os.Exit(1)
		}
		if changeFlag != "" && pivotFlag != "" {
			fmt.Fprintln(os.Stderr, "오류: --change 와 --pivot 은 함께 못 씁니다")
			os.Exit(1)
		}
		if sortFlag != "" && pivotFlag != "" {
			fmt.Fprintln(os.Stderr, "오류: --sort 와 --pivot 은 함께 못 씁니다 (피벗한 표는 값 열이 여러 개)")
			os.Exit(1)
		}

		if userIDFlag == "" && (strings.Join(givenClasses(), "") == "" || itemFlag == "" || periodFlag == "") {
			var need []string // 빠진 것만 — 다 준 것까지 필수라고 하면 무엇이 틀렸는지 헷갈린다
			if strings.Join(givenClasses(), "") == "" {
				need = append(need, "분류(-c1 …, m 의 [분류])")
			}
			if itemFlag == "" {
				need = append(need, "항목(-i, m 의 [항목])")
			}
			if periodFlag == "" {
				need = append(need, "주기(-p, m 의 [수록정보])")
			}
			fmt.Fprintf(os.Stderr, "오류: 빠진 인자: %s (또는 --user-id). 확인: kosis m %s %s\n", strings.Join(need, ", "), orgID, tblID)
			fmt.Fprintln(os.Stderr, "  사용법: kosis d --help") // 도움말 전체는 원인을 묻는다 — 한 줄만
			os.Exit(1)
		}
		if hasLatest && latestFlag <= 0 {
			fmt.Fprintln(os.Stderr, "오류: --latest는 1 이상의 정수여야 합니다.")
			fmt.Fprintln(os.Stderr, "  사용법: kosis d --help") // 도움말 전체는 원인을 묻는다 — 한 줄만
			os.Exit(1)
		}
		if (startFlag == "") != (endFlag == "") {
			fmt.Fprintln(os.Stderr, "오류: --start와 --end는 함께 사용해야 합니다.")
			fmt.Fprintln(os.Stderr, "  사용법: kosis d --help") // 도움말 전체는 원인을 묻는다 — 한 줄만
			os.Exit(1)
		}
		if hasPeriods && (hasLatest || hasRange) {
			fmt.Fprintln(os.Stderr, "오류: --periods는 --latest 또는 --start/--end와 함께 사용할 수 없습니다.")
			fmt.Fprintln(os.Stderr, "  사용법: kosis d --help") // 도움말 전체는 원인을 묻는다 — 한 줄만
			os.Exit(1)
		}
		if hasLatest && hasRange {
			fmt.Fprintln(os.Stderr, "오류: --latest와 --start/--end는 함께 사용할 수 없습니다.")
			fmt.Fprintln(os.Stderr, "  사용법: kosis d --help") // 도움말 전체는 원인을 묻는다 — 한 줄만
			os.Exit(1)
		}

		// 조회 전 검사: parquet 은 열이 API 키로 고정이라 피벗 열을 담을 수 없다
		if (changeFlag != "" || ratioFlag != "") && outputFlag != "" && output.DetectFormat(outputFlag) == "parquet" {
			fmt.Fprintln(os.Stderr, "오류: parquet 은 열이 고정(C1…DT)이라 --change·--ratio 열을 담을 수 없습니다 — .csv·.xlsx·.json 으로 저장하세요")
			os.Exit(1)
		}
		if pivotFlag != "" && outputFlag != "" && output.DetectFormat(outputFlag) == "parquet" {
			fmt.Fprintln(os.Stderr, "오류: parquet 은 열이 고정(C1…DT)이라 --pivot 결과를 담을 수 없습니다 — .csv·.xlsx·.json 으로 저장하세요")
			os.Exit(1)
		}

		// API 클라이언트 생성 (캐시 초기화 포함)
		client, err := NewAPIClient()
		if err != nil {
			fmt.Fprintf(os.Stderr, "오류: %v\n", err)
			os.Exit(1)
		}

		// 표의 분류 축 수만큼 --classN 이 있어야 한다 (빠지면 API 는 [20] objL 만 돌려준다)
		if userIDFlag == "" {
			if summary, merr := client.MetaSummary(orgID, tblID); merr == nil {
				if class1Flag == "" && len(summary.Axes()) > 0 && !hasAxis(summary, 1) { // 축 정보가 없으면(ITM 실패) 판단하지 않는다 // 1번 축이 없는 표 — API 는 objL1 을 요구하니 ALL 로 채운다
					class1Flag = "ALL"
					fmt.Fprintln(os.Stderr, "ℹ 이 표는 축 번호가 2부터라 1번 축이 없습니다 — -c1 을 ALL 로 채웠습니다")
				}
				if extra := extraClassFlags(summary, givenClasses()); len(extra) > 0 {
					fmt.Fprintf(os.Stderr, "오류: 이 표는 분류 축이 %d개라 %s 는 없는 축입니다 — 빼고 조회하세요 (축 확인: kosis m %s %s)\n", axisCount(summary), strings.Join(extra, ", "), orgID, tblID)
					os.Exit(1)
				}
				if missing := missingClassFlags(summary, givenClasses()); len(missing) > 0 {
					short := strings.NewReplacer("--class", "-c").Replace(strings.Join(missing, ", ")) // SKILL 과 같은 -cN 표기
					fmt.Fprintf(os.Stderr, "오류: 이 표는 분류 축이 %d개라 다음 플래그도 필요합니다: %s\n", axisCount(summary), short)
					fmt.Fprintf(os.Stderr, "  코드 확인: kosis m %s %s (또는 --find·--under·--level), 축 전체는 ALL\n", orgID, tblID)
					os.Exit(1)
				}
				// 수록 범위 밖 시점(-s/-e/--periods) — KOSIS 는 [30] 으로만 답해 코드 문제처럼 보인다
				if out := periodsOutOfRange(summary, periodFlag, append([]string{startFlag, endFlag}, strings.Split(periodsValue, ",")...)); out != "" {
					fmt.Fprintf(os.Stderr, "ℹ %s\n", out)
				}
				// -c1 level:2 → 그 축 2단계 코드 전부 (m --level 2 의 -c1 줄을 손으로 옮기지 않게)
				for i, f := range []*string{&class1Flag, &class2Flag, &class3Flag, &class4Flag, &class5Flag, &class6Flag, &class7Flag, &class8Flag} {
					if !strings.Contains(strings.ToLower(*f), "level:") { // (!코드 빼기는 level: 과 함께만)
						continue
					}
					expanded, lerr := expandLevels(summary, i+1, *f)
					if lerr != nil {
						fmt.Fprintf(os.Stderr, "오류: -c%d %s: %v\n", i+1, *f, lerr)
						os.Exit(1)
					}
					fmt.Fprintf(os.Stderr, "ℹ -c%d %s → 코드 %d개: %s (뺄 코드는 !코드 — 예 \"level:1+!03\")\n", i+1, *f, strings.Count(expanded, "+")+1, capList(codeNames(summary, i+1, expanded), 30))
					*f = expanded
				}
				// 표에 없는 주기를 요청하면 KOSIS 가 다른 주기 행을 오류 없이 돌려준다 (월간 표에 -p Y → 월별 행) — 조회 전에 막는다
				if have := cyclesOf(summary); len(have) > 0 && !have["?"] && !have[api.NormalizePrdSe(strings.ToUpper(periodFlag))] {
					var list []string
					for _, p := range summary.Periods {
						list = append(list, fmt.Sprintf("-p %s(%s %s~%s)", api.NormalizePrdSe(p.PrdSe), p.PrdSe, p.StrtPrdDe, p.EndPrdDe))
					}
					fmt.Fprintf(os.Stderr, "오류: 이 표에는 -p %s 주기가 없습니다. 있는 주기: %s\n", api.NormalizePrdSe(periodFlag), strings.Join(list, ", "))
					fmt.Fprintln(os.Stderr, "  다른 주기 값이 필요하면 그 주기가 있는 표를 찾으세요 (예: 연간 등락률은 연도별 표가 따로 있다)")
					os.Exit(1)
				}
			}
		}

		// 플래그에서 옵션 생성
		opts := api.DataOptions{
			Class1:     class1Flag,
			Class2:     class2Flag,
			Class3:     class3Flag,
			Class4:     class4Flag,
			Class5:     class5Flag,
			Class6:     class6Flag,
			Class7:     class7Flag,
			Class8:     class8Flag,
			Item:       itemFlag,
			PrdSe:      periodFlag,
			StartPrdDe: startFlag,
			EndPrdDe:   endFlag,
			// OutputFields는 API에 전달하지 않음 (클라이언트 사이드 필터링으로 처리)
		}
		if hasLatest && latestFlag > 0 {
			opts.NewEstPrdCnt = strconv.Itoa(latestFlag)
		}

		var results []api.DataRow

		// 실행 로직: --user-id, --periods, 기본
		if userIDFlag != "" {
			// 1. --user-id가 있으면 client.DataRegistered() 사용
			results, err = client.DataRegistered(userIDFlag, opts)
		} else if periodsValue != "" {
			// 2. --periods가 있으면 client.DataWithPeriods() 사용
			periods := strings.Split(periodsValue, ",")
			for i := range periods {
				periods[i] = strings.TrimSpace(periods[i])
			}
			results, err = client.DataWithPeriods(orgID, tblID, periods, opts)
		} else {
			// 3. 그 외 → client.DataWithAutoSplit() 사용 (4만 셀 초과 시 자동 분할)
			splitOpts := api.SplitOptions{
				MaxCells:    40000,
				NoAutoSplit: noAutoSplitFlag,
				Concurrency: concurrencyFlag,
			}
			var progressFn func(int, int)
			if formatFlag == "table" {
				progressFn = func(current, total int) {
					if total > 1 {
						fmt.Fprintf(os.Stderr, "[%d/%d] 조회 중...\n", current, total)
					}
				}
			}
			results, err = client.DataWithAutoSplit(orgID, tblID, opts, splitOpts, progressFn)
		}

		if err != nil {
			fmt.Fprintf(os.Stderr, "오류: %v\n", err)
			// [21] 잘못된 변수·[30] 데이터 없음이면 요청 코드를 메타와 대조해 어느 인자가 문제인지 짚는다
			if (strings.Contains(err.Error(), "[21]") || strings.Contains(err.Error(), "[30]")) && orgID != "" && tblID != "" && userIDFlag == "" {
				if summary, merr := client.MetaSummary(orgID, tblID); merr == nil {
					for _, line := range diagnoseCodes(summary, givenClasses(), itemFlag, orgID, tblID) {
						fmt.Fprintln(os.Stderr, line)
					}
				}
			}
			if strings.Contains(err.Error(), "[50]") {
				fmt.Fprintln(os.Stderr, "ℹ [50] 은 KOSIS 서버 오류입니다 — 잠시 뒤 다시 해 보고, 같은 조합만 계속 실패하면 ALL 대신 코드를 좁히거나 같은 주제의 다른 표(kosis s)를 쓰세요")
			}
			fmt.Fprintln(os.Stderr, "ℹ 원인을 확인했으면 다음에 같은 실수를 하지 않도록 기록하세요: kosis rules add (kosis rules add --help)")
			os.Exit(1)
		}

		// 메타 조회하여 실제 컬럼명 가져오기
		var colMeta *api.ColumnMeta
		var colMetaFull *api.ColumnMeta // 필터링 전 전체 메타 (--fields 변환용)
		if orgID != "" && tblID != "" {
			if summary, err := client.MetaSummary(orgID, tblID); err == nil {
				// JSON 출력, --with-code, --fields 중 하나면 코드 컬럼(C1~C8, ITM_ID)을 포함한다.
				// (table/csv 기본 출력은 기존대로 이름 컬럼만 — 하위호환 유지)
				if needCodeColumns(formatFlag, outputFlag, withCodeFlag, fieldsFlag) {
					colMetaFull = summary.BuildColumnMetaWithCode()
				} else {
					colMetaFull = summary.BuildColumnMeta()
				}
				colMeta = colMetaFull
				if len(results) > 0 {
					colMeta = colMetaFull.FilterByData(results)
				}
				if line := partialYear(summary, periodFlag, results); line != "" {
					fmt.Fprintln(os.Stderr, line)
				}
				if line := newerCycle(summary, periodFlag); line != "" && !hasRange && !hasPeriods { // 시점을 직접 고른 조회엔 안내하지 않는다
					fmt.Fprintln(os.Stderr, line)
				}
				if line := echoCodes(summary, givenClasses(), itemFlag); line != "" {
					fmt.Fprintln(os.Stderr, line)
				}
				if p := emptyLatest(results); p != "" {
					fmt.Fprintf(os.Stderr, "⚠ 마지막 시점 %s 의 값이 모두 비어 있습니다(- 등) — 아직 공표 전일 수 있습니다. '최근 값'은 그 전 시점을 쓰고 그렇게 밝히세요\n", p)
				}
				if ended := codesEndedEarly(givenClasses(), results, ""); len(ended) > 0 {
					fmt.Fprintf(os.Stderr, "⚠ 다른 코드보다 일찍 값이 끊긴 코드: %s\n", strings.Join(ended, ", "))
					fmt.Fprintf(os.Stderr, "  폐지·개편된 코드(이름이 같은 새 코드로 이어짐)이거나 그 시점부터 수록되지 않은 행입니다. 확인: kosis m %s %s --find <이름>\n", orgID, tblID)
					for _, h := range sameNameSuccessors(summary, ended) {
						fmt.Fprintf(os.Stderr, "  %s\n", h)
					}
				} else if exp, last := expectedLast(summary, periodFlag, endFlag, periodsValue), lastPeriod(results); len(exp) == len(last) && exp > last {
					// 모든 코드가 같은 시점에 끝났는데 표는 더 뒤까지 — 코드 탓이 아니라 이 조합(항목)·공표 시점 문제일 수 있다
					fmt.Fprintf(os.Stderr, "ℹ 이 조회의 값은 %s 까지입니다 (표의 -p %s 끝은 %s) — 항목·조합에 따라 일찍 끝났거나 아직 공표 전입니다\n", last, api.NormalizePrdSe(strings.ToUpper(periodFlag)), exp)
					for _, h := range sameNameSuccessors(summary, codesEndedEarly(givenClasses(), results, exp)) {
						fmt.Fprintf(os.Stderr, "  %s\n", h)
					}
				}
				if miss := missingClasses(givenClasses(), results); len(miss) > 0 {
					fmt.Fprintf(os.Stderr, "⚠ 요청한 분류 코드 중 결과에 한 줄도 없는 코드: %s\n", capList(miss, 10))
					inMeta, notInMeta := splitByMeta(summary, miss)
					if len(notInMeta) > 0 {
						fmt.Fprintf(os.Stderr, "  이 표 메타에 없는 코드(오타·짐작한 코드·개편된 지역 코드): %s — 코드 확인: kosis m %s %s --find <이름>\n", capList(notInMeta, 10), orgID, tblID)
					}
					if len(inMeta) > 0 {
						fmt.Fprintf(os.Stderr, "  메타에는 있지만 이 시점·조합에 행이 없는 코드(통합·폐지된 코드, 그 시점 미수록, 공표 범위 밖): %s (비공개 값은 행이 오고 수치값이 X·- 로 온다 — 이건 그 경우가 아니다)\n", capList(inMeta, 10))
					}
					for _, h := range childrenOfMissing(summary, miss) {
						fmt.Fprintf(os.Stderr, "  %s\n", h)
					}
				}
				if miss := missingItems(summary.Items, itemFlag, results); len(miss) > 0 {
					fmt.Fprintf(os.Stderr, "⚠ 요청한 항목 중 이 시점 범위에 값이 하나도 없는 항목: %s\n", strings.Join(miss, ", "))
					// -l N 은 표의 최근 시점이라 그 항목이 끝난 표에선 [30] — 수록 기간 전체로 안내한다
					span := ""
					if len(summary.Periods) > 0 {
						span = fmt.Sprintf(" -s %s -e %s", summary.Periods[0].StrtPrdDe, summary.Periods[0].EndPrdDe)
					}
					fmt.Fprintf(os.Stderr, "  항목마다 수록 시점이 다를 수 있습니다 (표의 최신 시점 ≠ 그 항목의 최신 시점). 그 항목이 있는 시점 확인: kosis d %s %s … -i <코드>%s (-l 1 은 표의 최근 시점이라 비어 있을 수 있음)\n", orgID, tblID, span)
				}
			}
		}

		// 표 주석(CMMT, 캐시됨): 잠정치·누계처럼 값을 읽는 법이 바뀌는 말은 stderr 한 줄로
		var cmmt []map[string]interface{}
		if orgID != "" && tblID != "" && userIDFlag == "" && len(results) > 0 {
			cmmt, _ = client.MetaRaw(orgID, tblID, "CMMT")
			if notesSay(cmmt, "잠정") {
				fmt.Fprintf(os.Stderr, "ℹ 표 주석에 '잠정'이 있습니다 — 최근 값은 잠정치일 수 있으니 답에 밝히세요 (kosis m %s %s --type CMMT)\n", orgID, tblID)
			}
		}
		if f := futurePeriod(results, time.Now()); f != "" {
			fmt.Fprintf(os.Stderr, "⚠ 결과에 아직 오지 않은 시점(%s)이 있습니다 — 추계·전망치일 수 있습니다(학년도 표는 아님 — 표 이름·주석 확인). 실적이 필요하면 -s/-e 로 지난 시점까지 고르세요 (올해 값도 전망일 수 있음)\n", f)
		}
		if z := allZeroItems(results); len(z) > 0 {
			fmt.Fprintf(os.Stderr, "⚠ 값이 모두 0 인 항목: %s — 작성하지 않은 항목일 수 있습니다. 같은 뜻의 다른 항목(kosis m %s %s --axis item)·ex 로 확인하고, 실제로 0 이면 0 이 답입니다\n", strings.Join(z, ", "), orgID, tblID)
		}
		if u := joinedUnit(results); u != "" {
			fmt.Fprintf(os.Stderr, "ℹ 단위 칸 %q 는 여러 단위를 이은 표 전체 단위일 수 있습니다 — 행마다 분류·항목 이름 괄호의 단위(예 '(천명당)')를 따르세요\n", u)
		}
		if len(results) > 0 && unitsEmpty(results) && !namesCarryUnit(results) {
			fmt.Fprintf(os.Stderr, "ℹ 단위 칸이 비어 있습니다 — 항목 이름 괄호, kosis m %s %s --axis item 의 [단위], kosis ex 에서 확인하고 짐작한 단위는 짐작이라고 밝히세요\n", orgID, tblID)
		}

		var changes map[string][2]string // 행 키 → {증감, 증감률}
		if changeFlag != "" {
			changes = computeChanges(results, changeFlag == "yoy")
			if notesSay(cmmt, "누계") {
				fmt.Fprintln(os.Stderr, "⚠ 이 표 주석에 '누계'가 있습니다 — 값이 연초부터의 누계면 --change 는 누계끼리 비교한 값입니다(그 달만의 증감이 아님). 그 달 값은 이번 달 누계 − 지난달 누계 (kosis m <ORG> <TBL> --type CMMT 로 확인)")
			}
			if len(changes) == 0 && len(results) > 0 {
				fmt.Fprintln(os.Stderr, "ℹ --change: 비교할 시점이 결과에 없습니다 — 비교 시점까지 받으세요 (yoy: 월 -l 13, 분기 -l 5, 년 -l 2 / prev: -l 2)")
			} else {
				// 비교 기준으로만 받은 앞 시점 행(증감 빈 칸)은 뺀다 — -l 13 이면 최신 달만 남는다
				kept, noChange := keepChanged(results, changes)
				if len(kept) < len(results) {
					fmt.Fprintf(os.Stderr, "ℹ --change: 비교 기준으로만 쓴 앞 시점 행 %d개는 뺐습니다 (남은 행 %d개)\n", len(results)-len(kept), len(kept))
				}
				if len(noChange) > 0 {
					var ids []string
					for _, r := range noChange {
						ids = append(ids, strings.TrimSpace(strings.Join([]string{r.C1, r.C2, r.C3, r.ItmID}, " "))+"@"+r.PrdDe)
					}
					fmt.Fprintf(os.Stderr, "⚠ 증감을 못 구한 최신 행 %d개 (비교 시점 값이 없거나 숫자 아님 — 신설 코드·X): %s\n", len(noChange), capList(ids, 10))
				}
				results = kept
			}
		}
		var ratios map[string]string
		if ratioFlag != "" {
			num, den, _ := strings.Cut(ratioFlag, "/")
			var rerr error
			if ratios, rerr = computeRatios(results, strings.TrimSpace(num), strings.TrimSpace(den)); rerr != nil {
				fmt.Fprintf(os.Stderr, "오류: --ratio: %v\n", rerr)
				os.Exit(1)
			}
		}
		if sortFlag == "" && len(sameNameCodes(results)) > 0 {
			// 편입 전후 코드(군위군 47720·27720)를 함께 받으면 코드별로 묶여 나온다 — 시계열로 읽게 시점순
			sort.SliceStable(results, func(i, j int) bool { return results[i].PrdDe < results[j].PrdDe })
		}
		missBeforeSort := missingClasses(givenClasses(), results) // --no-total·--top 으로 줄기 전에
		if noTotalFlag {
			before := len(results)
			results = dropTotals(results, givenClasses())
			fmt.Fprintf(os.Stderr, "--no-total: 합계 행 %d개 뺌\n", before-len(results))
		}
		var ranks map[string]string // --sort 순위 (동률은 같은 순위)
		if sortFlag != "" {
			by := sortByFlag
			if by == "" && changes != nil {
				by = "change" // --change 와 함께면 증감으로 (수치값 순위는 --sort-by value)
			}
			if ((by == "change" || by == "pct") && len(changes) == 0) || (by == "ratio" && len(ratios) == 0) {
				// 정렬 값이 하나도 없으면 원래 순서의 앞 N행이 순위처럼 나간다 — 막는다
				fmt.Fprintln(os.Stderr, "오류: --sort 기준 값(증감·비율)이 하나도 없습니다 — 비교 시점까지 받으세요 (yoy: 월 -l 13, 분기 -l 5, 년 -l 2 / prev: -l 2) 또는 --sort-by value")
				os.Exit(1)
			}
			val := func(r api.DataRow) string { return r.DT }
			switch by {
			case "change":
				val = func(r api.DataRow) string { return changes[rowKey(r)][0] }
			case "pct":
				val = func(r api.DataRow) string { return changes[rowKey(r)][1] }
			case "ratio":
				val = func(r api.DataRow) string { return ratios[rowKey(r)] }
			}
			if by != "" {
				hint := ""
				if by == "change" && sortByFlag == "" {
					hint = " — 증감률(%) 순위는 --sort-by pct"
				}
				fmt.Fprintf(os.Stderr, "ℹ --sort: %s 기준%s\n", map[string]string{"value": "수치값", "change": "증감", "pct": "증감률(%)", "ratio": "비율(%)"}[by], hint)
			}
			groups := map[string]bool{}
			grp := sortGrouper(results) // 자르기 전 결과로 한 번만
			mixedAxes := multiCodeAxes(results)
			for _, r := range results {
				groups[grp(r)] = true
			}
			results = sortTopBy(results, sortFlag == "desc", topFlag, val)
			ranks = rankRows(results, val, grp)    // 묶음은 자르기 전 결과 기준 (잘린 뒤 다시 정하면 순위가 틀린다)
			var sortSummary *api.MetaSummaryResult // 메타는 캐시돼 다시 불러도 호출이 늘지 않는다
			if orgID != "" && tblID != "" && userIDFlag == "" {
				sortSummary, _ = client.MetaSummary(orgID, tblID)
			}
			if miss := withoutCoveredParents(sortSummary, givenClasses(), missBeforeSort); len(miss) > 0 {
				// 값 없는 코드(통합시 12 등)는 순위에 없다 — 하위 코드를 넣지 않으면 그 지역이 순위에서 조용히 빠진다
				hint := " — 그 코드의 값이 없어 순위에서 빠졌습니다"
				if sortSummary != nil && len(childrenOfMissing(sortSummary, miss)) > 0 {
					hint = " — 위 '아래 코드' 안내대로 하위 코드를 넣어야 순위가 맞습니다"
				}
				fmt.Fprintf(os.Stderr, "⚠ 순위에 없는 코드: %s%s\n", capList(miss, 10), hint)
			}
			if ax := mixedAxes; len(ax) > 1 {
				fmt.Fprintf(os.Stderr, "⚠ 코드가 여럿인 축이 %d개(%s) — 순위가 그 조합으로 섞입니다. 순위를 매길 축 하나만 여러 코드로 두세요\n", len(ax), strings.Join(ax, "×"))
			}
			if len(groups) > 1 {
				fmt.Fprintf(os.Stderr, "ℹ --sort: 항목·시점 %d묶음마다 따로 정렬·순위를 매겼습니다 (한 항목·시점만 보려면 -i·-l 로 좁히세요)\n", len(groups))
			} else if topFlag > 0 && len(results) > topFlag {
				fmt.Fprintf(os.Stderr, "ℹ --top %d: %d위와 값이 같은 행까지 %d행을 보여 줍니다 (동률)\n", topFlag, topFlag, len(results))
			}
		}

		// 데이터를 동적 컬럼명으로 map 슬라이스로 변환
		dataMap := make([]map[string]interface{}, len(results))
		for i, row := range results {
			m := make(map[string]interface{})
			if colMeta != nil {
				for _, col := range colMeta.Columns {
					m[col.Label] = row.GetField(col.Key)
				}
			} else if apiKeysFlag {
				m = apiKeyRow(row) // 폴백(메타 없음·--user-id) + --api-keys: API 키 그대로
			} else {
				// 폴백: 기존 고정 키
				m["분류값명1"] = row.C1NM
				m["분류값명2"] = row.C2NM
				m["분류값명3"] = row.C3NM
				m["분류값명4"] = row.C4NM
				m["분류값명5"] = row.C5NM
				m["분류값명6"] = row.C6NM
				m["분류값명7"] = row.C7NM
				m["분류값명8"] = row.C8NM
				m["항목명"] = row.ItmNM
				m["수록주기"] = row.PrdSe
				m["수록시점"] = row.PrdDe
				m["수치값"] = row.DT
				m["단위"] = row.UnitNM
				m["최종수정일"] = row.LstChn
			}
			if changes != nil {
				c := changes[rowKey(row)]
				m["증감"], m["증감률(%)"] = c[0], c[1]
			}
			if ratios != nil {
				m["비율(%)"] = ratios[rowKey(row)]
			}
			if ranks != nil {
				m["순위"] = ranks[rowKey(row)]
			}
			dataMap[i] = m
		}

		// 포맷 옵션 생성 - 동적 컬럼 순서 지정
		formatOpts := output.FormatOptions{
			Korean:  !apiKeysFlag,
			RawKeys: apiKeysFlag,
		}
		if colMeta != nil {
			for _, col := range colMeta.Columns {
				formatOpts.Columns = append(formatOpts.Columns, col.Label)
			}
		} else if apiKeysFlag {
			formatOpts.Columns = apiKeyColumns(dataMap)
		}

		if (changes != nil || ratios != nil || ranks != nil) && len(formatOpts.Columns) > 0 { // 수치값 바로 뒤에
			dtLabel := "수치값"
			if colMeta != nil {
				dtLabel = colMeta.GetLabel("DT")
			} else if apiKeysFlag {
				dtLabel = "DT"
			}
			var cols []string
			for _, c := range formatOpts.Columns {
				cols = append(cols, c)
				if c == dtLabel {
					if changes != nil {
						cols = append(cols, "증감", "증감률(%)")
					}
					if ratios != nil {
						cols = append(cols, "비율(%)")
					}
					if ranks != nil {
						cols = append(cols, "순위")
					}
				}
			}
			formatOpts.Columns = cols
		}

		// --fields가 있으면 사용자 지정 컬럼으로 덮어쓰기
		// API 키(C1_NM, DT 등)와 한글 라벨(시도별, 수치값 등) 모두 지원
		if fieldsFlag != "" {
			fields := strings.Split(fieldsFlag, ",")
			// 변환용 메타: 필터링 전 전체 메타 사용 (필터링 후에는 매핑이 빠질 수 있음)
			lookupMeta := colMetaFull
			if lookupMeta == nil {
				lookupMeta = colMeta
			}
			for i := range fields {
				fields[i] = strings.TrimSpace(fields[i])
				if fields[i] == "비고" { // v0.5.24 까지의 LST_CHN_DE 열 이름 (지금은 최종수정일)
					fields[i] = "LST_CHN_DE"
				}
				// API 키가 입력된 경우 한글 라벨로 변환
				if lookupMeta != nil {
					if label := lookupMeta.GetLabel(fields[i]); label != fields[i] {
						fields[i] = label
					}
				}
			}
			formatOpts.Columns = fields
		}

		// --pivot: 한 축의 값을 열로 펼친다 (지역을 열로 놓은 비교표 등)
		if pivotFlag != "" {
			if colMeta == nil {
				fmt.Fprintln(os.Stderr, "오류: --pivot 은 메타(분류 이름)를 읽어야 합니다 — 메타 조회에 실패했습니다")
				os.Exit(1)
			}
			pcol, drop, ok := pivotColumn(colMetaFull, pivotFlag)
			if !ok {
				fmt.Fprintf(os.Stderr, "오류: --pivot %q 에 맞는 축이 없습니다 (C1~C8, ITM, PRD 또는 분류 이름)\n", pivotFlag)
				os.Exit(1)
			}
			drop[colMetaFull.GetLabel("LST_CHN_DE")] = true // 행마다 달라 열로 펼치면 행이 갈라진다
			var dups int
			unitCol := ""
			if pcol == colMetaFull.GetLabel("ITM_NM") { // 항목마다 단위가 다르면 단위가 행을 가른다 → 열 이름에 붙인다
				unitCol = colMetaFull.GetLabel("UNIT_NM")
				drop[unitCol] = true
			}
			if same := sameNameCodes(results); len(same) > 0 && !withCodeFlag {
				fmt.Fprintf(os.Stderr, "ℹ --pivot: 이름이 같은 다른 코드가 한 행으로 합쳐집니다: %s — 코드별로 보려면 --with-code\n", capList(same, 5))
			}
			dataMap, formatOpts.Columns, dups = pivotRows(dataMap, formatOpts.Columns, pcol, colMetaFull.GetLabel("DT"), drop, unitCol)
			if dups > 0 {
				fmt.Fprintf(os.Stderr, "경고: 같은 칸에 값이 %d번 겹쳐 첫 값만 남겼습니다 — --fields 로 나눌 축을 남기세요\n", dups)
			}
		}

		// --chart가 있으면 차트 렌더링
		if dataChartFlag != "" {
			chartType, err := chart.ParseChartType(dataChartFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "오류: %v\n", err)
				os.Exit(1)
			}

			chartFmt := chart.FormatTerminal
			if dataChartFmtFlag != "" {
				chartFmt, err = chart.ParseChartFormat(dataChartFmtFlag)
				if err != nil {
					fmt.Fprintf(os.Stderr, "오류: %v\n", err)
					os.Exit(1)
				}
			}

			seriesList, axisInfo := chart.ExtractSeriesWithAxis(dataMap)
			if len(seriesList) == 0 {
				fmt.Fprintln(os.Stderr, "오류: 차트를 생성할 수 있는 데이터가 없습니다")
				os.Exit(1)
			}

			chartOpts := chart.Options{
				Type:     chartType,
				Format:   chartFmt,
				Title:    dataChartTitle,
				Output:   outputFlag,
				Open:     dataOpenFlag,
				XLabel:   axisInfo.XLabel,
				YLabel:   axisInfo.YLabel,
				Template: dataChartTpl,
				Subtitle: dataChartSub,
				Source:   dataChartSrc,
				Note:     dataChartNote,
			}

			if err := chart.Render(seriesList, chartOpts); err != nil {
				fmt.Fprintf(os.Stderr, "차트 오류: %v\n", err)
				os.Exit(1)
			}

			if outputFlag != "" && chartFmt != chart.FormatTerminal {
				fmt.Fprintf(os.Stderr, "✓ 차트가 %s로 저장되었습니다.\n", outputFlag)
			}
			return
		}

		// --api-keys: --fields·--pivot 을 한국어 라벨로 다 처리한 뒤, 출력 직전에 열 이름만 API 키로 바꾼다
		// (라벨이 아닌 열 — 피벗으로 생긴 값 열 등 — 은 그대로)
		if outputUsesAPIKeys(apiKeysFlag, outputFlag) && colMetaFull != nil {
			toKey := map[string]string{}
			for _, cm := range []*api.ColumnMeta{colMetaFull, colMeta} {
				if cm != nil {
					for _, c := range cm.Columns {
						toKey[c.Label] = c.Key
					}
				}
			}
			toKey["증감"], toKey["증감률(%)"], toKey["비율(%)"], toKey["순위"] = "DT_CHG", "DT_CHG_PCT", "DT_RATIO_PCT", "RANK"
			dataMap, formatOpts.Columns = renameColumns(dataMap, formatOpts.Columns, toKey)
		}

		// --output이 있으면 WriteToFile() 사용
		if outputFlag != "" {
			err = output.WriteToFile(dataMap, outputFlag, formatOpts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "파일 저장 오류: %v\n", err)
				os.Exit(1)
			}
			fmt.Fprintf(os.Stderr, "✓ 데이터가 %s로 저장되었습니다.\n", outputFlag)
		} else {
			// 없으면 NewFormatter(format)으로 stdout 출력
			formatter, err := output.NewFormatter(formatFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "포맷 오류: %v\n", err)
				os.Exit(1)
			}
			err = formatter.Format(dataMap, formatOpts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "포맷팅 오류: %v\n", err)
				os.Exit(1)
			}
		}

		// Record history (simplified representation)
		histCmd := fmt.Sprintf("data %s %s -c1 %s -i %s -p %s", orgID, tblID, class1Flag, itemFlag, periodFlag)
		history.Add(histCmd, len(results))
	},
}

var (
	class1Flag       string
	class2Flag       string
	class3Flag       string
	class4Flag       string
	class5Flag       string
	class6Flag       string
	class7Flag       string
	class8Flag       string
	itemFlag         string
	periodFlag       string
	startFlag        string
	endFlag          string
	latestFlag       int
	periodsFlag      string
	formatFlag       string
	outputFlag       string
	fieldsFlag       string
	withCodeFlag     bool
	pivotFlag        string
	sortFlag         string
	topFlag          int
	changeFlag       string
	ratioFlag        string
	sortByFlag       string
	noTotalFlag      bool
	apiKeysFlag      bool
	userIDFlag       string
	noAutoSplitFlag  bool
	concurrencyFlag  int
	dataChartFlag    string
	dataChartFmtFlag string
	dataChartTitle   string
	dataChartTpl     string
	dataChartSub     string
	dataChartSrc     string
	dataChartNote    string
	dataOpenFlag     bool
)

// runDataInteractive implements interactive mode for data command.
func runDataInteractive() {
	// API 클라이언트 생성 (캐시 초기화 포함)
	client, err := NewAPIClient()
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}

	// 1. 검색어 입력
	keyword := interactive.Prompt("? 검색어를 입력하세요:")
	if keyword == "" {
		fmt.Fprintln(os.Stderr, "검색어가 입력되지 않았습니다.")
		return
	}

	// 2. 검색 실행
	fmt.Fprint(os.Stderr, "  📋 검색 중...\n")
	searchOpts := api.SearchOptions{ResultCount: 20}
	searchResults, err := client.Search(keyword, searchOpts)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}

	if len(searchResults) == 0 {
		fmt.Fprintln(os.Stderr, "검색 결과가 없습니다.")
		return
	}

	// 통계표 선택 옵션 구성
	var tableOptions []string
	for _, r := range searchResults {
		tableOptions = append(tableOptions, fmt.Sprintf("%s (%s/%s)", r.TblNM, r.OrgID, r.TblID))
	}

	// 3. 통계표 선택
	selectedIdx, _ := interactive.Select("? 통계표를 선택하세요:", tableOptions)
	if selectedIdx < 0 {
		fmt.Fprintln(os.Stderr, "통계표를 선택해야 합니다.")
		return
	}

	selectedTable := searchResults[selectedIdx]
	orgID := selectedTable.OrgID
	tblID := selectedTable.TblID

	// 4. 메타 데이터 로드
	fmt.Fprint(os.Stderr, "  📋 메타 로딩 중...\n")
	metaSummary, err := client.MetaSummary(orgID, tblID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}

	// 5. 지역(분류1) 선택
	var classOptions []string
	classCodeMap := make(map[int]string)
	for i, c := range metaSummary.Classifications {
		code := c.Code
		if code == "" {
			code = c.ItmID
		}
		label := c.Label
		if label == "" {
			label = c.ItmNM
		}
		classOptions = append(classOptions, fmt.Sprintf("%s (%s)", label, code))
		classCodeMap[i] = code
	}

	selectedClassIndices := interactive.MultiSelect("? 지역(분류)을 선택하세요 (공백 구분 번호):", classOptions)
	if len(selectedClassIndices) == 0 {
		fmt.Fprintln(os.Stderr, "분류를 선택해야 합니다.")
		return
	}

	var selectedClassCodes []string
	for _, idx := range selectedClassIndices {
		selectedClassCodes = append(selectedClassCodes, classCodeMap[idx])
	}
	class1Value := strings.Join(selectedClassCodes, "+")

	// 6. 항목 선택
	var itemOptions []string
	itemCodeMap := make(map[int]string)
	for i, it := range metaSummary.Items {
		code := it.Code
		if code == "" {
			code = it.ItmID
		}
		label := it.Label
		if label == "" {
			label = it.ItmNM
		}
		itemOptions = append(itemOptions, fmt.Sprintf("%s (%s)", label, code))
		itemCodeMap[i] = code
	}

	itemIdx, _ := interactive.Select("? 항목을 선택하세요:", itemOptions)
	if itemIdx < 0 {
		fmt.Fprintln(os.Stderr, "항목을 선택해야 합니다.")
		return
	}

	itemValue := itemCodeMap[itemIdx]

	// 7. 수록주기 자동 감지 (또는 사용자 선택)
	// 간단히 처음 주기를 사용하거나 사용자에게 물을 수 있음
	var periodValue string
	if len(metaSummary.Periods) > 0 {
		periodValue = metaSummary.Periods[0].Code
	} else {
		periodValue = "M" // 기본값
	}

	// 8. 기간: 최근 몇 개 시점?
	latestCount := interactive.SelectInt("? 기간: 최근 몇 개 시점? (기본 5):", 5)

	// 9. 데이터 조회
	fmt.Fprint(os.Stderr, "  📊 데이터 조회 중...\n")
	opts := api.DataOptions{
		Class1:       class1Value,
		Item:         itemValue,
		PrdSe:        periodValue,
		NewEstPrdCnt: fmt.Sprintf("%d", latestCount),
	}

	splitOpts := api.SplitOptions{
		MaxCells:    40000,
		NoAutoSplit: false,
		Concurrency: concurrencyFlag,
	}

	results, err := client.DataWithAutoSplit(orgID, tblID, opts, splitOpts, func(current, total int) {
		if total > 1 {
			fmt.Fprintf(os.Stderr, "[%d/%d] 조회 중...\n", current, total)
		}
	})

	if err != nil {
		fmt.Fprintf(os.Stderr, "오류: %v\n", err)
		return
	}

	fmt.Fprintf(os.Stderr, "✓ %d건 조회 완료\n", len(results))

	// 10. 재사용 명령어 표시
	fmt.Fprintf(os.Stderr, "\n💡 다음에 같은 조회:\n   kosis d %s %s -c1 \"%s\" -i %s -p %s -l %d\n\n",
		orgID, tblID, class1Value, itemValue, periodValue, latestCount)

	// 11. 내보내기 여부
	if interactive.Confirm("? 내보내기 하시겠습니까?", false) {
		exportChoices := []string{
			"CSV",
			"Excel (XLSX)",
			"JSON",
			"SQLite",
			"Parquet",
		}
		exportIdx, _ := interactive.Select("? 내보내기 형식을 선택하세요:", exportChoices)

		if exportIdx >= 0 {
			var ext string
			switch exportIdx {
			case 0:
				ext = ".csv"
			case 1:
				ext = ".xlsx"
			case 2:
				ext = ".json"
			case 3:
				ext = ".db"
			case 4:
				ext = ".parquet"
			}

			filename := fmt.Sprintf("%s_%s%s", orgID, tblID, ext)
			filename = interactive.Prompt(fmt.Sprintf("? 파일명 [기본: %s]:", filename))
			if filename == "" {
				filename = fmt.Sprintf("%s_%s%s", orgID, tblID, ext)
			}

			// 데이터를 map 슬라이스로 변환
			dataMap := make([]map[string]interface{}, len(results))
			for i, row := range results {
				dataMap[i] = map[string]interface{}{
					"C1_NM":      row.C1NM,
					"C2_NM":      row.C2NM,
					"C3_NM":      row.C3NM,
					"C4_NM":      row.C4NM,
					"C5_NM":      row.C5NM,
					"C6_NM":      row.C6NM,
					"C7_NM":      row.C7NM,
					"C8_NM":      row.C8NM,
					"ITM_NM":     row.ItmNM,
					"PRD_DE":     row.PrdDe,
					"DT":         row.DT,
					"LST_CHN_DE": row.LstChn,
					"UNIT":       row.UnitNM,
					"PRD_SE":     row.PrdSe,
				}
			}

			formatOpts := output.FormatOptions{Korean: true}
			err := output.WriteToFile(dataMap, filename, formatOpts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "파일 저장 오류: %v\n", err)
				return
			}
			fmt.Fprintf(os.Stderr, "✓ 데이터가 %s로 저장되었습니다.\n", filename)
		}
	} else {
		// 내보내기하지 않으면 테이블 형식으로 출력
		dataMap := make([]map[string]interface{}, len(results))
		for i, row := range results {
			dataMap[i] = map[string]interface{}{
				"C1_NM":  row.C1NM,
				"ITM_NM": row.ItmNM,
				"PRD_DE": row.PrdDe,
				"DT":     row.DT,
			}
		}

		formatter, err := output.NewFormatter("table")
		if err != nil {
			fmt.Fprintf(os.Stderr, "포맷 오류: %v\n", err)
			os.Exit(1)
		}
		formatOpts := output.FormatOptions{Korean: true}
		err = formatter.Format(dataMap, formatOpts)
		if err != nil {
			fmt.Fprintf(os.Stderr, "포맷팅 오류: %v\n", err)
			os.Exit(1)
		}
	}
}

func init() {
	rootCmd.AddCommand(dataCmd)

	// 플래그 정의
	// class 계열 short flag(-c1~-c8)는 root에서 사전 정규화하여 지원합니다.
	dataCmd.Flags().Var(&joinValue{&class1Flag}, "class1", "[1단계] 분류1 코드. meta의 [분류] 첫 번째 항목에 대응. -c1로 축약. ALL=전체, \"00+11\"=복수, \"11*\"=하위 전체, level:N=그 축 N단계 코드 전부 (예: -c1 \"11\"=서울, -c1 level:1 --no-total=시도 17개). 거듭 주면 + 로 합침")
	dataCmd.Flags().Var(&joinValue{&class2Flag}, "class2", "[1단계] 분류2 코드. meta [분류] 두 번째 항목. -c2로 축약 (예: 시도별 다음에 오는 산업별)")
	dataCmd.Flags().Var(&joinValue{&class3Flag}, "class3", "[1단계] 분류3 코드. meta [분류] 세 번째 항목. -c3로 축약")
	dataCmd.Flags().Var(&joinValue{&class4Flag}, "class4", "[1단계] 분류4 코드. meta [분류] 네 번째 항목. -c4로 축약")
	dataCmd.Flags().Var(&joinValue{&class5Flag}, "class5", "[1단계] 분류5 코드. meta [분류] 다섯 번째 항목. -c5로 축약")
	dataCmd.Flags().Var(&joinValue{&class6Flag}, "class6", "[1단계] 분류6 코드. meta [분류] 여섯 번째 항목. -c6로 축약")
	dataCmd.Flags().Var(&joinValue{&class7Flag}, "class7", "[1단계] 분류7 코드. meta [분류] 일곱 번째 항목. -c7로 축약")
	dataCmd.Flags().Var(&joinValue{&class8Flag}, "class8", "[1단계] 분류8 코드. meta [분류] 여덟 번째 항목. -c8로 축약")
	dataCmd.Flags().VarP(&joinValue{&itemFlag}, "item", "i", "[2단계] 항목 코드. meta의 [항목] 코드를 그대로 사용. ALL=전체, \"T10+T20\"=복수 (한글 이름 사용 시 에러 21)")
	dataCmd.Flags().StringVarP(&periodFlag, "period", "p", "", "[3단계] 수록주기. meta [수록정보]의 prdSe 값에 대응: Y=년, M=월, Q=분기, H=반기, F=2년 이상 여러 해(2·3·4·5년…) (불일치 시 에러 30)")
	dataCmd.Flags().StringVarP(&startFlag, "start", "s", "", "[4단계] 시작 시점. --end와 함께 연속 범위 지정. 주기별 형식: 연=2020, 월=202401, 분기=202401(YYYYQQ), 반기=202401(YYYYHH)")
	dataCmd.Flags().StringVarP(&endFlag, "end", "e", "", "[4단계] 종료 시점. --start와 짝으로 사용 (예: -s 2020 -e 2024)")
	dataCmd.Flags().IntVarP(&latestFlag, "latest", "l", 0, "[4단계] 표의 최근 N개 시점만 조회 (코드마다 N개가 아니다 — 중간에 끊긴 코드는 그 안의 행만). --start/--end와 혼용 불가. 시점 플래그 미지정 시 최근 1개")
	dataCmd.Flags().StringVar(&periodsFlag, "periods", "", "[4단계] 비연속 시점 골라 조회 (쉼표 구분). 예: \"2020,2022,2025\" 또는 \"202401,202407\"")
	dataCmd.Flags().StringVarP(&formatFlag, "format", "f", "table", "[5단계] 화면 출력 형식: table=정렬표, json=파이프/jq용, csv=쉼표구분, md=Markdown 표")
	dataCmd.Flags().StringVarP(&outputFlag, "output", "o", "", "[5단계] 파일로 저장. 확장자로 형식 자동 결정: .csv .xlsx .json .db(SQLite) .parquet")
	dataCmd.Flags().StringVar(&fieldsFlag, "fields", "", "[5단계] 출력할 컬럼만 선택 (쉼표 구분). 예: \"C1_NM,ITM_NM,PRD_DE,DT\" (DT=수치값)")
	dataCmd.Flags().BoolVar(&apiKeysFlag, "api-keys", false, "[5단계] 열 이름을 표마다 다른 한국어 축 이름 대신 API 키(C1, C1_NM … ITM_ID, ITM_NM, PRD_DE, DT, UNIT_NM)로 — 여러 표를 같은 코드로 처리할 때")
	dataCmd.Flags().StringVar(&sortFlag, "sort", "", "[5단계] 수치값으로 행 정렬: desc(큰 것부터)·asc. 숫자가 아닌 값(-, X, 이름)은 맨 뒤. 순위 질문용 — 전국·시도 합계 행도 함께 정렬되니 m --level 로 코드를 좁혀라")
	dataCmd.Flags().StringVar(&changeFlag, "change", "", "[5단계] 같은 계열(분류·항목)의 비교 시점 대비 '증감'·'증감률(%)' 열 추가: yoy=1년 전 같은 달·분기·해, prev=직전 시점. 비교 시점도 받아야 한다 (월 yoy 는 -l 13)")
	dataCmd.Flags().StringVar(&sortByFlag, "sort-by", "", "[5단계] --sort 기준: value=수치값, change=증감, pct=증감률(%), ratio=비율(%). 기본은 수치값, --change 와 함께면 증감")
	dataCmd.Flags().StringVar(&ratioFlag, "ratio", "", "[5단계] 같은 축의 두 코드 비율 '비율(%)' 열: 분자코드/분모코드 (예 T2/T1 항목끼리, ALL/<합계코드> 그 축 모든 행의 합계 대비, Y70+Y75/000 분자 여러 코드 — 코드별 % 와 합계 % 를 stderr 로). 분자 코드 행(ALL 이면 그 축 모든 행)에 채우고, 다른 축·시점이 같은 행끼리 나눈다 — 분모 코드도 조회에 넣어야 한다")
	dataCmd.Flags().BoolVar(&noTotalFlag, "no-total", false, "[5단계] 여러 코드를 준 축(ALL·*·+)에서 합계 행(이름이 전국·계·합계·총계·소계·전체)을 뺀다 — 순위용. 코드 하나로 고정한 축은 건드리지 않는다")
	dataCmd.Flags().IntVar(&topFlag, "top", 0, "[5단계] 정렬 뒤 앞에서 N행만 (--sort 와 함께)")
	dataCmd.Flags().StringVar(&pivotFlag, "pivot", "", "[5단계] 이 축의 값을 열로 펼쳐 비교표로: C1~C8(분류), ITM(항목 — 열 이름 \"항목명(단위)\"), PRD(시점) 또는 분류 이름(시도별). parquet 저장과 함께 못 씀")
	dataCmd.Flags().BoolVar(&withCodeFlag, "with-code", false, "[5단계] 이름 외에 원본 코드 컬럼(C1~C8, ITM_ID)도 함께 출력. 지도(행정동코드)·DB 결합 시 사용. JSON은 항상 포함")
	dataCmd.Flags().StringVar(&userIDFlag, "user-id", "", "KOSIS 웹에서 사전 등록한 userStatsId로 조회. 지정 시 --class/--item/--period 생략 가능")
	dataCmd.Flags().BoolVar(&noAutoSplitFlag, "no-auto-split", false, "4만 셀(API 상한) 초과 시 자동 분할 조회를 끔. 켜두면 초과 요청은 에러로 실패")
	dataCmd.Flags().IntVar(&concurrencyFlag, "concurrency", 0, "분할 조회 시 동시 워커 수. 0=자동 max(API 키 개수, 2). 값을 올리면 빨라지지만 API 호출 제한에 걸릴 수 있음")
	dataCmd.Flags().StringVar(&dataChartFlag, "chart", "", "[6단계] 조회 결과를 차트로 생성: line=추이, bar=비교, pie=구성비")
	dataCmd.Flags().StringVar(&dataChartFmtFlag, "chart-format", "", "[6단계] 차트 출력 포맷: terminal(기본, 텍스트), png/svg/pdf(이미지), html(인터랙티브), excel, mermaid(Markdown 삽입용)")
	dataCmd.Flags().StringVar(&dataChartTitle, "title", "", "[6단계] 차트 제목 (상단 중앙 표시)")
	dataCmd.Flags().StringVar(&dataChartTpl, "template", "", "[6단계] html 포맷일 때 쓰는 템플릿: line-chart, comparison, bar-rank, bar-vertical, stacked-bar, pie-share, area, dual-axis, heatmap, treemap, dashboard")
	dataCmd.Flags().StringVar(&dataChartSub, "subtitle", "", "[6단계] 차트 부제목 (제목 아래 표시)")
	dataCmd.Flags().StringVar(&dataChartSrc, "source", "", "[6단계] 차트 하단 출처 문구 (예: \"자료: KOSIS\")")
	dataCmd.Flags().StringVar(&dataChartNote, "note", "", "[6단계] 차트 하단 주석 문구")
	dataCmd.Flags().BoolVar(&dataOpenFlag, "open", false, "[6단계] 차트 파일 생성 후 기본 브라우저/뷰어로 자동 열기")
}

// pivotColumn --pivot 인자 → 펼칠 열 라벨과, 함께 빼야 할 열(그 축의 코드 열).
func pivotColumn(cm *api.ColumnMeta, arg string) (string, map[string]bool, bool) {
	key := strings.ToUpper(strings.TrimSpace(arg))
	switch {
	case key == "ITM" || key == "ITEM" || key == "ITM_NM":
		key = "ITM_NM"
	case key == "PRD" || key == "PRD_DE":
		key = "PRD_DE"
	case len(key) == 2 && key[0] == 'C' && key[1] >= '1' && key[1] <= '8':
		key += "_NM"
	}
	codeKey := strings.TrimSuffix(key, "_NM")
	if key == "ITM_NM" {
		codeKey = "ITM_ID"
	}
	for _, c := range cm.Columns {
		if c.Key == key || c.Label == arg {
			if c.Label == arg {
				codeKey = strings.TrimSuffix(c.Key, "_NM")
				if c.Key == "ITM_NM" {
					codeKey = "ITM_ID"
				}
			}
			return c.Label, map[string]bool{cm.GetLabel(codeKey): true}, true
		}
	}
	return "", nil, false
}

// pivotRows pivotCol 의 값마다 열을 만들고 valueCol 값을 채운다. 나머지 열(drop 제외)이 행을 가른다.
// 반환: 새 행, 새 열 순서, 같은 칸에 겹친 수(첫 값을 남김).
// unitCol 이 있으면 열 이름을 "값(단위)" 로 한다 (항목을 열로 펼칠 때).
func pivotRows(rows []map[string]interface{}, cols []string, pivotCol, valueCol string, drop map[string]bool, unitCol string) ([]map[string]interface{}, []string, int) {
	var keyCols []string
	for _, c := range cols {
		if c != pivotCol && c != valueCol && !drop[c] {
			keyCols = append(keyCols, c)
		}
	}
	var pivots []string
	seenPivot := map[string]bool{}
	index := map[string]int{}
	var out []map[string]interface{}
	dups := 0
	for _, r := range rows {
		pv := fmt.Sprint(r[pivotCol])
		if u := fmt.Sprint(r[unitCol]); unitCol != "" && r[unitCol] != nil && u != "" {
			pv += "(" + u + ")"
		}
		if !seenPivot[pv] {
			seenPivot[pv] = true
			pivots = append(pivots, pv)
		}
		parts := make([]string, len(keyCols))
		for i, c := range keyCols {
			parts[i] = fmt.Sprint(r[c])
		}
		k := strings.Join(parts, "\x00")
		i, ok := index[k]
		if !ok {
			m := map[string]interface{}{}
			for _, c := range keyCols {
				m[c] = r[c]
			}
			out = append(out, m)
			i = len(out) - 1
			index[k] = i
		}
		if _, exists := out[i][pv]; exists {
			dups++
			continue
		}
		out[i][pv] = r[valueCol]
	}
	return out, append(keyCols, pivots...), dups
}

// axisCount 표의 분류 축 수 (OBJ_ID 종류 수, 항목 제외).
func axisCount(s *api.MetaSummaryResult) int {
	seen := map[string]bool{}
	for _, c := range s.Classifications {
		seen[c.ObjID] = true
	}
	return len(seen)
}

// hasAxis 표에 n 번 분류 축이 있는지.
func hasAxis(s *api.MetaSummaryResult, n int) bool { return s.HasAxis(n) }

// missingClassFlags 축 순서대로 비어 있는 --classN 과 그 축 이름.
func missingClassFlags(s *api.MetaSummaryResult, given []string) []string {
	var missing []string
	for _, a := range s.Axes() {
		if a.N <= len(given) && strings.TrimSpace(given[a.N-1]) == "" {
			missing = append(missing, fmt.Sprintf("--class%d(%s)", a.N, a.Name))
		}
	}
	return missing
}

// fallbackAPIKeys 메타가 없을 때 --api-keys 출력 열 순서.
var fallbackAPIKeys = []string{"C1", "C1_NM", "C2", "C2_NM", "C3", "C3_NM", "C4", "C4_NM",
	"C5", "C5_NM", "C6", "C6_NM", "C7", "C7_NM", "C8", "C8_NM",
	"ITM_ID", "ITM_NM", "PRD_SE", "PRD_DE", "DT", "UNIT_NM", "LST_CHN_DE"}

// renameColumns 열 이름을 rename 에 따라 바꾼다 (없는 이름은 그대로).
func renameColumns(rows []map[string]interface{}, cols []string, rename map[string]string) ([]map[string]interface{}, []string) {
	to := func(c string) string {
		if k, ok := rename[c]; ok {
			return k
		}
		return c
	}
	out := make([]map[string]interface{}, len(rows))
	for i, r := range rows {
		m := make(map[string]interface{}, len(r))
		for k, v := range r {
			m[to(k)] = v
		}
		out[i] = m
	}
	newCols := make([]string, len(cols))
	for i, c := range cols {
		newCols[i] = to(c)
	}
	return out, newCols
}

// apiKeyRow 메타 없이 API 키로 한 행 (빈 값은 뺌).
func apiKeyRow(row api.DataRow) map[string]interface{} {
	m := map[string]interface{}{}
	for _, k := range fallbackAPIKeys {
		if v := row.GetField(k); v != "" {
			m[k] = v
		}
	}
	return m
}

// apiKeyColumns 값이 있는 API 키 열 (fallbackAPIKeys 순서).
func apiKeyColumns(rows []map[string]interface{}) []string {
	var cols []string
	for _, k := range fallbackAPIKeys {
		for _, r := range rows {
			if _, ok := r[k]; ok {
				cols = append(cols, k)
				break
			}
		}
	}
	return cols
}

// needCodeColumns 코드 열(C1~C8, ITM_ID)을 넣을지. JSON 은 항상 — -f json 이든 -o *.json 이든
// 실제로 나가는 형식으로 판단한다. parquet 도 스키마에 코드 열이 있어 항상 넣는다. --with-code·--fields 도 넣는다.
func needCodeColumns(format, outputPath string, withCode bool, fields string) bool {
	effective := format
	if outputPath != "" {
		effective = output.DetectFormat(outputPath)
	}
	return effective == "json" || effective == "parquet" || withCode || fields != ""
}

// outputUsesAPIKeys 출력 열 이름을 API 키로 바꿀지 — --api-keys 이거나, 열이 API 키로 고정된 parquet 파일.
func outputUsesAPIKeys(apiKeys bool, outputPath string) bool {
	return apiKeys || (outputPath != "" && output.DetectFormat(outputPath) == "parquet")
}

// totalName 합계 행 이름 (공백 무시). ponytail: 이름 기반 휴리스틱 — 이름이 다른 합계 행(예 '광업 및 제조업')은 못 잡는다, 그땐 코드를 직접 고른다
var totalName = regexp.MustCompile(`^((전국|계|합계|총계|소계|전산업|전체)(\(.*\))?|전체[가-힣]{0,4}|.*\(계\))$`)

// dropTotals 여러 코드를 준 축에서 이름이 합계인 행을 뺀다. 코드 하나로 고정한 축은 보지 않는다.
func dropTotals(rows []api.DataRow, given []string) []api.DataRow {
	var multi []int
	for i, g := range given {
		if g != "" && (strings.EqualFold(g, "ALL") || strings.Contains(g, "*") || strings.Contains(g, "+")) {
			multi = append(multi, i+1)
		}
	}
	out := rows[:0:0]
	for _, r := range rows {
		total := false
		for _, n := range multi {
			if totalName.MatchString(strings.Join(strings.Fields(r.GetField(fmt.Sprintf("C%d_NM", n))), "")) {
				total = true
				break
			}
		}
		if !total {
			out = append(out, r)
		}
	}
	return out
}

// notesSay 표 주석(CMMT)에 그 말이 있는지 — '누계'(월 값이 연초부터의 누계, 주택 인허가 등), '잠정'(최근 값 잠정치).
func notesSay(cmmt []map[string]interface{}, word string) bool {
	for _, c := range cmmt {
		if strings.Contains(fmt.Sprint(c["CMMT_DC"]), word) {
			return true
		}
	}
	return false
}

// unitsEmpty 결과 모든 행의 단위 칸이 비었는지.
func unitsEmpty(rows []api.DataRow) bool {
	for _, r := range rows {
		if strings.TrimSpace(r.UnitNM) != "" {
			return false
		}
	}
	return true
}

// rowKey 행 하나를 가리키는 키 (분류 코드·항목·시점).
func rowKey(r api.DataRow) string {
	return strings.Join([]string{r.C1, r.C2, r.C3, r.C4, r.C5, r.C6, r.C7, r.C8, r.ItmID, r.PrdDe}, "|")
}

// computeChanges 같은 계열(분류 코드·항목) 안에서 비교 시점 대비 증감·증감률(%).
// yoy: 연도만 1 뺀 시점(202608→202508, 2025→2024), prev: 그 계열의 직전 시점. 비교 값이 없거나 숫자가 아니면 빈 칸.
func computeChanges(rows []api.DataRow, yoy bool) map[string][2]string {
	num := func(s string) (float64, int, bool) {
		s = strings.ReplaceAll(strings.TrimSpace(s), ",", "")
		f, err := strconv.ParseFloat(s, 64)
		dec := 0
		if i := strings.IndexByte(s, '.'); i >= 0 {
			dec = len(s) - i - 1
		}
		return f, dec, err == nil
	}
	series := map[string][]api.DataRow{}
	for _, r := range rows {
		k := strings.Join([]string{r.C1, r.C2, r.C3, r.C4, r.C5, r.C6, r.C7, r.C8, r.ItmID}, "|")
		series[k] = append(series[k], r)
	}
	out := map[string][2]string{}
	for _, g := range series {
		sort.SliceStable(g, func(i, j int) bool { return g[i].PrdDe < g[j].PrdDe })
		byPrd := map[string]api.DataRow{}
		for _, r := range g {
			byPrd[r.PrdDe] = r
		}
		for i, r := range g {
			var base api.DataRow
			ok := false
			if yoy {
				if y, err := strconv.Atoi(r.PrdDe[:min(4, len(r.PrdDe))]); err == nil && len(r.PrdDe) >= 4 {
					base, ok = byPrd[strconv.Itoa(y-1)+r.PrdDe[4:]]
				}
			} else if i > 0 {
				base, ok = g[i-1], true
			}
			if !ok {
				continue
			}
			cur, cd, ok1 := num(r.DT)
			prev, pd, ok2 := num(base.DT)
			if !ok1 || !ok2 {
				continue
			}
			diff := strconv.FormatFloat(cur-prev, 'f', max(cd, pd), 64)
			pct := ""
			if prev != 0 {
				pct = strconv.FormatFloat((cur-prev)/math.Abs(prev)*100, 'f', 2, 64)
			}
			out[rowKey(r)] = [2]string{diff, pct}
		}
	}
	return out
}

// joinCodes 쉼표·'+' 로 나눈 코드를 다듬어 '+' 로 다시 잇는다 ("T1 ,T2+T3" → "T1+T2+T3"). 쉼표가 없으면 그대로.
func joinCodes(v string) string {
	if !strings.Contains(v, ",") {
		return v
	}
	var parts []string
	for _, p := range strings.FieldsFunc(v, func(r rune) bool { return r == ',' || r == '+' }) {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, "+")
}

// sortTop 수치값으로 정렬(같은 값은 원래 순서), top>0 이면 앞 N행. 숫자가 아닌 값은 방향과 상관없이 맨 뒤.
func sortTop(rows []api.DataRow, desc bool, top int) []api.DataRow {
	return sortTopBy(rows, desc, top, func(r api.DataRow) string { return r.DT })
}

// sortTopBy 정렬 기준 값을 고른다 (수치값·증감·증감률·비율 — --sort-by).
func sortTopBy(rows []api.DataRow, desc bool, top int, val func(api.DataRow) string) []api.DataRow {
	num := func(s string) (float64, bool) {
		f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", ""), 64)
		return f, err == nil
	}
	// 항목·시점이 섞이면(수출액·수입액, 2024·2025) 묶음마다 따로 — 단위가 다른 값끼리 순위를 매기지 않는다
	var order []string
	groups := map[string][]api.DataRow{}
	sortGroup := sortGrouper(rows)
	for _, r := range rows {
		k := sortGroup(r)
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	var out []api.DataRow
	for _, k := range order {
		g := groups[k]
		sort.SliceStable(g, func(i, j int) bool {
			a, aok := num(val(g[i]))
			b, bok := num(val(g[j]))
			if aok != bok {
				return aok
			}
			if desc {
				return a > b
			}
			return a < b
		})
		if top > 0 && len(g) > top {
			n := top // 경계 동률은 함께 — 경계 값이 숫자일 때만, 숫자로 비교 ("3.30" = "3.3")
			if bv, ok := num(val(g[top-1])); ok {
				for n < len(g) {
					v, ok2 := num(val(g[n]))
					if !ok2 || v != bv {
						break
					}
					n++
				}
			}
			g = g[:n]
		}
		out = append(out, g...)
	}
	return out
}

// sortGrouper --sort 를 따로 하는 묶음: 항목마다, 그리고 그 항목에 계열(분류 코드 조합)이 여럿이면 시점마다
// (시도 순위). 계열이 하나뿐이면 시점끼리 비교한다 (가장 높았던 해).
func sortGrouper(rows []api.DataRow) func(api.DataRow) string {
	series := map[string]map[string]bool{}
	for _, r := range rows {
		if series[r.ItmID] == nil {
			series[r.ItmID] = map[string]bool{}
		}
		series[r.ItmID][strings.Join([]string{r.C1, r.C2, r.C3, r.C4, r.C5, r.C6, r.C7, r.C8}, "|")] = true
	}
	return func(r api.DataRow) string {
		if len(series[r.ItmID]) > 1 {
			return r.ItmID + "|" + r.PrdDe
		}
		return r.ItmID
	}
}

// computeRatios 같은 축의 두 코드 num/den 의 비율(%) — 다른 축 코드·항목·시점이 같은 행끼리. 결과는 분자 행의 키에.
// 두 코드가 어느 축(C1~C8·항목)에 있는지 결과에서 찾는다.
func computeRatios(rows []api.DataRow, num, den string) (map[string]string, error) {
	keys := []string{"C1", "C2", "C3", "C4", "C5", "C6", "C7", "C8", "ITM_ID"}
	all := strings.EqualFold(num, "ALL") // 분자 ALL: 그 축의 모든 행을 분모 대비 % 로 (시도별 전국 대비 등)
	nums := map[string]bool{}            // 분자 여러 코드(Y70+Y75): 코드마다 % 를 채우고 합계 비율은 stderr 로
	for _, c := range strings.FieldsFunc(num, func(r rune) bool { return r == '+' || r == ',' }) {
		nums[strings.TrimSpace(c)] = true
	}
	axis := ""
	for _, k := range keys {
		hasN, hasD, other := false, false, false
		for _, r := range rows {
			v := r.GetField(k)
			hasN = hasN || nums[v]
			hasD = hasD || v == den
			other = other || (v != "" && v != den)
		}
		if hasD && (hasN || (all && other)) {
			axis = k
			break
		}
	}
	if axis == "" {
		return nil, fmt.Errorf("결과에서 %s 와 %s 가 같은 축에 함께 있지 않습니다 — 두 코드를 모두 조회에 넣으세요", num, den)
	}
	fmt.Fprintf(os.Stderr, "ℹ --ratio: %s 축에서 %s ÷ %s × 100 (비율(%%) 열) — 1인당·사업체당 같은 배수면 100 으로 나누거나 kosis ratio --scale 1\n", axis, num, den)
	other := func(r api.DataRow) string {
		var parts []string
		for _, k := range keys {
			if k != axis {
				parts = append(parts, r.GetField(k))
			}
		}
		return strings.Join(append(parts, r.PrdDe), "|")
	}
	denVal := map[string]float64{}
	for _, r := range rows {
		if r.GetField(axis) == den {
			if v, err := strconv.ParseFloat(strings.ReplaceAll(r.DT, ",", ""), 64); err == nil {
				denVal[other(r)] = v
			}
		}
	}
	out := map[string]string{}
	sums, cnt, order := map[string]float64{}, map[string]int{}, []string{}
	for _, r := range rows {
		if !all && !nums[r.GetField(axis)] {
			continue
		}
		v, err := strconv.ParseFloat(strings.ReplaceAll(r.DT, ",", ""), 64)
		d, ok := denVal[other(r)]
		if err == nil && ok && d != 0 {
			out[rowKey(r)] = strconv.FormatFloat(v/d*100, 'f', 2, 64)
			if _, seen := sums[other(r)]; !seen {
				order = append(order, other(r))
			}
			sums[other(r)] += v / d * 100
			cnt[other(r)]++
		}
	}
	if !all && len(nums) > 1 {
		var lines []string
		for _, k := range order {
			where := strings.Join(strings.FieldsFunc(k, func(r rune) bool { return r == '|' }), " ")
			if cnt[k] < len(nums) { // 일부 코드 값이 X·- 거나 행이 없으면 합계가 작게 나온다 — 내지 않는다
				lines = append(lines, fmt.Sprintf("합계 못 냄: 분자 %d개 중 %d개만 값 있음 (%s)", len(nums), cnt[k], where))
				continue
			}
			lines = append(lines, fmt.Sprintf("%s%% (%s)", strconv.FormatFloat(sums[k], 'f', 2, 64), where))
		}
		if len(lines) > 10 {
			lines = append(lines[:10], fmt.Sprintf("… 외 %d개 (코드별 %% 는 비율(%%) 열)", len(lines)-10))
		}
		for _, l := range lines {
			fmt.Fprintf(os.Stderr, "ℹ --ratio 분자 %s 합계: %s\n", num, l)
		}
	}
	return out, nil
}

// partialYear 연간(-p Y) 결과의 마지막 연도가 아직 안 끝난 해면 안내 — 월·분기 자료가 그해 중간까지면
// 연간 값은 부분 연도(1~N월 누계 등)일 수 있다.
func partialYear(summary *api.MetaSummaryResult, prdSe string, rows []api.DataRow) string {
	if api.NormalizePrdSe(prdSe) != "Y" {
		return ""
	}
	last := ""
	for _, r := range rows {
		if r.PrdDe > last {
			last = r.PrdDe
		}
	}
	if len(last) < 4 {
		return ""
	}
	for _, p := range summary.Periods {
		c := api.NormalizePrdSe(p.PrdSe)
		end := strings.NewReplacer(".", "", " ", "", "/4", "", "/2", "").Replace(strings.TrimSpace(p.EndPrdDe))
		if len(end) < 5 || end[:4] != last[:4] {
			continue
		}
		sub, err := strconv.Atoi(end[4:]) // "08"·"8"·"04" 모두 숫자로
		if err != nil {
			continue
		}
		if (c == "M" && sub != 12) || (c == "Q" && sub != 4) || ((c == "H" || c == "S") && sub != 2) {
			return fmt.Sprintf("⚠ 연간 마지막 시점 %s 은 아직 끝나지 않은 해일 수 있습니다 (-p %s 가 %s 까지) — 부분 연도 값이면 완결된 앞 연도를 쓰거나 그렇게 밝히세요", last[:4], c, strings.TrimSpace(p.EndPrdDe))
		}
	}
	return ""
}

// cyclesOf 표에 있는 주기 코드 집합 (Y·M·Q·H/S·F). 연간 A 는 Y 와 같다.
func cyclesOf(summary *api.MetaSummaryResult) map[string]bool {
	have := map[string]bool{}
	for _, p := range summary.Periods {
		c := api.NormalizePrdSe(p.PrdSe)
		if !strings.Contains("YMQHSF", c) || len(c) != 1 { // 부정기·일 등 kosis 가 모르는 이름 — 검사하지 않는다(fail-open)
			have["?"] = true
		}
		have[c] = true
		if c == "H" || c == "S" { // 반기는 H·S 둘 다 받는다
			have["H"], have["S"] = true, true
		}
	}
	return have
}

// newerCycle 요청 주기보다 더 최근 연도까지 있는 다른 주기가 있으면 안내 ("최신" 질문에 연간 값을 내는 실수 방지).
func newerCycle(summary *api.MetaSummaryResult, prdSe string) string {
	want := api.NormalizePrdSe(prdSe)
	if want == "F" { // 다년 주기 질문엔 다른 주기를 권하지 않는다
		return ""
	}
	year := func(s string) string {
		if s = strings.TrimSpace(s); len(s) >= 4 {
			return s[:4]
		}
		return ""
	}
	mine := ""
	for _, p := range summary.Periods {
		if api.NormalizePrdSe(p.PrdSe) == want {
			mine = year(p.EndPrdDe)
		}
	}
	if mine == "" {
		return ""
	}
	for _, p := range summary.Periods {
		if code := api.NormalizePrdSe(p.PrdSe); code != want && year(p.EndPrdDe) > mine {
			return fmt.Sprintf("ℹ 이 표는 -p %s 가 %s 까지로 더 최근입니다 (-p %s 는 %s 까지). '가장 최근' 값이 필요하면 -p %s 로 다시 조회하세요 (연간 질문이면 지금 값이 맞습니다)", code, p.EndPrdDe, want, mine, code)
		}
	}
	return ""
}

// givenClasses -c1 … -c8 플래그 값 (순서 = 축 번호).
func givenClasses() []string {
	return []string{class1Flag, class2Flag, class3Flag, class4Flag, class5Flag, class6Flag, class7Flag, class8Flag}
}

// echoCodes 콕 집어 준 코드(ALL·* 제외)가 이 표에서 무슨 이름인지 한 줄로 — 같은 코드가 표마다 다른 뜻일 때
// (예: 형제 표에서 C_1 = 학생수 / 직원수) 오류 없이 엉뚱한 값을 받는 것을 AI 가 바로 알아채게 한다.
func echoCodes(summary *api.MetaSummaryResult, given []string, item string) string {
	name := map[string]string{}
	for _, r := range summary.Codes() {
		name[r.Flag+"|"+r.Code] = r.Name
	}
	var parts []string
	add := func(flag, short, val string) {
		if val == "" || strings.EqualFold(val, "ALL") {
			return
		}
		codes := strings.Split(val, "+")
		for i, c := range codes {
			if i == 8 { // ponytail: 긴 목록은 앞 8개만, 나머지는 개수로
				parts = append(parts, fmt.Sprintf("%s …외 %d개", short, len(codes)-8))
				break
			}
			if c = strings.TrimSpace(c); c != "" && !strings.Contains(c, "*") {
				if n := name[flag+"|"+c]; n != "" {
					parts = append(parts, fmt.Sprintf("%s %s=%s", short, c, n))
				}
			}
		}
	}
	for i, g := range given {
		add(fmt.Sprintf("--class%d", i+1), fmt.Sprintf("-c%d", i+1), g)
	}
	add("--item", "-i", item)
	if len(parts) == 0 {
		return ""
	}
	return "조회한 코드: " + strings.Join(parts, ", ")
}

// diagnoseCodes 조회 실패 뒤 요청 코드(ALL·* 제외)를 메타와 대조한 안내 줄. 모두 있으면 조합·시점 문제라고 알린다.
func diagnoseCodes(summary *api.MetaSummaryResult, given []string, item, orgID, tblID string) []string {
	known := map[string]map[string]bool{}
	example := map[string]string{}
	for _, r := range summary.Codes() {
		if known[r.Flag] == nil {
			known[r.Flag] = map[string]bool{}
			example[r.Flag] = r.Axis + " 코드 예: " + r.Code
		}
		known[r.Flag][r.Code] = true
	}
	var bad []string
	checked := 0
	check := func(flag, short, val string) {
		if val == "" || strings.EqualFold(val, "ALL") {
			return
		}
		if known[flag] == nil { // 그 번호의 축이 이 표에 없다 (축 번호가 2부터인 표 등)
			if strings.HasPrefix(flag, "--class") && len(summary.Axes()) > 0 { // 메타가 비면 판정하지 않는다
				var axes []string
				for _, a := range summary.Axes() {
					axes = append(axes, fmt.Sprintf("-c%d %s", a.N, a.Name))
				}
				checked++
				bad = append(bad, fmt.Sprintf("%s %s (이 표에 그 번호의 축이 없음 — 있는 축: %s)", short, val, strings.Join(axes, ", ")))
			}
			return
		}
		for _, c := range strings.Split(val, "+") {
			if c = strings.TrimSpace(c); c == "" || strings.Contains(c, "*") {
				continue
			}
			checked++
			if !known[flag][c] {
				hint := example[flag]
				var cands []string // 긴 코드의 끝만 옮겨 쓴 경우 (M00 → 13102112880M00) — 후보가 하나일 때만 제안
				for k := range known[flag] {
					if len(c) >= 2 && len(k) > len(c) && strings.HasSuffix(k, c) {
						cands = append(cands, k)
					}
				}
				if len(cands) == 1 {
					hint = "혹시 " + cands[0] + " ?"
				}
				bad = append(bad, fmt.Sprintf("%s %s (%s)", short, c, hint))
			}
		}
	}
	for i, g := range given {
		check(fmt.Sprintf("--class%d", i+1), fmt.Sprintf("-c%d", i+1), g)
	}
	check("--item", "-i", item)
	if checked == 0 { // 대조한 코드가 없으면(메타가 비었거나 모두 ALL·*) 결론을 내지 않는다
		return nil
	}
	if len(bad) > 0 {
		return []string{"⚠ 이 표에 없는 코드: " + strings.Join(bad, ", "), fmt.Sprintf("  코드 찾기: kosis m %s %s --find <이름> (또는 --axis <축>)", orgID, tblID)}
	}
	span := ""
	for _, p := range summary.Periods {
		span += fmt.Sprintf(" %s %s~%s", api.NormalizePrdSe(p.PrdSe), p.StrtPrdDe, p.EndPrdDe)
	}
	return []string{"ℹ 요청한 코드는 모두 이 표에 있습니다 — 이 시점·코드 조합에 값이 없는 것입니다. 수록 시점:" + span,
		"  한 축씩 ALL 로 바꿔 어느 조합이 비는지 확인하세요 (상위·하위가 종속된 축은 합계 코드 조합이 없을 수 있음)"}
}

// emptyLatest 결과의 마지막 시점 행이 모두 숫자가 아니면(-, 빈 칸, …) 그 시점. 아니면 "".
func emptyLatest(rows []api.DataRow) string {
	last := ""
	for _, r := range rows {
		if r.PrdDe > last {
			last = r.PrdDe
		}
	}
	if last == "" {
		return ""
	}
	for _, r := range rows {
		if r.PrdDe == last {
			if _, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(r.DT), ",", ""), 64); err == nil {
				return ""
			}
			if v := strings.TrimSpace(r.DT); v == "X" || v == "x" { // 비공개는 값이 있는 것
				return ""
			}
		}
	}
	return last
}

// codesEndedEarly -cN 으로 콕 집은 코드 중 결과 전체의 마지막 시점보다 일찍 값이 끝난 코드 ("-c1 46110(~2025)").
// 개편으로 옛 코드가 중간에 끊겼는데 앞 시점 값이 있어 missingClasses 에 안 걸리는 경우를 잡는다.
func codesEndedEarly(given []string, rows []api.DataRow, expected string) []string {
	last := ""
	for _, r := range rows {
		if r.PrdDe > last {
			last = r.PrdDe
		}
	}
	if len(expected) == len(last) && expected > last { // 코드 하나만 줬을 때도 — 표가 더 뒤까지 있으면 그 코드는 끊긴 것
		last = expected
	}
	var out []string
	for i, g := range given {
		if g == "" || strings.EqualFold(g, "ALL") {
			continue
		}
		key := fmt.Sprintf("C%d", i+1)
		end := map[string]string{}
		for _, r := range rows {
			if c := r.GetField(key); r.PrdDe > end[c] {
				end[c] = r.PrdDe
			}
		}
		for _, c := range strings.Split(g, "+") {
			if c = strings.TrimSpace(c); c != "" && !strings.Contains(c, "*") && end[c] != "" && end[c] < last {
				out = append(out, fmt.Sprintf("-c%d %s(~%s)", i+1, c, end[c]))
			}
		}
	}
	return out
}

// missingClasses -cN 에 콕 집어 준 코드(ALL·* 제외) 중 결과 행에 한 번도 안 나온 것 ("-c2 24" 형태).
func missingClasses(given []string, rows []api.DataRow) []string {
	if len(rows) == 0 {
		return nil
	}
	var out []string
	for i, g := range given {
		if g == "" || strings.EqualFold(g, "ALL") {
			continue
		}
		key := fmt.Sprintf("C%d", i+1)
		got := map[string]bool{}
		for _, r := range rows {
			got[r.GetField(key)] = true
		}
		for _, c := range strings.Split(g, "+") {
			if c = strings.TrimSpace(c); c != "" && !strings.Contains(c, "*") && !got[c] {
				out = append(out, fmt.Sprintf("-c%d %s", i+1, c))
			}
		}
	}
	return out
}

// missingItems 요청한 항목(ALL 이면 메타의 모든 항목) 중 결과에 한 행도 없는 항목 "코드 이름(단위)".
// 결과가 하나도 없으면(그 자체가 오류로 보고됨) 따지지 않는다.
func missingItems(metaItems []api.MetaResult, itemFlag string, rows []api.DataRow) []string {
	if len(rows) == 0 || itemFlag == "" {
		return nil
	}
	got := map[string]bool{}
	for _, r := range rows {
		got[r.ItmID] = true
	}
	name := map[string]string{}
	var want []string
	for _, m := range metaItems {
		n := m.ItmNM
		if m.UnitNM != "" {
			n += "(" + m.UnitNM + ")"
		}
		name[m.ItmID] = n
		if strings.EqualFold(itemFlag, "ALL") {
			want = append(want, m.ItmID)
		}
	}
	if !strings.EqualFold(itemFlag, "ALL") {
		for _, c := range strings.Split(itemFlag, "+") {
			if c = strings.TrimSpace(c); c != "" {
				want = append(want, c)
			}
		}
	}
	var out []string
	for _, c := range want {
		if !got[c] {
			out = append(out, strings.TrimSpace(c+" "+name[c]))
		}
	}
	return out
}

// joinValue -c1 A -c1 B 처럼 같은 플래그를 거듭 주면 "A+B" 로 합친다 (string 플래그는 마지막 값만 남겨 앞 코드가 조용히 사라졌다 — RSI R15).
type joinValue struct{ p *string }

func (j *joinValue) String() string {
	if j.p == nil {
		return ""
	}
	return *j.p
}

func (j *joinValue) Set(v string) error {
	if *j.p != "" && v != "" {
		v = *j.p + "+" + v
	}
	*j.p = v
	return nil
}

func (j *joinValue) Type() string { return "string" }

// childrenOfMissing 결과에 없는 코드("-c1 12") 중 하위 코드가 있는 것 — 상위(통합시 등)는 값이 없고 하위에 값이 있는 표.
func childrenOfMissing(summary *api.MetaSummaryResult, miss []string) []string {
	if summary == nil {
		return nil
	}
	axisN := map[string]int{}
	for _, a := range summary.Axes() {
		axisN[a.ObjID] = a.N
	}
	var out []string
	for _, m := range miss {
		var n int
		var code string
		if _, err := fmt.Sscanf(m, "-c%d %s", &n, &code); err != nil {
			continue
		}
		var names, codes []string
		for _, c := range summary.Classifications {
			if axisN[c.ObjID] == n && c.UpItmID == code {
				names = append(names, c.ItmID+" "+c.ItmNM)
				codes = append(codes, c.ItmID)
			}
		}
		if len(codes) > 0 && len(codes) <= 30 {
			out = append(out, fmt.Sprintf("-c%d %s 아래 코드: %s → -c%d \"%s\"", n, code, capList(names, 5), n, strings.Join(codes, "+")))
		}
	}
	return out
}

// expandLevels "-c1 level:2" (또는 "00+level:2") 의 level:N 을 그 축 N단계 코드로 바꾼다.
func expandLevels(summary *api.MetaSummaryResult, axisN int, v string) (string, error) {
	byLevel := map[int][]string{}
	flag := fmt.Sprintf("--class%d", axisN)
	for _, r := range summary.Codes() {
		if r.Flag == flag {
			byLevel[r.Level] = append(byLevel[r.Level], r.Code)
		}
	}
	var out []string
	drop := map[string]bool{}
	for _, tok := range strings.Split(v, "+") {
		tok = strings.TrimSpace(tok)
		if strings.HasPrefix(tok, "!") { // 뺄 코드 (level:1 에 섞인 동부·읍부·면부 등)
			drop[strings.TrimPrefix(tok, "!")] = true
			continue
		}
		low := strings.ToLower(tok)
		if !strings.HasPrefix(low, "level:") {
			out = append(out, tok)
			continue
		}
		n, err := strconv.Atoi(strings.TrimPrefix(low, "level:"))
		if err != nil || len(byLevel[n]) == 0 {
			var have []string
			for l := 1; l <= 20; l++ {
				if len(byLevel[l]) > 0 {
					have = append(have, fmt.Sprintf("level:%d(%d개)", l, len(byLevel[l])))
				}
			}
			return "", fmt.Errorf("이 축에 그 단계가 없습니다. 있는 단계: %s", strings.Join(have, ", "))
		}
		out = append(out, byLevel[n]...)
	}
	kept := out[:0]
	for _, c := range out {
		if !drop[c] {
			kept = append(kept, c)
		}
	}
	return strings.Join(kept, "+"), nil
}

// codeNames "a+b" 코드를 "a 이름" 목록으로 (level:N 이 무엇으로 바뀌었는지 보이게).
func codeNames(summary *api.MetaSummaryResult, axisN int, codes string) []string {
	name := map[string]string{}
	flag := fmt.Sprintf("--class%d", axisN)
	for _, r := range summary.Codes() {
		if r.Flag == flag {
			name[r.Code] = r.Name
		}
	}
	var out []string
	for _, c := range strings.Split(codes, "+") {
		out = append(out, strings.TrimSpace(c+" "+name[c]))
	}
	return out
}

// splitByMeta 결과에 없는 코드("-c1 12")를 표 메타에 있는 것과 없는 것으로 나눈다.
func splitByMeta(summary *api.MetaSummaryResult, miss []string) (inMeta, notInMeta []string) {
	have := map[string]bool{}
	for _, r := range summary.Codes() {
		have[strings.Replace(r.Flag, "--class", "-c", 1)+" "+r.Code] = true
	}
	for _, m := range miss {
		if have[m] {
			inMeta = append(inMeta, m)
		} else {
			notInMeta = append(notInMeta, m)
		}
	}
	return inMeta, notInMeta
}

// joinedUnit 단위 칸이 '명 건'·'농장 마리' 처럼 짧은 단위를 공백으로 이은 값이면 그 값 (표 전체 단위를 행마다 붙인 경우).
func joinedUnit(rows []api.DataRow) string {
	for _, r := range rows {
		f := strings.Fields(r.UnitNM)
		if len(f) < 2 {
			continue
		}
		short := !strings.Contains("천 만 십만 백만 천만 억 십억 백억 천억 조", f[0]) // '백만 달러' 는 한 단위
		for _, w := range f {
			short = short && utf8.RuneCountInString(w) <= 2
		}
		if short {
			return r.UnitNM
		}
	}
	return ""
}

// capList 목록이 길면 앞 n개와 '외 N개' (level:2 로 수백 코드를 줄 때 stderr 가 넘치지 않게).
func capList(l []string, n int) string {
	if len(l) <= n {
		return strings.Join(l, ", ")
	}
	return fmt.Sprintf("%s 외 %d개", strings.Join(l[:n], ", "), len(l)-n)
}

// keepChanged --change 결과에서 비교 기준으로만 받은 앞 시점 행(증감 없음)을 뺀다.
// 계열마다 가장 최근 시점 행은 증감이 없어도 남긴다 (신설 코드·X 값 — 조용히 지우지 않는다). noChange 는 그런 행.
func keepChanged(rows []api.DataRow, changes map[string][2]string) (kept, noChange []api.DataRow) {
	series := func(r api.DataRow) string {
		return strings.Join([]string{r.C1, r.C2, r.C3, r.C4, r.C5, r.C6, r.C7, r.C8, r.ItmID}, "|")
	}
	latest := map[string]string{}
	for _, r := range rows {
		if k := series(r); r.PrdDe > latest[k] {
			latest[k] = r.PrdDe
		}
	}
	for _, r := range rows {
		c, ok := changes[rowKey(r)]
		has := ok && c[0] != ""
		switch {
		case has:
			kept = append(kept, r)
		case r.PrdDe == latest[series(r)]:
			kept = append(kept, r)
			noChange = append(noChange, r)
		}
	}
	return kept, noChange
}

// allZeroItems 결과에서 값이 2개 이상이고 모두 0 인 항목 ("T3 보급률") — 미작성 항목을 0 으로 읽는 조용한 오답 방지.
func allZeroItems(rows []api.DataRow) []string {
	type st struct {
		name    string
		n       int
		nonZero bool
	}
	by := map[string]*st{}
	var order []string
	for _, r := range rows {
		v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(r.DT), ",", ""), 64)
		if err != nil {
			continue
		}
		x, ok := by[r.ItmID]
		if !ok {
			x = &st{name: r.ItmNM}
			by[r.ItmID] = x
			order = append(order, r.ItmID)
		}
		x.n++
		x.nonZero = x.nonZero || v != 0
	}
	var out []string
	for _, id := range order {
		if x := by[id]; x.n >= 2 && !x.nonZero {
			out = append(out, strings.TrimSpace(id+" "+x.name))
		}
	}
	return out
}

// rankRows 정렬된 행의 순위 (1,2,2,4 — 같은 값은 같은 순위, 숫자 아닌 값은 빈 칸). '한국 몇 위' 를 바로 읽게.
func rankRows(sorted []api.DataRow, val func(api.DataRow) string, sortGroup func(api.DataRow) string) map[string]string {
	out := map[string]string{}
	type st struct {
		n, rank int
		prev    float64
	}
	by := map[string]*st{} // 묶음(항목·시점)마다 순위
	for _, r := range sorted {
		g := by[sortGroup(r)]
		if g == nil {
			g = &st{}
			by[sortGroup(r)] = g
		}
		g.n++
		v, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(val(r)), ",", ""), 64)
		if err != nil {
			continue
		}
		if g.rank == 0 || v != g.prev {
			g.rank = g.n
		}
		g.prev = v
		out[rowKey(r)] = strconv.Itoa(g.rank)
	}
	return out
}

// futurePeriod 결과의 가장 늦은 시점이 지금보다 뒤면 그 시점 (추계·전망 표: IMF WEO 2031, 장래인구추계 2070).
func futurePeriod(rows []api.DataRow, now time.Time) string {
	last := ""
	for _, r := range rows {
		if r.PrdDe > last {
			last = r.PrdDe
		}
	}
	if len(last) < 4 {
		return ""
	}
	y, err := strconv.Atoi(last[:4])
	if err != nil {
		return ""
	}
	if y > now.Year() {
		return last
	}
	if y == now.Year() && len(last) == 6 { // 월: 올해 아직 오지 않은 달 (분기 202603 은 달로 봐도 지난 시점이라 오탐 없음)
		if m, err := strconv.Atoi(last[4:]); err == nil && m > int(now.Month()) {
			return last
		}
	}
	return ""
}

// extraClassFlags 표에 없는 축 번호에 준 -cN (축 정보가 없으면 판단하지 않는다).
func extraClassFlags(summary *api.MetaSummaryResult, given []string) []string {
	if len(summary.Axes()) == 0 {
		return nil
	}
	var out []string
	for i, g := range given {
		if i > 0 && g != "" && !summary.HasAxis(i+1) { // -c1 은 1번 축이 없는 표도 API 가 요구한다(ALL 로 채움)
			out = append(out, fmt.Sprintf("-c%d", i+1))
		}
	}
	return out
}

// normPrd 수록정보 시점("2026.07", "2026 2/4", "2025")을 d 시점 코드(202607, 202602, 2025)로.
func normPrd(v, cycle string) string {
	v = strings.TrimSpace(v)
	if cycle == "Q" || cycle == "H" || cycle == "S" {
		if f := strings.Fields(v); len(f) == 2 && len(f[1]) > 0 {
			return f[0] + "0" + f[1][:1]
		}
	}
	return strings.NewReplacer(".", "", " ", "", "/", "").Replace(v)
}

// periodsOutOfRange 요청 시점 중 그 주기 수록 범위 밖인 것 (안내 한 줄, 없으면 "").
func periodsOutOfRange(summary *api.MetaSummaryResult, prdSe string, req []string) string {
	c := api.NormalizePrdSe(strings.ToUpper(prdSe))
	for _, p := range summary.Periods {
		if api.NormalizePrdSe(p.PrdSe) != c {
			continue
		}
		lo, hi := normPrd(p.StrtPrdDe, c), normPrd(p.EndPrdDe, c)
		var out []string
		for _, r := range req {
			r = strings.TrimSpace(r)
			if r == "" || len(r) != len(hi) {
				continue
			}
			if r > hi || (len(lo) == len(r) && r < lo) {
				out = append(out, r)
			}
		}
		if len(out) > 0 {
			return fmt.Sprintf("요청 시점 %s 은 이 표 -p %s 수록 범위(%s ~ %s) 밖입니다 — 그 시점은 행이 없습니다 (코드 문제가 아님)", strings.Join(out, ", "), c, strings.TrimSpace(p.StrtPrdDe), strings.TrimSpace(p.EndPrdDe))
		}
	}
	return ""
}

// sameNameCodes 같은 축에서 이름은 같고 코드가 다른 것 ("군위군(27720·47720)" — 편입·개편 전후 코드).
func sameNameCodes(rows []api.DataRow) []string {
	type key struct {
		axis int
		name string
	}
	codes := map[key][]string{}
	var order []key
	for _, r := range rows {
		for i, p := range [][2]string{{r.C1, r.C1NM}, {r.C2, r.C2NM}, {r.C3, r.C3NM}, {r.C4, r.C4NM}, {r.C5, r.C5NM}, {r.C6, r.C6NM}, {r.C7, r.C7NM}, {r.C8, r.C8NM}} {
			if p[0] == "" || p[1] == "" {
				continue
			}
			k := key{i + 1, strings.TrimSpace(p[1])}
			if _, ok := codes[k]; !ok {
				order = append(order, k)
			}
			found := false
			for _, c := range codes[k] {
				found = found || c == p[0]
			}
			if !found {
				codes[k] = append(codes[k], p[0])
			}
		}
	}
	var out []string
	for _, k := range order {
		if len(codes[k]) > 1 {
			out = append(out, fmt.Sprintf("%s(%s)", k.name, strings.Join(codes[k], "·")))
		}
	}
	return out
}

// namesCarryUnit 분류·항목 이름이 '(명)'·'(천명당)' 처럼 단위를 괄호로 품고 있는지 — 그러면 빈 단위 칸 안내를 생략한다.
func namesCarryUnit(rows []api.DataRow) bool {
	for _, r := range rows {
		hit := false
		for _, n := range []string{r.ItmNM, r.C1NM, r.C2NM, r.C3NM, r.C4NM, r.C5NM, r.C6NM, r.C7NM, r.C8NM} {
			hit = hit || (strings.HasSuffix(strings.TrimSpace(n), ")") && strings.Contains(n, "("))
		}
		if !hit {
			return false
		}
	}
	return len(rows) > 0
}

// sameNameSuccessors 일찍 끊긴 코드("-c1 47720(~2022)")와 이름이 같은 같은 축 다른 코드 — 이어 붙일 후보.
func sameNameSuccessors(summary *api.MetaSummaryResult, ended []string) []string {
	rows := summary.Codes()
	var out []string
	for _, e := range ended {
		var n int
		var code string
		if _, err := fmt.Sscanf(e, "-c%d %s", &n, &code); err != nil {
			continue
		}
		code, _, _ = strings.Cut(code, "(")
		flag := fmt.Sprintf("--class%d", n)
		name := ""
		for _, r := range rows {
			if r.Flag == flag && r.Code == code {
				name = r.Name
			}
		}
		var others []string
		for _, r := range rows {
			if name != "" && r.Flag == flag && r.Code != code && r.Name == name {
				others = append(others, r.Code+" "+r.Path)
			}
		}
		if len(others) > 0 && len(others) <= 5 {
			out = append(out, fmt.Sprintf("-c%d %s(%s) 와 이름이 같은 코드: %s — 이어 보려면 -c%d 에 함께 넣으세요", n, code, name, strings.Join(others, ", "), n))
		}
	}
	return out
}

// expectedLast 이 조회가 닿아야 할 마지막 시점: 표의 그 주기 끝, -e·--periods 가 더 이르면 그것.
func expectedLast(summary *api.MetaSummaryResult, prdSe, end, periods string) string {
	c := api.NormalizePrdSe(strings.ToUpper(prdSe))
	last := ""
	for _, p := range summary.Periods {
		if api.NormalizePrdSe(p.PrdSe) == c {
			last = normPrd(p.EndPrdDe, c)
		}
	}
	if periods != "" {
		maxP := ""
		for _, p := range strings.Split(periods, ",") {
			if p = strings.TrimSpace(p); p > maxP {
				maxP = p
			}
		}
		end = maxP
	}
	if end != "" && len(end) == len(last) && end < last {
		return end
	}
	return last
}

// withoutCoveredParents 결과에 없는 코드 중, 하위 코드를 이미 모두 함께 준 상위 코드(12 + 1224+1236)는 뺀다.
func withoutCoveredParents(summary *api.MetaSummaryResult, given, miss []string) []string {
	if summary == nil {
		return miss
	}
	giv := map[string]bool{}
	for i, g := range given {
		for _, c := range strings.Split(g, "+") {
			giv[fmt.Sprintf("-c%d %s", i+1, strings.TrimSpace(c))] = true
		}
	}
	axisN := map[string]int{}
	for _, a := range summary.Axes() {
		axisN[a.ObjID] = a.N
	}
	var out []string
	for _, m := range miss {
		var n int
		var code string
		if _, err := fmt.Sscanf(m, "-c%d %s", &n, &code); err != nil {
			out = append(out, m)
			continue
		}
		kids, covered := 0, true
		for _, c := range summary.Classifications {
			if axisN[c.ObjID] == n && c.UpItmID == code {
				kids++
				covered = covered && giv[fmt.Sprintf("-c%d %s", n, c.ItmID)]
			}
		}
		if kids == 0 || !covered {
			out = append(out, m)
		}
	}
	return out
}

// lastPeriod 결과의 가장 늦은 시점.
func lastPeriod(rows []api.DataRow) string {
	last := ""
	for _, r := range rows {
		if r.PrdDe > last {
			last = r.PrdDe
		}
	}
	return last
}

// multiCodeAxes 결과에서 코드가 둘 이상인 분류 축 ("c1", "c2").
func multiCodeAxes(rows []api.DataRow) []string {
	var out []string
	for i := 1; i <= 8; i++ {
		seen := map[string]bool{}
		for _, r := range rows {
			seen[r.GetField(fmt.Sprintf("C%d", i))] = true
		}
		delete(seen, "")
		if len(seen) > 1 {
			out = append(out, fmt.Sprintf("c%d", i))
		}
	}
	return out
}
