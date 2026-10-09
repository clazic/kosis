package cmd

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/nlp"
	"github.com/spf13/cobra"
)

var quickFailureRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?m)^오류:`),
	regexp.MustCompile(`(?m)^Error:`),
	regexp.MustCompile(`(?m)^검색 실패:`),
	regexp.MustCompile(`(?m)^조회 실패:`),
	regexp.MustCompile(`(?m)^포맷(팅)? 오류:`),
	regexp.MustCompile(`(?m)unknown shorthand flag`),
	regexp.MustCompile(`(?m)invalid argument`),
}

var quickCmd = &cobra.Command{
	Use:     "quick <사용자요청>",
	Aliases: []string{"q"},
	Short:   "자연어로 통계 조회 (규칙 기반 또는 AI)",
	Long: `KOSIS 원스텝 조회

자연어로 통계 데이터를 한 번에 조회합니다.
내부적으로 검색 -> 메타 확인 -> 데이터 조회를 자동 수행합니다.

기본: 규칙 기반 키워드 매칭 (오프라인 동작)
--ai 플래그 사용 시: AI가 실제 검색·메타 목록에서 통계표와 코드를 골라 조회 (사전 밖 통계도 가능)

사용법:
  kosis quick "<자연어 요청>" [flags]
  kosis q "<자연어 요청>"
  kosis q                              대화형 모드

플래그:
  -a, --ai[=<공급자>]      AI 조회. 값 생략 시 기본 글생성 공급자(hermes)
  -f, --format <type>      출력 형식: table(기본), json, csv, md
  -o, --output <파일>      파일 저장
  -y, --yes                확인 프롬프트 없이 바로 실행 (스크립트·AI 에이전트용)

예제:
  # 규칙 기반 (기본)
  kosis q "서울 미분양 최근 6개월"
  kosis q "GDP 2020~2024"
  kosis q "소비자물가 월별"
  kosis q "전국 인구 최근 5년"

  # AI 사용 (기본 공급자 hermes)
  kosis q "작년까지 최근 5년간 울산에서 태어난 아기 수" --ai
  kosis q "실업률 추세" --ai openai        # ai-add로 등록한 공급자

  # 확인 없이 바로 실행
  kosis q "서울 미분양 최근 6개월" --yes

  # 파일로 저장
  kosis q "GDP 2020~2024" -o gdp.xlsx

  # 대화형 모드
  kosis q

인식 가능한 패턴:
  지역:    서울, 부산, 대구, 인천, 광주, 대전, 울산, 세종, 경기 ...
  기간:    "최근 N개월/년", "2020~2024", "2020,2022,2025", "월별", "연별"
  통계:    미분양, GDP, 물가, 인구, 실업률 등 주요 키워드

AI 모드 설명:
  AI는 코드를 만들지 않고 실제 KOSIS 목록에서 고르기만 합니다 (지어낸 코드가 나가지 않음).
    ① 해석  hermes(글생성): 요청 → 검색어·조건·기간 (실패 시 규칙 기반으로 대신 해석)
    ② 검색  kosis search 로 후보 통계표
    ③ 선택  Jev(TypeSafe): 후보 중 통계표 1개
    ④ 메타  kosis meta 로 실제 분류·항목·주기
    ⑤ 선택  Jev: 축마다 코드 1개
    ⑥ 실행  확인 후 kosis data (--yes로 생략)
  각 선택에는 출처가 붙습니다: [Jev 신뢰도] [학습] [요청] [유일] [교정]. 신뢰도 0.3 미만은 ⚠ 표시.

학습 (~/.kosis/learnings.yaml):
  사람이 y 로 확인하고 결과가 나온 선택만 학습해, 다음 같은 요청(키워드+조건)에 바로 씁니다(검색·Jev 생략).
  --yes 실행, 신뢰도 0.3 미만 선택, 결과 0행은 학습하지 않습니다.
  통계표 탓이 분명한 실행 오류([20]·[21])는 그 표를 피합니다. [30](데이터 없음) 등은 학습하지 않습니다.
  실행 확인에서 n 을 누르면 무엇이 틀렸는지(통계표 / 분류·항목·주기 값) 묻고, 고른 정답을 학습합니다.
  학습된 표가 폐지되는 등 쓸 수 없게 되면 자동으로 피하고 다시 검색합니다. 초기화는 파일 삭제.

문서 자동 교정:
  사람이 교정한 통계표가 성공하면, 설치된 스킬 문서(SKILL.md·references)에서 그 키워드와 틀린 표가
  같은 줄에 있는 곳을 맞는 표로 고치고(원본은 숨김 백업 .<파일>.bak), LEARNINGS.md 오답노트에 기록합니다.
  kosis update 는 스킬 파일을 덮어쓰므로 영구 반영은 저장소 문서 수정이 필요합니다.

외부 전송:
  --ai 사용 시 요청 문장이 글생성 공급자(기본 hermes)와 Jev(api.typesafe.ai)로 전송됩니다.

API 키:
  kosis config ai-key hermes <KEY>    또는 환경변수 HERMES_API_KEY  (글생성)
  kosis config ai-key jev <KEY>       또는 환경변수 TYPESAFE_API_KEY (선택, https://typesafe.ai)

AI 공급자 관리:
  kosis config ai-list                          등록된 공급자 목록
  kosis config ai-add openai --url https://api.openai.com/v1 --model gpt-4o-mini --key-env OPENAI_API_KEY
  kosis config set-ai <공급자>                  기본 공급자 설정
  kosis config ai-remove <공급자>

관련 명령어:
  kosis config set-ai <공급자>     기본 AI 공급자 설정
  kosis data ...                   세밀한 파라미터 지정 조회`,

	Example: `  # 규칙 기반 조회
  kosis quick "서울 미분양 최근 6개월"

  # AI 사용 (기본 공급자 hermes)
  kosis quick "GDP 최근 5년 추이" --ai

  # 등록한 다른 공급자 사용
  kosis quick "인구 통계" --ai openai

  # 대화형 모드
  kosis quick`,

	Run: func(cmd *cobra.Command, args []string) {
		aiFlag, _ := cmd.Flags().GetString("ai")
		useAI := cmd.Flags().Changed("ai")
		if aiFlag == aiDefaultSentinel {
			aiFlag = "" // 기본 공급자
		}
		formatFlag, _ := cmd.Flags().GetString("format")
		outputFlag, _ := cmd.Flags().GetString("output")

		// 대화형 모드: 인자가 없으면 사용자 입력 받음
		var userInput string
		if len(args) == 0 {
			userInput = promptUserInput()
			if userInput == "" {
				return
			}
		} else {
			// CLI 모드: 인자를 사용자 요청으로 사용
			userInput = strings.Join(args, " ")
		}

		// API 키 확인
		if !config.HasAPIKey() {
			fmt.Println(config.NoAPIKeyMessage())
			return
		}

		// AI 도구를 사용하는 경우
		if useAI {
			handleAIGeneration(aiFlag, userInput, formatFlag, outputFlag)
			return
		}

		handleRuleBasedMatching(userInput, formatFlag, outputFlag)
	},
}

// init은 quick 명령어를 root에 등록하고 플래그를 추가합니다.
func init() {
	rootCmd.AddCommand(quickCmd)

	quickCmd.Flags().StringP("ai", "a", "", "AI 조회(hermes 해석 + Jev 선택 + 학습). 값 생략(--ai) 시 기본 글생성 공급자(hermes), --ai <이름>으로 글생성 공급자 지정. 미사용 시 오프라인 규칙 기반 매칭. 목록은 kosis config ai-list")
	quickCmd.Flags().Lookup("ai").NoOptDefVal = aiDefaultSentinel
	quickCmd.Flags().StringP("format", "f", "", "출력 형식: table, json, csv, md. 미지정 시 config의 기본 형식 사용")
	quickCmd.Flags().StringP("output", "o", "", "파일로 저장. 확장자로 형식 자동 결정: .csv .xlsx .json .db .parquet")
	quickCmd.Flags().BoolVarP(&quickAssumeYes, "yes", "y", false, "확인 프롬프트 없이 바로 실행 (스크립트·AI 에이전트처럼 입력을 받을 수 없는 환경용)")
}

// quickAssumeYes는 --yes 플래그 값. 비대화형 환경에선 프롬프트가 EOF로 끝나 조회가 취소되므로 건너뛸 수단이 필요하다.
var quickAssumeYes bool

// aiDefaultSentinel 값 없이 --ai만 줬을 때의 플래그 값 (= 기본 공급자 사용).
const aiDefaultSentinel = "default"

// normalizeQuickAIFlag `quick ... --ai hermes`처럼 공백으로 준 공급자명을 `--ai=hermes`로 합친다.
// --ai는 값 생략이 가능한 플래그라 pflag가 다음 인자를 값으로 먹지 않기 때문이다.
// 등록된 공급자명일 때만 합치므로 `--ai "요청문"` 같은 입력을 잘못 삼키지 않는다.
func normalizeQuickAIFlag(args []string) []string {
	if len(args) == 0 || (args[0] != "quick" && args[0] != "q") {
		return args
	}
	var providers map[string]config.AIProvider
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		a := args[i]
		if (a == "--ai" || a == "-a") && i+1 < len(args) {
			if providers == nil {
				if cfg, err := config.Load(); err == nil {
					providers = cfg.AI.Providers
				} else {
					providers = map[string]config.AIProvider{}
				}
			}
			if _, ok := providers[args[i+1]]; ok {
				out = append(out, "--ai="+args[i+1])
				i++
				continue
			}
		}
		out = append(out, a)
	}
	return out
}

// handleRuleBasedMatching은 규칙 기반으로 명령어를 매칭하고 실행합니다.
func handleRuleBasedMatching(userRequest, formatFlag, outputFlag string) {
	match := nlp.Match(userRequest)

	// 바로가기 매칭이 되면 data 명령으로 즉시 실행
	if match.Matched {
		dataArgs, err := buildDataArgsFromMatch(match, formatFlag, outputFlag)
		if err != nil {
			printQuickFailure("규칙 기반", err.Error(), suggestionsForBuildFailure(match))
			return
		}
		displayCmd := "kosis data " + quoteArgs(dataArgs)

		fmt.Printf("🧭 규칙 기반 명령어:\n  %s\n\n", displayCmd)
		if !confirmExecution() {
			fmt.Println("실행 취소됨")
			return
		}

		if err := runSubcommand("data", dataArgs); err != nil {
			fmt.Println("❌ quick 판정: 실패")
			fmt.Printf("원인: %v\n", err)
			fmt.Println("다음 액션: kosis meta <ORG_ID> <TBL_ID>로 코드 확인 후 data를 직접 실행하세요.")
			return
		}

		fmt.Println("✅ quick 판정: 성공")
		printReusableCommand(displayCmd)
		return
	}

	// 바로가기 미매칭 시 search로 안전하게 fallback
	keyword := strings.TrimSpace(match.Keyword)
	if keyword == "" {
		keyword = strings.TrimSpace(userRequest)
	}
	if keyword == "" {
		printQuickFailure("규칙 기반", "검색 키워드를 해석하지 못했습니다.", []string{
			"예: kosis quick \"서울 미분양 최근 6개월\"",
			"또는 kosis search \"<키워드>\"로 직접 조회",
		})
		return
	}

	searchArgs := []string{keyword}
	if formatFlag != "" {
		searchArgs = append(searchArgs, "-f", formatFlag)
	}
	displayCmd := "kosis search " + quoteArgs(searchArgs)

	fmt.Printf("🔎 규칙 기반 바로가기 매칭 실패, 검색으로 전환합니다:\n  %s\n\n", displayCmd)
	if !confirmExecution() {
		fmt.Println("실행 취소됨")
		return
	}

	if err := runSubcommand("search", searchArgs); err != nil {
		fmt.Println("❌ quick 판정: 실패")
		fmt.Printf("원인: %v\n", err)
		fmt.Println("다음 액션: 키워드를 단순화해서 search를 다시 실행하세요.")
		return
	}

	fmt.Println("✅ quick 판정: 성공")
	printReusableCommand(displayCmd)
}

// promptUserInput은 대화형 모드에서 사용자 입력을 받습니다.
func promptUserInput() string {
	fmt.Print("통계 조회 요청을 입력하세요 (취소: q): ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input == "q" || input == "quit" {
			fmt.Println("입력이 취소되었습니다.")
			return ""
		}
		if input != "" {
			return input
		}
	}
	return ""
}

// confirmExecution은 생성된 명령어 실행 여부를 사용자에게 확인합니다.
func confirmExecution() bool {
	if quickAssumeYes {
		fmt.Println("확인 없이 실행합니다 (--yes)")
		return true
	}
	fmt.Print("이 명령어를 실행하시겠습니까? (y/n): ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return response == "y" || response == "yes" || response == "예" || response == "ㅇ"
	}
	return false
}

// buildDataArgsFromMatch converts rule-based match output into kosis data args.
// 사전에 없는 값을 기본값으로 지어내지 않는다. 모르면 에러를 반환한다 —
// 틀린 코드로 조회하면 조용한 오답(엉뚱한 분류의 수치)이 나올 수 있기 때문이다.
func buildDataArgsFromMatch(match *nlp.MatchResult, formatFlag, outputFlag string) ([]string, error) {
	sc := match.Shortcut

	args := []string{match.OrgID, match.TblID}

	// [1] 고정 축 (미분양의 대분류, GDP의 계정항목 등)
	for _, flag := range sortedKeys(sc.Fixed) {
		args = append(args, flag, sc.Fixed[flag])
	}

	// [2] 지역 축
	region := match.RegionName
	switch {
	case sc.Region == nil && region != "" && region != "전국":
		// "전국"은 지역 필터가 아니라 수사이므로 통과시키지만, 그 외 지역명은 적용할 수 없다.
		return nil, fmt.Errorf("%s(%s %s)는 지역별 분류를 제공하지 않습니다. '%s'을(를) 적용할 수 없습니다",
			sc.Label, sc.OrgID, sc.TblID, region)
	case sc.Region != nil:
		name := region
		if name == "" {
			name = "전국"
		}
		code, ok := sc.Region.Codes[name]
		if !ok {
			return nil, fmt.Errorf("%s(%s %s)의 '%s' 지역 코드가 사전에 없습니다. kosis meta %s %s로 확인하세요",
				sc.Label, sc.OrgID, sc.TblID, name, sc.OrgID, sc.TblID)
		}
		args = append(args, sc.Region.Flag, code)
	}

	// [3] 항목
	if sc.Item == "" {
		return nil, fmt.Errorf("%s(%s %s)의 항목 코드가 사전에 없습니다. kosis meta %s %s로 확인하세요",
			sc.Label, sc.OrgID, sc.TblID, sc.OrgID, sc.TblID)
	}
	args = append(args, "--item", sc.Item)

	// [4] 주기 — 통계표가 제공하지 않는 주기는 거부한다.
	period, err := resolvePeriod(sc, match.Period)
	if err != nil {
		return nil, err
	}
	args = append(args, "--period", period)

	switch {
	case strings.TrimSpace(match.Periods) != "":
		args = append(args, "--periods", match.Periods)
	case strings.TrimSpace(match.Start) != "" && strings.TrimSpace(match.End) != "":
		args = append(args, "--start", match.Start, "--end", match.End)
	case match.Latest > 0:
		args = append(args, "--latest", strconv.Itoa(match.Latest))
	default:
		args = append(args, "--latest", "1")
	}

	if formatFlag != "" {
		args = append(args, "-f", formatFlag)
	}
	if outputFlag != "" {
		args = append(args, "-o", outputFlag)
	}

	return args, nil
}

// suggestionsForBuildFailure 사전 정보 부족으로 조회를 포기했을 때의 다음 액션.
func suggestionsForBuildFailure(match *nlp.MatchResult) []string {
	sc := match.Shortcut
	actions := make([]string, 0, 3)

	// 지역 미지원 표에 지역을 요청한 경우: 전국 조회로 안내.
	// 지역·기간 토큰이 모두 소비되어 Keyword가 빌 수 있으므로 매칭 토큰을 기준으로 만든다.
	if sc.Region == nil && match.RegionName != "" && match.RegionName != "전국" {
		subject := strings.TrimSpace(match.MatchedToken + " " + match.Keyword)
		actions = append(actions,
			fmt.Sprintf("지역 없이 조회: kosis quick %q", subject),
			fmt.Sprintf("지역별 통계표를 직접 찾기: kosis search %q", match.MatchedToken+" 지역"))
	}
	if len(sc.Periods) > 0 {
		actions = append(actions,
			fmt.Sprintf("이 통계표가 제공하는 주기: %s", strings.Join(sc.Periods, ", ")))
	}
	if sc.OrgID != "" && sc.TblID != "" {
		actions = append(actions,
			fmt.Sprintf("분류·항목·주기 확인: kosis meta %s %s", sc.OrgID, sc.TblID))
	}
	return actions
}

// resolvePeriod 통계표가 지원하는 주기인지 확인하고 확정한다.
// 사용자가 지정하지 않았으면 사전의 첫 번째(대표) 주기를 쓴다.
func resolvePeriod(sc nlp.TableShortcut, requested string) (string, error) {
	if len(sc.Periods) == 0 {
		return "", fmt.Errorf("%s(%s %s)의 수록주기가 사전에 없습니다. kosis meta %s %s --type PRD로 확인하세요",
			sc.Label, sc.OrgID, sc.TblID, sc.OrgID, sc.TblID)
	}
	if requested == "" {
		return sc.Periods[0], nil
	}
	for _, p := range sc.Periods {
		if p == requested {
			return requested, nil
		}
	}
	return "", fmt.Errorf("%s(%s %s)는 '%s' 주기를 제공하지 않습니다 (지원: %s)",
		sc.Label, sc.OrgID, sc.TblID, requested, strings.Join(sc.Periods, ", "))
}

// sortedKeys 맵 순회 순서를 고정해 생성되는 명령어가 실행마다 달라지지 않게 한다.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func runSubcommand(subcommand string, args []string) error {
	_, err := runSubcommandOutput(subcommand, args)
	return err
}

// runSubcommandOutput 하위 명령을 실행해 화면에 출력하고, 출력 내용도 돌려준다 (결과 행 판정용).
func runSubcommandOutput(subcommand string, args []string) (string, error) {
	exePath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("실행 파일 경로 확인 실패: %w", err)
	}

	allArgs := append([]string{subcommand}, args...)
	cmd := exec.Command(exePath, allArgs...)
	cmd.Stdin = os.Stdin
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	if err := cmd.Run(); err != nil {
		if out.Len() > 0 {
			fmt.Print(out.String())
		}
		return out.String(), fmt.Errorf("%s 실행 실패(비정상 종료): %w", subcommand, err)
	}

	output := out.String()
	if output != "" {
		fmt.Print(output)
	}
	if failLine := firstFailureLine(output); failLine != "" {
		return output, fmt.Errorf("%s 실행 실패: %s", subcommand, failLine)
	}
	return output, nil
}

func printReusableCommand(command string) {
	fmt.Printf("\n💡 다음에 같은 조회:\n  %s\n", command)
}

func quoteArgs(args []string) string {
	var quoted []string
	for _, arg := range args {
		if strings.ContainsAny(arg, " \t\"") {
			escaped := strings.ReplaceAll(arg, "\"", "\\\"")
			quoted = append(quoted, "\""+escaped+"\"")
			continue
		}
		quoted = append(quoted, arg)
	}
	return strings.Join(quoted, " ")
}

func firstFailureLine(output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	lines := strings.Split(trimmed, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		for _, re := range quickFailureRegexes {
			if re.MatchString(line) {
				return line
			}
		}
	}
	return ""
}

func printQuickFailure(stage, cause string, actions []string) {
	fmt.Println("❌ quick 판정: 실패")
	if stage != "" {
		fmt.Printf("단계: %s\n", stage)
	}
	fmt.Printf("원인: %s\n", cause)
	if len(actions) > 0 {
		fmt.Println("다음 액션:")
		for i, action := range actions {
			fmt.Printf("  %d) %s\n", i+1, action)
		}
	}
}
