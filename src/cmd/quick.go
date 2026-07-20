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
--ai 플래그 사용 시: 외부 AI CLI 도구로 명령어 생성

사용법:
  kosis quick "<자연어 요청>" [flags]
  kosis q "<자연어 요청>"
  kosis q                              대화형 모드

플래그:
  --ai <도구명>            AI 도구 사용 (claude, gemini, codex 또는 커스텀)
  -f, --format <type>      출력 형식: table(기본), json, csv, md
  -o, --output <파일>      파일 저장

예제:
  # 규칙 기반 (기본)
  kosis q "서울 미분양 최근 6개월"
  kosis q "GDP 2020~2024"
  kosis q "소비자물가 월별"
  kosis q "전국 인구 최근 5년"

  # AI 사용
  kosis q "서울과 부산의 미분양 추이 비교" --ai claude
  kosis q "실업률 추세" --ai ollama

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
  --ai 플래그를 사용하면 외부 AI CLI 도구가 자연어를 분석하여
  kosis data 명령어를 자동 생성합니다. 생성된 명령어는 실행 전
  확인 프롬프트를 표시합니다.

AI 도구 관리:
  kosis config ai-list                   등록된 도구 목록
  kosis config set-ai claude             기본 AI 도구 설정
  kosis config ai-add ollama "ollama run llama3 '{prompt}'"
  kosis config ai-remove <도구명>

관련 명령어:
  kosis config set-ai <도구>       기본 AI 도구 설정
  kosis data ...                   세밀한 파라미터 지정 조회`,

	Example: `  # 규칙 기반 조회
  kosis quick "서울 미분양 최근 6개월"

  # Claude AI 사용
  kosis quick "GDP 최근 5년 추이" --ai claude

  # 커스텀 AI 도구 사용
  kosis quick "인구 통계" --ai ollama

  # 대화형 모드
  kosis quick`,

	Run: func(cmd *cobra.Command, args []string) {
		aiFlag, _ := cmd.Flags().GetString("ai")
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
		if aiFlag != "" {
			handleAIGeneration(aiFlag, userInput, formatFlag, outputFlag)
			return
		}

		handleRuleBasedMatching(userInput, formatFlag, outputFlag)
	},
}

// init은 quick 명령어를 root에 등록하고 플래그를 추가합니다.
func init() {
	rootCmd.AddCommand(quickCmd)

	quickCmd.Flags().StringP("ai", "a", "", "자연어 해석에 쓸 외부 AI CLI 도구 (claude, gemini, codex, ollama 등). 미지정 시 오프라인 규칙 기반 매칭. 목록은 kosis config ai-list")
	quickCmd.Flags().StringP("format", "f", "", "출력 형식: table, json, csv, md. 미지정 시 config의 기본 형식 사용")
	quickCmd.Flags().StringP("output", "o", "", "파일로 저장. 확장자로 형식 자동 결정: .csv .xlsx .json .db .parquet")
}

// handleAIGeneration은 AI 도구를 사용하여 명령어를 생성하고 실행합니다.
func handleAIGeneration(aiToolName, userRequest, formatFlag, outputFlag string) {
	cfg, err := config.Load()
	if err != nil {
		printQuickFailure("AI 경로", fmt.Sprintf("설정 로드 실패: %v", err), []string{
			"kosis config show 로 설정 파일 상태 확인",
			"환경변수 KOSIS_API_KEY 또는 ~/.kosis/config.yaml 점검",
		})
		return
	}

	// 지정된 AI 도구의 명령어 가져오기
	aiTool, exists := cfg.AI.Tools[aiToolName]
	if !exists {
		printQuickFailure("AI 경로", fmt.Sprintf("AI 도구 '%s'가 설정되지 않았습니다.", aiToolName), []string{
			"kosis config ai-list",
			"kosis config ai-add <이름> \"<명령어 '{prompt}' 포함>\"",
		})
		fmt.Println("등록된 도구 목록:")
		tools, _ := config.ListAITools()
		for _, t := range tools {
			installed := "미설치"
			if t.Installed {
				installed = "설치됨"
			}
			fmt.Printf("  - %s (%s)\n", t.Name, installed)
		}
		return
	}

	// AI 명령어 생성
	fmt.Printf("🤖 %s로 명령어 생성 중...\n", aiToolName)
	result, err := nlp.GenerateCommand(aiToolName, aiTool.Cmd, userRequest, nlp.GetSKILLContent())
	if err != nil {
		printQuickFailure("AI 경로", fmt.Sprintf("명령어 생성 실패: %v", err), []string{
			"kosis config ai-list 로 도구 설치 상태 확인",
			"kosis config ai-add ... 명령 템플릿에 '{prompt}' 포함 여부 확인",
			"동일 요청을 규칙 기반으로 재시도: kosis quick \"<요청>\"",
		})
		return
	}

	if result.Command == "" {
		printQuickFailure("AI 경로", "AI 응답에서 실행 가능한 kosis 명령을 찾지 못했습니다.", []string{
			"요청을 더 구체적으로 작성",
			"규칙 기반으로 재시도: kosis quick \"<요청>\"",
		})
		return
	}

	// 생성된 명령어 표시
	fmt.Printf("\n✨ 생성된 명령어:\n  %s\n\n", result.Command)

	// 사용자 확인
	if !confirmExecution() {
		fmt.Println("실행 취소됨")
		return
	}

	// 생성된 명령어 실행 (파싱하여 data 명령어로 전달)
	if err := executeGeneratedCommand(result.Command, formatFlag, outputFlag); err != nil {
		printQuickFailure("AI 경로", err.Error(), []string{
			"생성된 명령을 직접 점검",
			"kosis data/search를 수동 실행",
		})
		return
	}

	fmt.Println("✅ quick 판정: 성공")
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
	fmt.Print("이 명령어를 실행하시겠습니까? (y/n): ")
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		response := strings.ToLower(strings.TrimSpace(scanner.Text()))
		return response == "y" || response == "yes" || response == "예" || response == "ㅇ"
	}
	return false
}

// executeGeneratedCommand는 생성된 명령어를 파싱하여 실행합니다.
func executeGeneratedCommand(command, formatFlag, outputFlag string) error {
	parts := strings.Fields(command)
	if len(parts) > 0 && parts[0] == "kosis" {
		parts = parts[1:]
	}

	if len(parts) == 0 || (parts[0] != "d" && parts[0] != "data") {
		return fmt.Errorf("지원하지 않는 생성 명령: %s", command)
	}
	if len(parts) < 3 {
		return fmt.Errorf("생성된 명령어 인자가 부족합니다: %s", command)
	}

	dataArgs := normalizeDataArgs(parts[1:])

	// AI가 만든 명령을 검증한다. 단, 값을 고치지는 않는다 —
	// 임의 보정은 엉뚱한 데이터를 정답처럼 반환할 수 있어 명시적 실패보다 나쁘다.
	if err := validateGeneratedArgs(dataArgs); err != nil {
		return err
	}

	if formatFlag != "" {
		dataArgs = append(dataArgs, "-f", formatFlag)
	}
	if outputFlag != "" {
		dataArgs = append(dataArgs, "-o", outputFlag)
	}

	displayCmd := "kosis data " + quoteArgs(dataArgs)
	fmt.Printf("실행: %s\n\n", displayCmd)

	if err := runSubcommand("data", dataArgs); err != nil {
		return err
	}

	printReusableCommand(displayCmd)
	return nil
}

// validateGeneratedArgs AI가 생성한 data 인자를 API 호출 전에 검사합니다.
// "명백히 틀린 것"만 막고, "다를 수 있는 것"(어떤 분류 코드를 골랐는지 등)은 통과시킵니다.
// 값을 고치는 코드는 넣지 않습니다.
func validateGeneratedArgs(args []string) error {
	flags := map[string]string{}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			continue
		}
		name, val, hasEq := strings.Cut(a, "=")
		if !hasEq {
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				val = args[i+1]
				i++
			}
		}
		flags[name] = val
	}

	hasClass := false
	for i := 1; i <= 8; i++ {
		if _, ok := flags[fmt.Sprintf("--class%d", i)]; ok {
			hasClass = true
			break
		}
	}
	if _, ok := flags["--user-id"]; ok {
		hasClass = true // user-id 방식은 분류 지정이 불필요
	}
	if !hasClass {
		return fmt.Errorf("AI가 분류 코드(--class1 등)를 생략했습니다. 통계표의 분류를 확인해 직접 지정하세요")
	}
	if _, ok := flags["--item"]; !ok {
		return fmt.Errorf("AI가 항목 코드(--item)를 생략했습니다")
	}

	period, ok := flags["--period"]
	if !ok {
		return fmt.Errorf("AI가 수록주기(--period)를 생략했습니다")
	}
	switch period {
	case "Y", "M", "Q", "H", "D", "F", "IR":
	default:
		return fmt.Errorf("AI가 알 수 없는 수록주기를 지정했습니다: %s (Y/M/Q/H/D/F 중 하나여야 함)", period)
	}

	// 사전에 있는 통계표라면 주기 지원 여부까지 대조한다.
	if len(args) >= 2 {
		if sc, ok := nlp.LookupTable(args[0], args[1]); ok && len(sc.Periods) > 0 {
			supported := false
			for _, p := range sc.Periods {
				if p == period {
					supported = true
					break
				}
			}
			if !supported {
				return fmt.Errorf("%s(%s %s)는 '%s' 주기를 제공하지 않습니다 (지원: %s)",
					sc.Label, sc.OrgID, sc.TblID, period, strings.Join(sc.Periods, ", "))
			}
		}
	}
	return nil
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

// normalizeDataArgs remaps unsupported shorthand flags like -c1 to long flags.
func normalizeDataArgs(args []string) []string {
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

func runSubcommand(subcommand string, args []string) error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("실행 파일 경로 확인 실패: %w", err)
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
		return fmt.Errorf("%s 실행 실패(비정상 종료): %w", subcommand, err)
	}

	output := out.String()
	if output != "" {
		fmt.Print(output)
	}
	if failLine := firstFailureLine(output); failLine != "" {
		return fmt.Errorf("%s 실행 실패: %s", subcommand, failLine)
	}
	return nil
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
