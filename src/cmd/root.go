package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/clazic/kosis/internal/tui"
)

var appVersion = "dev"

const rootHelpText = `KOSIS CLI - 한국 통계 데이터 조회 도구

KOSIS(국가통계포털) Open API 기반 CLI/TUI 도구입니다.
인자 없이 실행하면 TUI 대시보드가 열립니다.

사용법:
  kosis [command]

통계표 명령어:
  search  (s)       통계표 키워드 검색
  meta    (m)       통계표 메타데이터 조회 (분류/항목/수록정보)
  data    (d)       통계표 데이터 조회
  list    (ls)      통계목록 트리 탐색
  explain (ex)      통계 조사 설명
  bulk              대용량 통계자료 다운로드 (SDMX/XLS)

주요지표 명령어:
  indicator (ind)   통계주요지표 검색/조회 (1,473개 핵심 지표)

시각화 명령어:
  chart             데이터를 차트로 시각화 (터미널/PNG/SVG/PDF/HTML/Excel/Mermaid)

편의 명령어:
  quick    (q)      자연어로 원스텝 조회 (규칙 기반 또는 AI)
  config            설정 관리 (API 키, AI 도구, 기본 포맷)
  bookmark (bm)     즐겨찾기 관리
  history  (hi)     조회 이력 관리
  completion        셸 자동완성 설정
  update            최신 버전으로 업데이트 (바이너리+스킬)

플래그:
  -v, --version             버전 정보
  -h, --help                도움말

시작하기:
  # 1. API 키 설정 (https://kosis.kr/openapi/ 에서 발급)
  kosis config set-key <YOUR_API_KEY>

  # 2. 통계표 검색
  kosis s "인구"

  # 3. 메타데이터 확인 (분류/항목 코드 파악)
  kosis m 101 DT_1IN1502

  # 4. 데이터 조회
  kosis d 101 DT_1IN1502 -c1 "11" -i T100 -p Y -l 5

  # 또는 자연어 한 줄로
  kosis q "서울 미분양 최근 6개월"

표준 워크플로우 (단계별 설명):
  1단계. 통계표 찾기 — kosis search "<키워드>"
         통계표 이름으로 검색하여 ORG_ID(기관 코드)와 TBL_ID(통계표 ID)를
         확보합니다. 이 두 값이 이후 모든 명령의 기본 인자입니다.
         키워드가 떠오르지 않으면 kosis ls로 주제 트리를 훑어 내려가세요.

  2단계. 코드 파악 — kosis meta <ORG_ID> <TBL_ID>
         통계표가 어떤 분류(지역·산업 등)와 항목(인구수·증감률 등)으로
         구성되는지, 어떤 주기(년/월/분기)로 수록되는지 확인합니다.
         출력의 [분류]는 -c1~-c8에, [항목]은 -i에, prdSe는 -p에 각각
         대응합니다. 코드는 반드시 숫자·영문 코드로 넘겨야 하며
         한글 이름을 쓰면 에러 21이 발생합니다.

  3단계. 시점 확인 — kosis data <ORG_ID> <TBL_ID> ... -l 5
         최근 5개 시점만 먼저 뽑아 값이 정상인지, 시점 표기 형식이
         맞는지 확인합니다. 여기서 틀린 코드를 바로잡고 넘어가세요.

  4단계. 본 조회 — kosis data <ORG_ID> <TBL_ID> ... -s <시작> -e <종료>
         범위를 넓혀 실제 데이터를 가져옵니다. 4만 셀을 넘으면 자동으로
         분할 조회하며, -o로 파일 저장 / --chart로 시각화까지 한 번에
         처리할 수 있습니다.

  (빠른 길) 위 4단계를 자동으로 수행: kosis quick "서울 미분양 최근 6개월"
  (빠른 길) 핵심 지표만 필요하면 통계표 대신: kosis ind d "GDP"

더 알아보기:
  kosis <command> --help     각 명령어 상세 도움말
  kosis data --help          통계표 조회 파라미터 확인
  kosis indicator --help     주요지표 명령어 확인
`

func SetVersion(v string) {
	appVersion = v
}

// skipUpdateCheckCmds update/version/config 명령은 자동 업데이트 체크 스킵
var skipUpdateCheckCmds = map[string]bool{
	"update":  true,
	"version": true,
	"config":  true,
}

var rootCmd = &cobra.Command{
	Use:     "kosis",
	Short:   "KOSIS CLI - 한국 통계 데이터 조회 도구",
	Long:    rootHelpText,
	Version: appVersion,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		// update/version/config 명령 및 비대화형은 스킵
		name := cmd.Name()
		if skipUpdateCheckCmds[name] {
			return
		}
		startBackgroundUpdateCheck()
	},
	PersistentPostRun: func(cmd *cobra.Command, args []string) {
		name := cmd.Name()
		if skipUpdateCheckCmds[name] {
			return
		}
		printUpdateNotice()
	},
	Run: func(cmd *cobra.Command, args []string) {
		runDashboard(cmd)
	},
}

func Execute() {
	rootCmd.Version = appVersion
	rootCmd.SetArgs(normalizeClassShortFlags(os.Args[1:]))
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func runDashboard(cmd *cobra.Command) {
	// Non-interactive mode: do not block on stdin.
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Println("KOSIS 대시보드 (비대화형)")
		fmt.Println()
		fmt.Println("빠른 시작:")
		fmt.Println("  1) API 키 설정")
		fmt.Println("     kosis config set-key <YOUR_API_KEY>")
		fmt.Println("  2) 통계표 검색")
		fmt.Println("     kosis s \"인구\"")
		fmt.Println("  3) 통계표 데이터 조회")
		fmt.Println("     kosis d 101 DT_1IN1502 -c1 \"11\" -i T100 -p Y -l 5")
		fmt.Println("  4) 주요지표 조회")
		fmt.Println("     kosis ind d \"GDP\"")
		fmt.Println()
		fmt.Println("대화형 선택은 터미널에서 `kosis`를 실행하면 사용할 수 있습니다.")
		fmt.Println("바로 도움말: kosis --help")
		return
	}

	// TUI 대시보드 시작
	if err := tui.StartTUI(); err != nil {
		fmt.Printf("TUI 오류: %v\n", err)
		os.Exit(1)
	}
}

func rootHelpFunc(defaultHelp func(*cobra.Command, []string)) func(*cobra.Command, []string) {
	return func(cmd *cobra.Command, args []string) {
		if cmd == rootCmd {
			fmt.Fprint(cmd.OutOrStdout(), rootHelpText)
			return
		}
		defaultHelp(cmd, args)
	}
}

func normalizeClassShortFlags(args []string) []string {
	if len(args) == 0 {
		return args
	}

	out := make([]string, 0, len(args))
	for _, arg := range args {
		switch {
		case arg == "-c1":
			out = append(out, "--class1")
		case arg == "-c2":
			out = append(out, "--class2")
		case arg == "-c3":
			out = append(out, "--class3")
		case arg == "-c4":
			out = append(out, "--class4")
		case arg == "-c5":
			out = append(out, "--class5")
		case arg == "-c6":
			out = append(out, "--class6")
		case arg == "-c7":
			out = append(out, "--class7")
		case arg == "-c8":
			out = append(out, "--class8")
		case strings.HasPrefix(arg, "-c1="):
			out = append(out, "--class1="+strings.TrimPrefix(arg, "-c1="))
		case strings.HasPrefix(arg, "-c2="):
			out = append(out, "--class2="+strings.TrimPrefix(arg, "-c2="))
		case strings.HasPrefix(arg, "-c3="):
			out = append(out, "--class3="+strings.TrimPrefix(arg, "-c3="))
		case strings.HasPrefix(arg, "-c4="):
			out = append(out, "--class4="+strings.TrimPrefix(arg, "-c4="))
		case strings.HasPrefix(arg, "-c5="):
			out = append(out, "--class5="+strings.TrimPrefix(arg, "-c5="))
		case strings.HasPrefix(arg, "-c6="):
			out = append(out, "--class6="+strings.TrimPrefix(arg, "-c6="))
		case strings.HasPrefix(arg, "-c7="):
			out = append(out, "--class7="+strings.TrimPrefix(arg, "-c7="))
		case strings.HasPrefix(arg, "-c8="):
			out = append(out, "--class8="+strings.TrimPrefix(arg, "-c8="))
		default:
			out = append(out, arg)
		}
	}

	return out
}

func init() {
	defaultHelp := rootCmd.HelpFunc()
	rootCmd.SetHelpFunc(rootHelpFunc(defaultHelp))
}
