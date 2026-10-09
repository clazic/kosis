package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/interactive"
	"github.com/clazic/kosis/internal/learn"
	"github.com/clazic/kosis/internal/nlp"
	"golang.org/x/term"
)

// quick --ai 파이프라인
//
//	① 해석  hermes(글생성): 요청 → 영어 풀이·키워드·조건·기간   (실패 시 규칙 기반 폴백)
//	② 검색  KOSIS search → 후보 통계표                          (학습: 틀렸던 표 제외)
//	③ 선택  Jev: 후보 중 통계표 1개                             (학습: 확정된 표는 바로 사용)
//	④ 메타  KOSIS meta → 실제 분류·항목·주기 코드
//	⑤ 선택  Jev: 축마다 코드 1개 (한 번의 요청으로 병렬)         (학습: 확정 코드 사용, 틀린 코드 제외)
//	⑥ 실행  kosis data → 결과에 따라 학습 기록, 사람이 확인한 교정은 스킬 문서에도 반영
//
// AI는 코드를 만들지 않는다. 실제 API가 돌려준 목록에서 고르기만 하므로 지어낸 코드가 나갈 수 없다.

const (
	lowConfidence    = 0.3
	maxClassAxes     = 8 // kosis data 가 지원하는 --class1~8
	maxAIAttempts    = 3
	maxLearnAttempts = 6     // 학습 모드는 교정을 여러 번 해 볼 수 있게
	allCode          = "ALL" // 축 전체 펼치기 ("구군별", "업종별")
	srcJev           = "Jev"
	srcLearned       = "학습"
	srcRequested     = "요청"
	srcOnly          = "유일"
	srcCorrected     = "교정"
	itemAxis         = "@ITEM"
	periodAxis       = "@PRD"
	learnReasonLen   = 120
)

// errTableUnusable 메타가 없거나 비어 이 통계표로는 조회할 수 없음 (학습된 표라면 회피 대상).
var errTableUnusable = errors.New("통계표를 사용할 수 없음")

// aiPick 축 하나의 선택 결과.
type aiPick struct {
	Axis     string // 학습 키: 분류 OBJ_ID | @ITEM | @PRD
	Cond     string // 학습 조건 키 (@ITEM 은 키워드까지 포함)
	Flag     string // --class1 … | --item | --period
	Label    string
	Code     string
	Name     string
	Source   string
	Conf     float64
	Ranked   []string            // Jev 확률 순 후보 (교정용)
	Options  map[string]string   // 코드 → 이름
	Children map[string][]string // 상위 코드 → 하위 코드 (메타 UP_ITM_ID, "울산광역시의 하위 전체" 교정용)
}

// aiPlan 실행 직전의 전체 계획.
type aiPlan struct {
	Intent     *nlp.Intent
	Topic      string // 학습 주제 키 (키워드+조건)
	CondKey    string
	Table      string // ORG/TBL
	TableName  string
	TableSrc   string
	TableConf  float64
	TableRank  []string
	TableNames map[string]string
	Picks      []*aiPick
}

func (p *aiPlan) orgTbl() (string, string) {
	org, tbl, _ := strings.Cut(p.Table, "/")
	return org, tbl
}

// aiEnv 파이프라인이 공유하는 의존성.
type aiEnv struct {
	client  *api.Client
	jev     nlp.Endpoint
	store   *learn.Store
	request string
	quiet   bool // 평가 모드: 진행 로그 생략
}

func (e *aiEnv) logf(format string, a ...any) {
	if !e.quiet {
		fmt.Printf(format, a...)
	}
}

// newAIEnv 선택기(Jev)·KOSIS 클라이언트·학습 저장소를 준비한다. 실패 시 안내를 출력하고 nil.
func newAIEnv() *aiEnv {
	_, selector, err := config.GetAIProvider(config.SelectorProvider)
	if err != nil || !selector.IsTypeSafe() {
		printQuickFailure("AI 경로", "선택 공급자(jev)가 설정되지 않았습니다.", []string{"kosis config ai-list 로 jev 항목 확인"})
		return nil
	}
	if selector.ResolveKey() == "" {
		printQuickFailure("AI 경로", "Jev API 키가 없습니다.", []string{"kosis config ai-key jev <TYPESAFE_API_KEY>" + keyEnvHint(selector)})
		return nil
	}
	client, err := NewAPIClient()
	if err != nil {
		printQuickFailure("AI 경로", err.Error(), nil)
		return nil
	}
	return &aiEnv{
		client: client,
		jev:    nlp.Endpoint{Name: config.SelectorProvider, BaseURL: selector.BaseURL, Model: selector.Model, APIKey: selector.ResolveKey()},
		store:  learn.Load(),
	}
}

// handleAIGeneration quick --ai 진입점. providerName이 비면 기본 글생성 공급자.
func handleAIGeneration(providerName, userRequest, formatFlag, outputFlag string) {
	env := newAIEnv()
	if env == nil {
		return
	}
	env.runQuestion(providerName, userRequest, formatFlag, outputFlag, false)
}

// handleAILearnSession quick --learn: 질문 → 실행 → 결과 확인 → (틀리면 교정·재실행) → 다음 질문 … 을 q 까지 반복.
// 실행 전에는 묻지 않는다. 결과를 본 뒤에만 맞는지 묻는다 — 결과를 봐야 맞는지 판단할 수 있기 때문이다.
func handleAILearnSession(providerName, firstRequest, formatFlag, outputFlag string) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		printQuickFailure("학습 모드", "--learn 은 결과를 보고 답해야 하므로 터미널에서만 동작합니다.", []string{
			"터미널에서 kosis q --learn 실행",
			"스크립트에서는 kosis q \"<요청>\" --ai --yes (학습 없음)",
		})
		return
	}
	env := newAIEnv()
	if env == nil {
		return
	}
	fmt.Println("🎓 학습 모드 — 결과가 틀리면 n 으로 교정하세요. 고친 내용은 다음 질문부터 반영됩니다. (종료: q)")
	req := firstRequest
	for n := 1; ; n++ {
		if strings.TrimSpace(req) == "" {
			req = interactive.Prompt(fmt.Sprintf("\n[%d] 통계 질문을 입력하세요:", n))
			if req == "" {
				fmt.Println("학습 모드를 종료합니다. 학습 파일:", learn.FilePath())
				return
			}
		}
		env.runQuestion(providerName, req, formatFlag, outputFlag, true)
		req = ""
	}
}

// runQuestion 질문 하나를 끝까지 처리한다.
// learnMode=false: 실행 전 확인(y/n), 거부하면 교정 / learnMode=true: 바로 실행하고 결과를 본 뒤 맞는지 확인.
func (e *aiEnv) runQuestion(providerName, userRequest, formatFlag, outputFlag string, learnMode bool) {
	e.request = userRequest
	intent := interpretRequest(providerName, userRequest)
	plan, err := e.chooseTable(intent, "")
	if err != nil {
		printQuickFailure("AI 경로", err.Error(), []string{"검색어를 바꿔 재시도하거나 kosis search \"<키워드>\"로 직접 확인"})
		return
	}

	attempts := maxAIAttempts
	if learnMode {
		attempts = maxLearnAttempts
	}
	for attempt := 0; attempt < attempts; attempt++ {
		if plan.Picks == nil {
			if err := e.chooseAxes(plan); err != nil {
				// 학습된 표가 더는 쓸 수 없으면(폐지·이관 등) 회피로 기록하고 한 번 새로 검색한다.
				if errors.Is(err, errTableUnusable) && plan.TableSrc == srcLearned {
					fmt.Printf("⚠ 학습된 통계표 %s를 쓸 수 없어 다시 검색합니다: %v\n", plan.Table, err)
					e.store.TableBad(plan.Topic, plan.Table, "학습된 표 사용 불가: "+learn.Truncate(err.Error(), learnReasonLen))
					saveLearn(e.store)
					if plan, err = e.chooseTable(intent, ""); err == nil {
						continue
					}
				}
				printQuickFailure("AI 경로", err.Error(), []string{
					fmt.Sprintf("kosis meta %s 로 분류를 확인한 뒤 kosis data 로 직접 조회", strings.Replace(plan.Table, "/", " ", 1)),
				})
				return
			}
		}

		dataArgs := buildAIDataArgs(plan, formatFlag, outputFlag, time.Now())
		displayCmd := "kosis data " + quoteArgs(dataArgs)
		printAIPlan(plan)
		fmt.Printf("\n🧭 생성된 명령어:\n  %s\n\n", displayCmd)

		if !learnMode && !confirmExecution() {
			if attempt == attempts-1 {
				fmt.Println("실행 취소됨 (교정 횟수 초과 — kosis meta / kosis data 로 직접 조회하세요)")
				return
			}
			next, retry := e.collectCorrection(plan)
			saveLearn(e.store)
			if !retry {
				fmt.Println("실행 취소됨")
				return
			}
			plan = next
			continue
		}

		out, runErr := runSubcommandOutput("data", dataArgs)
		rows := -1
		if runErr == nil {
			rows = countDataRows(out, effectiveFormat(formatFlag), outputFlag)
		}

		if !learnMode {
			e.finishOnce(plan, displayCmd, runErr, rows)
			return
		}

		// 학습 모드: 결과를 보여준 뒤 판단을 받는다.
		switch {
		case runErr != nil:
			fmt.Printf("❌ 실행 실패: %v\n", runErr)
			if learnFailure(e.store, plan, runErr) {
				fmt.Println("📝 통계표 오류로 학습했습니다.")
			}
		case rows == 0:
			fmt.Println("⚠ 결과 0행 — 기간에 데이터가 없거나 분류 조합이 비어 있습니다.")
		}
		verdict := askVerdict(runErr == nil && rows != 0)
		switch verdict {
		case "y":
			e.learnSuccess(plan, true)
			saveLearn(e.store)
			fmt.Println("✅ 맞다고 학습했습니다. 다음에 같은 질문은 이 선택을 바로 씁니다.")
			printReusableCommand(displayCmd)
			return
		case "s":
			fmt.Println("⏭ 건너뜀 (학습하지 않음)")
			return
		}
		if attempt == attempts-1 {
			fmt.Println("교정 횟수를 넘었습니다. 지금까지의 교정은 학습됐습니다.")
			saveLearn(e.store)
			return
		}
		next, retry := e.collectCorrection(plan)
		saveLearn(e.store)
		if !retry {
			fmt.Println("⏭ 이 질문은 여기까지 (교정 내용은 학습됨)")
			return
		}
		plan = next
	}
}

// finishOnce 일반 모드(--ai)의 실행 결과 처리.
func (e *aiEnv) finishOnce(plan *aiPlan, displayCmd string, runErr error, rows int) {
	if runErr != nil {
		learned := learnFailure(e.store, plan, runErr)
		saveLearn(e.store)
		fmt.Println("❌ quick 판정: 실패")
		fmt.Printf("원인: %v\n", runErr)
		if learned {
			fmt.Println("📝 이 실패를 학습했습니다. 같은 요청을 다시 실행하면 다른 통계표를 고릅니다.")
		}
		return
	}
	if rows == 0 {
		fmt.Println("⚠ quick 판정: 결과 없음 — 요청 기간에 데이터가 없거나 분류 조합이 비어 있습니다. 학습하지 않습니다.")
		return
	}
	fmt.Println("✅ quick 판정: 성공")
	if quickAssumeYes {
		fmt.Println("ℹ --yes 실행은 사람이 결과를 확인하지 않았으므로 학습하지 않습니다. 학습시키려면 --learn")
	} else {
		e.learnSuccess(plan, false)
		saveLearn(e.store)
	}
	printReusableCommand(displayCmd)
}

// askVerdict 결과를 본 사용자의 판단: y(맞음) / n(틀림, 교정) / s(건너뛰기).
func askVerdict(hasResult bool) string {
	label := "\n결과가 질문에 맞나요? [y 맞음 / n 틀림·교정 / s 건너뛰기]"
	if !hasResult {
		label = "\n결과를 얻지 못했습니다. 교정할까요? [n 교정 / s 건너뛰기]"
	}
	for {
		v := strings.ToLower(strings.TrimSpace(interactive.Prompt(label)))
		switch v {
		case "", "s", "skip":
			return "s"
		case "n", "no", "아니", "ㄴ":
			return "n"
		case "y", "yes", "예", "ㅇ":
			if hasResult {
				return "y"
			}
		}
	}
}

func keyEnvHint(p config.AIProvider) string {
	if p.KeyEnv == "" {
		return ""
	}
	return " 또는 환경변수 " + p.KeyEnv
}

func saveLearn(store *learn.Store) {
	if err := store.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "경고: 학습 저장 실패: %v\n", err)
	}
}

// effectiveFormat -f 가 없으면 설정의 기본 형식.
func effectiveFormat(formatFlag string) string {
	if formatFlag != "" {
		return formatFlag
	}
	if f, err := config.GetDefaultFormat(); err == nil {
		return f
	}
	return "table"
}

// countDataRows data 출력에서 결과 행 수를 센다. 셀 수 없으면 -1 (파일 저장 등).
// ponytail: 출력 형식별 휴리스틱. data 가 행 수를 구조적으로 알려주게 되면 그걸로 바꿀 것.
func countDataRows(out, format, outputFlag string) int {
	if outputFlag != "" {
		return -1
	}
	lines := strings.Split(strings.ReplaceAll(out, "\r\n", "\n"), "\n")
	count := func(prefix string) int {
		n := 0
		for _, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), prefix) {
				n++
			}
		}
		return n
	}
	switch format {
	case "json":
		for i, l := range lines {
			if strings.HasPrefix(strings.TrimSpace(l), "[") {
				var arr []json.RawMessage
				if json.Unmarshal([]byte(strings.Join(lines[i:], "\n")), &arr) == nil {
					return len(arr)
				}
				break
			}
		}
		return -1
	case "csv":
		n := 0
		for _, l := range lines {
			if strings.Contains(l, ",") && !strings.HasPrefix(l, "참고:") && !strings.HasPrefix(l, "경고") {
				n++
			}
		}
		return max(n-1, 0) // 헤더 제외
	case "md":
		return max(count("|")-2, 0) // 헤더·구분선 제외
	default: // table
		return max(count("│")-1, 0) // 헤더 제외
	}
}

// interpretRequest 글생성 공급자로 해석하고, 실패하면 규칙 기반으로 폴백한다 (해석 실패로 조회 전체를 막지 않는다).
func interpretRequest(providerName, userRequest string) *nlp.Intent {
	name, p, err := config.GetAIProvider(providerName)
	if err == nil && p.IsTypeSafe() {
		err = fmt.Errorf("'%s'는 선택 전용 공급자라 글생성에 쓸 수 없습니다", name)
	}
	if err == nil {
		fmt.Printf("🤖 %s(%s)로 요청 해석 중...\n", name, p.Model)
		ep := nlp.Endpoint{Name: name, BaseURL: p.BaseURL, Model: p.Model, APIKey: p.ResolveKey()}
		it, perr := nlp.ParseRequest(ep, userRequest, time.Now())
		if perr == nil {
			it.Period.Unit = api.NormalizePrdSe(it.Period.Unit)
			it.Period.Freq = api.NormalizePrdSe(it.Period.Freq)
			return it
		}
		err = perr
	}
	fmt.Printf("⚠ 해석 실패, 규칙 기반으로 대신 해석합니다: %v\n", err)
	return intentFromRules(userRequest)
}

// intentFromRules 규칙 기반 Match 결과를 Intent로 옮긴다.
func intentFromRules(userRequest string) *nlp.Intent {
	m := nlp.Match(userRequest)
	kw := strings.TrimSpace(m.MatchedToken + " " + m.Keyword)
	if kw == "" {
		kw = userRequest
	}
	it := &nlp.Intent{English: userRequest, Keyword: kw}
	if m.RegionName != "" {
		it.Conditions = []string{m.RegionName}
	}
	it.Period.Unit = m.Period
	switch {
	case m.Periods != "":
		it.Period.Kind, it.Period.List = "list", strings.Split(m.Periods, ",")
	case m.Start != "" && m.End != "":
		it.Period.Kind, it.Period.Start, it.Period.End = "range", m.Start, m.End
	case m.Latest > 0:
		it.Period.Kind, it.Period.Latest = "latest", m.Latest
	default:
		it.Period.Kind = "none"
	}
	return it
}

// jevState Jev에 보내는 상황 설명. Jev는 영어 질문만 이해하므로 영어 풀이를 앞세우고 한글 원문을 함께 준다.
func jevState(userRequest string, it *nlp.Intent) string {
	var b strings.Builder
	fmt.Fprintf(&b, "User request (Korean): %s\n", userRequest)
	if it.English != "" && it.English != userRequest {
		fmt.Fprintf(&b, "English: %s\n", it.English)
	}
	if len(it.Conditions) > 0 {
		fmt.Fprintf(&b, "Requested conditions: %s\n", strings.Join(it.Conditions, ", "))
	} else {
		b.WriteString("Requested conditions: none (use national/total values)\n")
	}
	b.WriteString("Option names are Korean KOSIS (Statistics Korea) labels.")
	return b.String()
}

// chooseTable ②③ 학습 → 검색 → Jev 순으로 통계표를 정한다. force가 있으면 그 표를 쓴다.
func (e *aiEnv) chooseTable(it *nlp.Intent, force string) (*aiPlan, error) {
	plan := &aiPlan{Intent: it, CondKey: learn.CondKey(it.Conditions), Topic: learn.TopicKey(it.Keyword, it.Conditions)}

	if force == "" {
		if pref := e.store.PreferTable(plan.Topic); pref != "" {
			plan.Table, plan.TableName, plan.TableSrc, plan.TableConf = pref, pref, srcLearned, 1
			return plan, nil
		}
	}

	e.logf("🔎 통계표 검색: %s\n", it.Keyword)
	var results []api.SearchResult
	tried := map[string]bool{}
	for i, kw := range keywordCandidates(it.Keyword, intentFromRules(e.request).Keyword) {
		if tried[learn.NormKey(kw)] {
			continue
		}
		tried[learn.NormKey(kw)] = true
		if i > 0 {
			e.logf("🔎 결과 없음, 재검색: %s\n", kw)
		}
		res, err := e.client.Search(kw, api.SearchOptions{ResultCount: 30})
		if err != nil && !strings.Contains(err.Error(), "[30]") { // [30] = 검색 결과 없음
			return nil, fmt.Errorf("검색 실패: %w", err)
		}
		if len(res) > 0 {
			results = res
			break
		}
		if force != "" {
			break
		}
	}

	avoid := e.store.AvoidTables(plan.Topic)
	options := map[string]string{}
	for _, r := range results {
		key := r.OrgID + "/" + r.TblID
		if _, bad := avoid[key]; bad && key != force {
			continue
		}
		if len(options) >= nlp.MaxChoiceOptions {
			break
		}
		options[key] = fmt.Sprintf("%s (%s, %s~%s)", r.TblNM, r.OrgNM, r.StrtPrdDe, r.EndPrdDe)
	}
	plan.TableNames = options

	if force != "" {
		plan.Table, plan.TableSrc, plan.TableConf = force, srcCorrected, 1
		plan.TableName = options[force]
		if plan.TableName == "" {
			plan.TableName = force
		}
		return plan, nil
	}
	if len(options) == 0 {
		if len(avoid) > 0 {
			return nil, fmt.Errorf("'%s' 검색 결과가 모두 과거에 틀렸던 표입니다 (학습 파일: %s)", it.Keyword, learn.FilePath())
		}
		return nil, fmt.Errorf("'%s' 검색 결과가 없습니다", it.Keyword)
	}

	ans, err := nlp.Choose(e.jev, jevState(e.request, it), map[string]nlp.ChoiceQuestion{
		"table": {
			Instructions: "Which statistical table best answers the user request? Prefer a table that contains the requested breakdown (e.g. region) and period.",
			Options:      options,
		},
	})
	if err != nil {
		return nil, err
	}
	a := ans["table"]
	plan.Table, plan.TableName, plan.TableSrc = a.Choice, options[a.Choice], srcJev
	plan.TableConf, plan.TableRank = a.Confidence, a.Ranked()
	return plan, nil
}

// searchStopwords 검색어에서 빼는 조사·접속어. "~별"로 끝나는 단어("구군별", "업종별")는 분류 지시라 따로 뺀다.
var searchStopwords = map[string]bool{"와": true, "과": true, "및": true, "랑": true, "이랑": true, "하고": true, "그리고": true, "또는": true, "의": true}

// keywordCandidates 검색 결과가 없을 때 시도할 검색어를 넓은 것부터 차례로 만든다.
//  1. 해석된 검색어 그대로  2. 조사·"~별" 단어를 뺀 것  3. 규칙 기반 키워드  4. 남은 단어 하나씩(긴 것부터)
//
// 문장 전체가 검색어로 넘어가면 KOSIS 가 [30](결과 없음)을 돌려주므로 점점 좁힌다.
func keywordCandidates(primary, rule string) []string {
	out := []string{primary}
	var core []string
	for _, w := range strings.Fields(primary) {
		w = strings.Trim(w, ",.?!")
		if w == "" || searchStopwords[w] || (strings.HasSuffix(w, "별") && len([]rune(w)) > 1) {
			continue
		}
		core = append(core, w)
	}
	if len(core) > 0 {
		out = append(out, strings.Join(core, " "))
	}
	if rule != "" {
		out = append(out, rule)
	}
	sorted := append([]string(nil), core...)
	sort.SliceStable(sorted, func(i, j int) bool { return len([]rune(sorted[i])) > len([]rune(sorted[j])) })
	return append(out, sorted...)
}

// condFirst 조건 단어가 들어간 값을 앞세운 코드 순서 (axisOptions·교정 후보 공통 기준).
func condFirst(options map[string]string, conds []string) []string {
	codes := make([]string, 0, len(options))
	for c := range options {
		codes = append(codes, c)
	}
	hit := func(code string) bool {
		for _, c := range conds {
			if c != "" && strings.Contains(options[code], c) {
				return true
			}
		}
		return false
	}
	sort.SliceStable(codes, func(i, j int) bool {
		hi, hj := hit(codes[i]), hit(codes[j])
		if hi != hj {
			return hi
		}
		return codes[i] < codes[j]
	})
	return codes
}

// axisOptions 축의 코드→이름. 255개를 넘으면 조건 단어가 들어간 값과 최상위 값을 우선해 줄인다.
// ponytail: 단순 문자열 포함 판정. 동의어(예: '서울' vs '서울시')가 문제 되면 hermes로 조건을 축 이름에 맞춰 정규화할 것.
func axisOptions(rows []api.MetaResult, conds []string, avoid map[string]bool) map[string]string {
	all := map[string]string{}
	var order []string
	for _, r := range rows {
		if avoid[r.ItmID] {
			continue
		}
		if _, dup := all[r.ItmID]; !dup {
			order = append(order, r.ItmID)
		}
		all[r.ItmID] = r.ItmNM
	}
	if len(all) <= nlp.MaxChoiceOptions {
		return all
	}
	up := map[string]string{}
	for _, r := range rows {
		up[r.ItmID] = r.UpItmID
	}
	out := map[string]string{}
	add := func(id string) {
		if len(out) < nlp.MaxChoiceOptions {
			out[id] = all[id]
		}
	}
	for _, id := range order { // 1순위: 조건 단어 포함
		for _, c := range conds {
			if c != "" && strings.Contains(all[id], c) {
				add(id)
				break
			}
		}
	}
	for _, id := range order { // 2순위: 최상위(합계·시도 등)
		if up[id] == "" {
			add(id)
		}
	}
	for _, id := range order { // 나머지는 앞에서부터
		add(id)
	}
	return out
}

// classAxis 메타의 분류축 하나.
type classAxis struct {
	id, label, flag string
	sn, seen        int
	rows            []api.MetaResult
}

// buildClassAxes OBJ_ID_SN(숫자) 순으로 정렬해 --class1..N 을 다시 매긴다.
// SN이 없거나 겹쳐도 등장 순서로 안정 정렬하므로 같은 --classN 이 두 번 붙지 않는다.
func buildClassAxes(classifications []api.MetaResult) ([]classAxis, error) {
	var axes []classAxis
	byID := map[string]int{}
	for _, r := range classifications {
		i, ok := byID[r.ObjID]
		if !ok {
			i = len(axes)
			byID[r.ObjID] = i
			sn, err := strconv.Atoi(strings.TrimSpace(r.ObjIDSN))
			if err != nil || sn <= 0 {
				sn = 1 << 30 // SN 없음: 뒤로, 등장 순서 유지
			}
			axes = append(axes, classAxis{id: r.ObjID, label: r.ObjNM, sn: sn, seen: i})
		}
		axes[i].rows = append(axes[i].rows, r)
	}
	sort.SliceStable(axes, func(i, j int) bool {
		if axes[i].sn != axes[j].sn {
			return axes[i].sn < axes[j].sn
		}
		return axes[i].seen < axes[j].seen
	})
	if len(axes) > maxClassAxes {
		return nil, fmt.Errorf("분류축이 %d개라 kosis data(--class1~%d)로 조회할 수 없습니다", len(axes), maxClassAxes)
	}
	for i := range axes {
		axes[i].flag = fmt.Sprintf("--class%d", i+1)
	}
	return axes, nil
}

// chooseAxes ④⑤ 메타를 받아 축마다 코드를 정한다. 학습된 값은 그대로, 나머지는 Jev 한 번으로 병렬 선택.
func (e *aiEnv) chooseAxes(plan *aiPlan) error {
	org, tbl := plan.orgTbl()
	e.logf("📋 메타 조회: %s %s\n", org, tbl)
	meta, err := e.client.MetaSummary(org, tbl)
	if err != nil {
		return fmt.Errorf("%w: 메타 조회 실패: %v", errTableUnusable, err)
	}
	if len(meta.Classifications) == 0 && len(meta.Items) == 0 {
		return fmt.Errorf("%w: 분류·항목 메타가 비어 있습니다", errTableUnusable)
	}
	axes, err := buildClassAxes(meta.Classifications)
	if err != nil {
		return err
	}

	var prdRows []api.MetaResult
	seen := map[string]bool{}
	for _, r := range meta.Periods {
		if c := api.NormalizePrdSe(r.PrdSe); c != "" && !seen[c] {
			seen[c] = true
			prdRows = append(prdRows, api.MetaResult{ItmID: c, ItmNM: r.PrdSe})
		}
	}
	if len(prdRows) == 0 {
		return fmt.Errorf("%w: 수록주기 정보가 없습니다", errTableUnusable)
	}

	plan.Picks = nil
	questions := map[string]nlp.ChoiceQuestion{}
	qPick := map[string]*aiPick{}
	conds := plan.Intent.Conditions
	itemCond := plan.CondKey + "#" + learn.NormKey(plan.Intent.Keyword) // 같은 표라도 측정 대상(키워드)이 다르면 항목이 다르다

	decide := func(id, label, flag, cond, instructions string, rows []api.MetaResult, requested string) {
		pk := &aiPick{Axis: id, Cond: cond, Flag: flag, Label: label, Children: map[string][]string{}}
		for _, r := range rows {
			if r.UpItmID != "" {
				pk.Children[r.UpItmID] = append(pk.Children[r.UpItmID], r.ItmID)
			}
		}
		plan.Picks = append(plan.Picks, pk)
		opts := axisOptions(rows, conds, e.store.AvoidCodes(plan.Table, id, cond))
		if len(opts) == 0 {
			// 과거 학습이 모든 값을 피하게 만들었으면 학습을 무시한다 — 학습이 축을 영구히 잠그면 안 된다.
			opts = axisOptions(rows, conds, nil)
		}
		pk.Options = opts
		if requested != "" {
			if name, ok := opts[requested]; ok {
				pk.Code, pk.Name, pk.Source, pk.Conf = requested, name, srcRequested, 1
				return
			}
		}
		if code, ok := e.store.AxisPick(plan.Table, id, cond); ok {
			if name, exists := learnedName(opts, code); exists {
				pk.Code, pk.Name, pk.Source, pk.Conf = code, name, srcLearned, 1
				return
			}
		}
		if len(opts) == 1 {
			for code, name := range opts {
				pk.Code, pk.Name, pk.Source, pk.Conf = code, name, srcOnly, 1
			}
			return
		}
		qid := fmt.Sprintf("q%d", len(questions))
		questions[qid] = nlp.ChoiceQuestion{Instructions: instructions, Options: opts}
		qPick[qid] = pk
	}

	for _, a := range axes {
		decide(a.id, a.label, a.flag, plan.CondKey,
			fmt.Sprintf("For the classification '%s', which value matches the user request? If the request does not restrict this classification, choose the total/national value (e.g. 계, 전국, 합계).", a.label),
			a.rows, "")
	}
	if len(meta.Items) > 0 {
		decide(itemAxis, "항목", "--item", itemCond, "Which item (measure) matches what the user asked for?", meta.Items, "")
	}
	decide(periodAxis, "수록주기", "--period", plan.CondKey,
		"Which publication frequency (년=yearly, 월=monthly, 분기=quarterly, 반기=half-yearly) best fits the requested period?",
		prdRows, requestedFreq(plan.Intent.Period))

	if len(questions) > 0 {
		ans, err := nlp.Choose(e.jev, jevState(e.request, plan.Intent), questions)
		if err != nil {
			return err
		}
		for qid, pk := range qPick {
			a := ans[qid]
			pk.Code, pk.Name, pk.Source, pk.Conf, pk.Ranked = a.Choice, pk.Options[a.Choice], srcJev, a.Confidence, a.Ranked()
		}
	}
	return nil
}

// requestedFreq 사용자가 원한 출력 주기. 명시(freq)가 없으면 "최근 N개월"의 단위를 쓴다.
func requestedFreq(ps nlp.PeriodSpec) string {
	if ps.Freq != "" {
		return ps.Freq
	}
	return ps.Unit
}

// maxListedCodes kosis data 가 "+"로 나열해 받는 코드 수 상한 (넘으면 "접두사*" 사용).
const maxListedCodes = 10

// childrenCode 하위 코드들을 data 인자 하나로. 10개 이하면 "a+b+c", 넘으면 모두 상위 코드로 시작할 때 "상위*".
func childrenCode(parent string, kids []string) (string, bool) {
	if len(kids) == 0 {
		return "", false
	}
	if len(kids) <= maxListedCodes {
		return strings.Join(kids, "+"), true
	}
	for _, k := range kids {
		if !strings.HasPrefix(k, parent) {
			return "", false // 접두사로 묶을 수 없으면 제공하지 않는다 (틀린 범위를 조회하느니 없는 게 낫다)
		}
	}
	return parent + "*", true
}

// learnedName 학습된 코드가 지금 메타에 있는지. ALL·"a+b"·"접두*" 같은 묶음 코드도 확인한다.
func learnedName(opts map[string]string, code string) (string, bool) {
	switch {
	case code == allCode:
		return "전체", true
	case strings.HasSuffix(code, "*"):
		prefix := strings.TrimSuffix(code, "*")
		if name, ok := opts[prefix]; ok {
			return name + " 하위 전체", true
		}
		for c := range opts {
			if strings.HasPrefix(c, prefix) {
				return prefix + "* 하위 전체", true
			}
		}
		return "", false
	case strings.Contains(code, "+"):
		var names []string
		for _, c := range strings.Split(code, "+") {
			n, ok := opts[c]
			if !ok {
				return "", false
			}
			names = append(names, n)
		}
		return strings.Join(names, "·"), true
	}
	name, ok := opts[code]
	return name, ok
}

// perYear 주기별 1년당 시점 수. 환산할 수 없는 주기(5년·일·부정기)는 0.
func perYear(unit string) int {
	switch unit {
	case "Y":
		return 1
	case "H":
		return 2
	case "Q":
		return 4
	case "M":
		return 12
	}
	return 0
}

// convPeriod "YYYY" 또는 "YYYYMM" 을 표 주기 형식으로. end 면 그 해(또는 그 달이 속한 분기·반기)의 끝.
func convPeriod(v, unit string, end bool) string {
	v = strings.TrimSpace(v)
	if len(v) < 4 {
		return v
	}
	year := v[:4]
	month := 0
	if len(v) >= 6 {
		month, _ = strconv.Atoi(v[4:6])
	}
	switch unit {
	case "Y", "F":
		return year
	case "M":
		if month > 0 {
			return v[:6]
		}
		if end {
			return year + "12"
		}
		return year + "01"
	case "Q", "H":
		per := 3
		last := 4
		if unit == "H" {
			per, last = 6, 2
		}
		if month > 0 {
			return year + strconv.Itoa((month-1)/per+1)
		}
		if end {
			return year + strconv.Itoa(last)
		}
		return year + "1"
	}
	return v
}

// periodArgs 해석한 기간을 표의 주기 형식에 맞춘다.
func periodArgs(ps nlp.PeriodSpec, unit string, now time.Time) []string {
	switch ps.Kind {
	case "range":
		start, end := ps.Start, ps.End
		if start == "" && end == "" {
			break
		}
		if start == "" {
			start = end
		}
		if end == "" { // "2020년 이후" → 올해까지
			end = strconv.Itoa(now.Year())
		}
		return []string{"--start", convPeriod(start, unit, false), "--end", convPeriod(end, unit, true)}
	case "list":
		var vs []string
		for _, v := range ps.List {
			vs = append(vs, convPeriod(v, unit, false))
		}
		if len(vs) > 0 {
			return []string{"--periods", strings.Join(vs, ",")}
		}
	case "latest":
		if ps.Latest > 0 {
			n := ps.Latest
			// "최근 5년"을 월간 표에, "최근 24개월"을 연간 표에 맞게 환산 (올림)
			if src, dst := perYear(ps.Unit), perYear(unit); src > 0 && dst > 0 && src != dst {
				n = (n*dst + src - 1) / src
			}
			return []string{"--latest", strconv.Itoa(n)}
		}
	}
	return []string{"--latest", "1"}
}

func buildAIDataArgs(plan *aiPlan, formatFlag, outputFlag string, now time.Time) []string {
	org, tbl := plan.orgTbl()
	args := []string{org, tbl}
	unit := ""
	for _, pk := range plan.Picks {
		args = append(args, pk.Flag, pk.Code)
		if pk.Axis == periodAxis {
			unit = pk.Code
		}
	}
	args = append(args, periodArgs(plan.Intent.Period, unit, now)...)
	if formatFlag != "" {
		args = append(args, "-f", formatFlag)
	}
	if outputFlag != "" {
		args = append(args, "-o", outputFlag)
	}
	return args
}

func printAIPlan(plan *aiPlan) {
	src := func(s string, c float64) string {
		if s == srcJev {
			mark := ""
			if c < lowConfidence {
				mark = " ⚠ 낮음"
			}
			return fmt.Sprintf("Jev %.2f%s", c, mark)
		}
		return s
	}
	fmt.Printf("\n📊 통계표: %s — %s [%s]\n", plan.Table, plan.TableName, src(plan.TableSrc, plan.TableConf))
	if plan.TableSrc == srcJev && plan.TableConf < lowConfidence {
		for i, k := range plan.TableRank {
			if i >= 3 {
				break
			}
			fmt.Printf("     후보 %d) %s — %s\n", i+1, k, plan.TableNames[k])
		}
	}
	for _, pk := range plan.Picks {
		fmt.Printf("   %-10s %s %s (%s) [%s]\n", pk.Label, pk.Flag, pk.Code, pk.Name, src(pk.Source, pk.Conf))
	}
}

// trusted 학습해도 되는 선택인가: 사람이 고른 값이거나 신뢰도가 충분한 Jev 선택.
func trusted(src string, conf float64) bool {
	switch src {
	case srcCorrected, srcLearned:
		return true
	case srcJev:
		return conf >= lowConfidence
	}
	return false // 요청·유일은 선택이 아니라 학습할 것이 없다
}

// learnSuccess 사람이 확인하고 결과가 나온 선택을 확정하고, 확정된 교정은 스킬 문서에 반영한다.
// verified: 사람이 결과를 보고 맞다고 했다(--learn) — 이때는 낮은 신뢰도 선택도 정답으로 학습한다.
func (e *aiEnv) learnSuccess(plan *aiPlan, verified bool) {
	kw := plan.Intent.Keyword
	ok := func(src string, conf float64) bool {
		if verified {
			return src != srcRequested && src != srcOnly
		}
		return trusted(src, conf)
	}
	if verified || trusted(plan.TableSrc, plan.TableConf) {
		corrected := e.store.AvoidTables(plan.Topic) // TableOK 가 지우기 전에 받아 둔다
		e.store.TableOK(plan.Topic, plan.Table)
		good := learn.ParseTableRef(plan.Table)
		for bad, reason := range corrected {
			if !strings.HasPrefix(reason, "사용자") {
				continue // 사람이 틀렸다고 한 표만 문서에서 고친다
			}
			e.fixSkillDocs(kw, learn.ParseTableRef(bad), good,
				fmt.Sprintf("'%s' 요청에 %s 를 잘못 고름 (quick --ai 학습)", kw, bad),
				reason, fmt.Sprintf("`%s %s` 사용", good.Org, good.Tbl))
		}
	}
	for _, pk := range plan.Picks {
		if !ok(pk.Source, pk.Conf) {
			continue
		}
		e.store.AxisOK(plan.Table, pk.Axis, pk.Cond, pk.Code)
		if pk.Source == srcCorrected {
			org, tbl := plan.orgTbl()
			e.fixSkillDocs("", learn.TableRef{}, learn.TableRef{},
				fmt.Sprintf("'%s' 요청의 %s 값 오선택 (quick --ai 학습)", kw, pk.Label), "사용자 교정",
				fmt.Sprintf("`%s %s %s %s` (%s)", org, tbl, pk.Flag, pk.Code, pk.Name))
		}
	}
}

// fixSkillDocs 설치된 스킬 문서의 틀린 표를 고치고 LEARNINGS.md 에 기록한다. 실패해도 조회는 성공으로 둔다.
func (e *aiEnv) fixSkillDocs(keyword string, bad, good learn.TableRef, pitfall, why, next string) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	dirs := collectSkillDirs(home)
	if keyword != "" {
		changes, err := learn.FixDocs(dirs, keyword, bad, good)
		for _, c := range changes {
			fmt.Printf("📝 문서 수정 %s:%d\n   - %s\n   + %s\n", c.File, c.Line, strings.TrimSpace(c.Before), strings.TrimSpace(c.After))
		}
		if err != nil {
			fmt.Fprintf(os.Stderr, "경고: 문서 수정 실패: %v\n", err)
		}
	}
	table := fmt.Sprintf("%s / `%s`", good.Org, good.Tbl)
	if good.Tbl == "" {
		table = "quick --ai"
	}
	added, err := learn.AppendLearning(dirs, table, pitfall, why, next)
	for _, f := range added {
		fmt.Printf("📝 오답노트 기록: %s\n", f)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "경고: %v\n", err)
	}
}

// learnFailure 실행 오류를 학습한다. 통계표 탓이 분명한 오류([20] 분류 누락, [21] 잘못된 요청 변수)만 기록한다.
// [30](데이터 없음)은 기간 탓이 대부분이고, 네트워크·파일 저장·셀 수 초과 등은 표가 틀린 게 아니므로 학습하지 않는다.
// 반환값: 학습했는지.
func learnFailure(store *learn.Store, plan *aiPlan, err error) bool {
	msg := err.Error()
	if !strings.Contains(msg, "[20]") && !strings.Contains(msg, "[21]") {
		return false
	}
	store.TableBad(plan.Topic, plan.Table, "실행 오류: "+learn.Truncate(msg, learnReasonLen))
	return true
}

// selectOrCancel interactive.Select 에 "취소" 항목을 붙인다. 취소면 -1.
func selectOrCancel(label string, options []string) int {
	i, _ := interactive.Select(label, append(options, "취소"))
	if i < 0 || i >= len(options) {
		return -1
	}
	return i
}

// searchTable 사용자가 입력한 검색어로 KOSIS 를 검색해 통계표를 고르게 한다. 취소면 "".
func (e *aiEnv) searchTable() (string, string) {
	for {
		kw := interactive.Prompt("검색어 (예: 사업체 종사자수 시군구 산업):")
		if kw == "" {
			return "", ""
		}
		results, err := e.client.Search(kw, api.SearchOptions{ResultCount: 20})
		if err != nil {
			fmt.Println("검색 실패:", err)
			continue
		}
		if len(results) == 0 {
			fmt.Println("결과가 없습니다. 다른 검색어를 입력하세요.")
			continue
		}
		var ids, names, labels []string
		for _, r := range results {
			ids = append(ids, r.OrgID+"/"+r.TblID)
			names = append(names, fmt.Sprintf("%s (%s, %s~%s)", r.TblNM, r.OrgNM, r.StrtPrdDe, r.EndPrdDe))
			labels = append(labels, fmt.Sprintf("%s/%s — %s (%s, %s~%s)", r.OrgID, r.TblID, r.TblNM, r.OrgNM, r.StrtPrdDe, r.EndPrdDe))
		}
		j := selectOrCancel("통계표를 고르세요", append(labels, "다른 검색어로 다시 검색"))
		switch {
		case j < 0:
			return "", ""
		case j < len(ids):
			return ids[j], names[j]
		}
	}
}

// collectCorrection 사용자가 실행을 거부했을 때 무엇이 틀렸는지 묻고 학습한다.
// 터미널이 아니면 물을 수 없으므로 학습 없이 취소한다.
func (e *aiEnv) collectCorrection(plan *aiPlan) (*aiPlan, bool) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, false
	}
	switch selectOrCancel("무엇이 틀렸나요? (학습해서 다음엔 반복하지 않습니다)", []string{"통계표가 틀림", "분류·항목·주기 값이 틀림"}) {
	case 0:
		e.store.TableBad(plan.Topic, plan.Table, "사용자: 통계표 틀림")
		var alts, labels []string
		for _, k := range plan.TableRank {
			if k != plan.Table && len(alts) < 8 {
				alts = append(alts, k)
				labels = append(labels, k+" — "+plan.TableNames[k])
			}
		}
		force, forceName := "", ""
		labels = append(labels, "검색어를 직접 입력해 찾기", "목록에 없음 (다시 검색해 Jev가 다시 고름)")
		j := selectOrCancel("맞는 통계표를 고르세요", labels)
		switch {
		case j < 0:
			return nil, false
		case j < len(alts):
			force = alts[j]
		case j == len(alts):
			if force, forceName = e.searchTable(); force == "" {
				return nil, false
			}
		}
		// 학습된 표였거나 "목록에 없음"이면 다시 검색한다 (방금 틀린 표는 회피 목록에 있어 제외된다).
		next, err := e.chooseTable(plan.Intent, force)
		if err != nil {
			fmt.Println("오류:", err)
			return nil, false
		}
		if forceName != "" && next.TableName == force {
			next.TableName = forceName // 검색으로 고른 표는 Jev 후보 목록에 이름이 없을 수 있다
		}
		return next, true

	case 1:
		var editable []*aiPick
		var labels []string
		for _, pk := range plan.Picks {
			if pk.Source == srcOnly || pk.Source == srcRequested {
				continue // 유일한 값·요청에서 온 값은 고를 대안이 없거나 사용자 의도다
			}
			editable = append(editable, pk)
			labels = append(labels, fmt.Sprintf("%s: %s (%s)", pk.Label, pk.Name, pk.Code))
		}
		if len(editable) == 0 {
			fmt.Println("고칠 수 있는 값이 없습니다 (모두 유일한 값이거나 요청에서 온 값).")
			return nil, false
		}
		k := selectOrCancel("틀린 축을 고르세요", labels)
		if k < 0 {
			return nil, false
		}
		pk := editable[k]
		cands := pk.Ranked
		if len(cands) == 0 {
			cands = condFirst(pk.Options, plan.Intent.Conditions)
		}
		var codes, names []string
		for _, c := range cands {
			if c != pk.Code && len(codes) < 20 {
				codes = append(codes, c)
				names = append(names, fmt.Sprintf("%s (%s)", pk.Options[c], c))
			}
		}
		if pk.Axis != periodAxis && pk.Code != allCode {
			// "구군별", "업종별"처럼 축 전체를 원하는 질문용
			codes = append([]string{allCode}, codes...)
			names = append([]string{"전체 (ALL) — 이 축의 모든 값을 펼쳐서 조회"}, names...)
			// "울산 구군별"처럼 지금 값의 하위만 펼치고 싶을 때 (메타의 상위-하위 관계)
			if code, ok := childrenCode(pk.Code, pk.Children[pk.Code]); ok {
				var kidNames []string
				for _, k := range pk.Children[pk.Code] {
					kidNames = append(kidNames, pk.Options[k])
				}
				codes = append([]string{code}, codes...)
				names = append([]string{fmt.Sprintf("%s 의 하위 전체 (%d개: %s)", pk.Name, len(kidNames), learn.Truncate(strings.Join(kidNames, ", "), 40))}, names...)
			}
		}
		j := selectOrCancel("맞는 값을 고르세요 (목록에 없으면 취소 후 kosis meta 로 확인)", names)
		if j < 0 {
			return nil, false
		}
		e.store.AxisBad(plan.Table, pk.Axis, pk.Cond, pk.Code, "사용자: 값 틀림")
		name, _ := learnedName(pk.Options, codes[j])
		pk.Code, pk.Name, pk.Source, pk.Conf = codes[j], name, srcCorrected, 1
		return plan, true
	}
	return nil, false
}
