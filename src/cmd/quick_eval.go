package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/clazic/kosis/internal/config"
	"github.com/clazic/kosis/internal/learn"
	"github.com/clazic/kosis/internal/nlp"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// quick-eval: quick --ai 의 선택을 정답과 비교해 점수를 매기고, 라운드 사이에 "사람의 교정"을 흉내 내
// 학습시켜 점수가 어떻게 바뀌는지 보여준다. 실제 학습 파일·스킬 문서는 건드리지 않는다(임시 학습 파일).
// 데이터 조회(data)는 하지 않고 계획만 채점한다 — 빠르고, 같은 계획이면 같은 점수.

var quickEvalCmd = &cobra.Command{
	Use:    "quick-eval <질문.yaml>",
	Short:  "quick --ai 학습 평가 (질문별 점수와 라운드별 학습 효과)",
	Hidden: true, // 개발·검증용. 일반 사용 흐름에는 나오지 않는다.
	Long: `quick --ai 의 통계표·값·기간 선택을 정답과 비교해 100점 만점으로 채점하고,
라운드 사이에 사람의 교정을 흉내 내 학습시켜 점수 변화를 보여줍니다.

채점 (100점):
  통계표 40  정답 허용 목록(tables)에 있으면
  값     40  정답 값 이름(values) 중 맞힌 비율
  주기   10  기대 수록주기(unit)와 같으면
  기간   10  기대 기간 인자(period)와 같으면

라운드 사이 학습 (사람 교정 흉내):
  표가 맞으면 확정, 틀리면 회피 + 정답 표가 검색 후보에 있으면 그 표로 교정
  값이 틀리면 정답 값으로 교정. 기간은 문장 해석 문제라 학습 대상이 아닙니다.

격리:
  학습은 임시 파일에만 기록합니다 (--learn-file 로 지정 가능). ~/.kosis/learnings.yaml 과
  스킬 문서는 바뀌지 않습니다. 요청 해석(hermes)은 질문당 1번만 하고 라운드 간 재사용합니다.

정답 파일 형식 (YAML 목록):
  - q: "재작년 경남 여성 고용률"
    tables: [101/INH_1DA7014S_03]          # 허용 통계표
    values: ["경상남도", "여자|여성"]       # 고른 값 이름에 포함돼야 할 말. |=대안, 이름@코드접두
    unit: Y                                # 기대 수록주기
    period: "--start {Y-2} --end {Y-2}"    # 기대 기간 인자. {Y}=올해, {Y-1}=작년, |=대안
    hard: "어려운 점 설명"

사용법:
  kosis quick-eval scripts/quick-ai-eval.yaml
  kosis quick-eval scripts/quick-ai-eval.yaml --rounds 3 --limit 10`,
	Args: cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		rounds, _ := cmd.Flags().GetInt("rounds")
		limit, _ := cmd.Flags().GetInt("limit")
		learnFile, _ := cmd.Flags().GetString("learn-file")
		provider, _ := cmd.Flags().GetString("ai")
		if err := runQuickEval(args[0], rounds, limit, learnFile, provider); err != nil {
			fmt.Printf("오류: %v\n", err)
			os.Exit(1)
		}
	},
}

func init() {
	rootCmd.AddCommand(quickEvalCmd)
	quickEvalCmd.Flags().Int("rounds", 3, "라운드 수 (라운드 사이에 학습)")
	quickEvalCmd.Flags().Int("limit", 0, "앞에서부터 n개 질문만 (0=전부)")
	quickEvalCmd.Flags().String("learn-file", "", "학습을 기록할 파일 (기본: 임시 파일, 끝나면 삭제)")
	quickEvalCmd.Flags().String("ai", "", "요청 해석에 쓸 글생성 공급자 (기본: 설정의 기본값)")
}

// evalCase 질문 하나와 정답.
type evalCase struct {
	Q      string   `yaml:"q"`
	Tables []string `yaml:"tables"`
	Values []string `yaml:"values"`
	Unit   string   `yaml:"unit"`
	Period string   `yaml:"period"`
	Hard   string   `yaml:"hard"`
}

// evalResult 한 라운드에서 질문 하나의 채점.
type evalResult struct {
	Total, Table, Values, Unit, Period int
	Chosen                             string   // 고른 표
	Src                                string   // 표 출처 (Jev/학습)
	Missed                             []string // 못 맞힌 항목
	Learned                            []string // 이 라운드 뒤 학습한 내용
}

func loadEvalCases(path string, limit int) ([]evalCase, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cases []evalCase
	if err := yaml.Unmarshal(data, &cases); err != nil {
		return nil, fmt.Errorf("%s 파싱 실패: %w", path, err)
	}
	for i, c := range cases {
		if c.Q == "" || len(c.Tables) == 0 {
			return nil, fmt.Errorf("%d번 질문에 q 또는 tables 가 없습니다", i+1)
		}
	}
	if limit > 0 && limit < len(cases) {
		cases = cases[:limit]
	}
	return cases, nil
}

// expandYears {Y}, {Y-1}, {Y+1} … 를 기준 연도로 바꾼다.
func expandYears(s string, now time.Time) string {
	for {
		i := strings.Index(s, "{Y")
		if i < 0 {
			return s
		}
		j := strings.Index(s[i:], "}")
		if j < 0 {
			return s
		}
		off := 0
		if inner := s[i+2 : i+j]; inner != "" {
			off, _ = strconv.Atoi(inner)
		}
		s = s[:i] + strconv.Itoa(now.Year()+off) + s[i+j+1:]
	}
}

// valueMatches 토큰("A|B", "북구@26")이 고른 값 중 하나와 맞는지.
func valueMatches(token string, picks []*aiPick) bool {
	for _, alt := range strings.Split(token, "|") {
		name, prefix, _ := strings.Cut(strings.TrimSpace(alt), "@")
		for _, pk := range picks {
			if pk.Axis == periodAxis {
				continue
			}
			if strings.Contains(pk.Name, name) && strings.HasPrefix(pk.Code, prefix) {
				return true
			}
		}
	}
	return false
}

// optionMatching 축의 후보 중 토큰에 맞는 코드. 이름이 정확히 같은 값을 우선한다.
func optionMatching(token string, pk *aiPick) (string, bool) {
	best := ""
	for _, alt := range strings.Split(token, "|") {
		name, prefix, _ := strings.Cut(strings.TrimSpace(alt), "@")
		for code, n := range pk.Options {
			if !strings.Contains(n, name) || !strings.HasPrefix(code, prefix) {
				continue
			}
			if n == name {
				return code, true
			}
			if best == "" || len(pk.Options[code]) < len(pk.Options[best]) {
				best = code // 가장 짧은 이름 = 가장 일반적인 값 (예: '여자' vs '여자 고용률 …')
			}
		}
	}
	return best, best != ""
}

func scoreEval(c evalCase, plan *aiPlan, now time.Time) evalResult {
	r := evalResult{Chosen: plan.Table, Src: plan.TableSrc}
	for _, t := range c.Tables {
		if t == plan.Table {
			r.Table = 40
		}
	}
	if r.Table == 0 {
		r.Missed = append(r.Missed, "표")
	}

	if len(c.Values) == 0 {
		r.Values = 40
	} else {
		hit := 0
		for _, v := range c.Values {
			if valueMatches(v, plan.Picks) {
				hit++
			} else {
				r.Missed = append(r.Missed, "값:"+strings.Split(v, "|")[0])
			}
		}
		r.Values = 40 * hit / len(c.Values)
	}

	unit := ""
	for _, pk := range plan.Picks {
		if pk.Axis == periodAxis {
			unit = pk.Code
		}
	}
	if c.Unit == "" || c.Unit == unit {
		r.Unit = 10
	} else {
		r.Missed = append(r.Missed, "주기:"+unit+"≠"+c.Unit)
	}

	got := strings.Join(periodArgs(plan.Intent.Period, unit, now), " ")
	if c.Period == "" {
		r.Period = 10
	} else {
		for _, alt := range strings.Split(expandYears(c.Period, now), "|") {
			if strings.TrimSpace(alt) == got {
				r.Period = 10
			}
		}
		if r.Period == 0 {
			r.Missed = append(r.Missed, "기간:"+got)
		}
	}
	r.Total = r.Table + r.Values + r.Unit + r.Period
	return r
}

// simulateCorrection 사람이 결과를 보고 교정하는 것을 정답으로 흉내 내 학습시킨다.
func simulateCorrection(store *learn.Store, c evalCase, plan *aiPlan, r *evalResult) {
	if r.Table == 0 {
		tableBad(store, plan, plan.Table, "평가 교정: 통계표 틀림")
		for _, g := range c.Tables {
			if name, ok := plan.TableNames[g]; ok {
				tableOK(store, plan, g)
				store.SetTableName(g, name)
				r.Learned = append(r.Learned, "표→"+g)
				return
			}
		}
		r.Learned = append(r.Learned, "표 회피(정답 후보 없음)")
		return
	}
	tableOK(store, plan, plan.Table)
	store.SetTableName(plan.Table, plan.TableName)

	// satisfies 이 축의 현재 값이 다른 정답 조건을 이미 만족하는가.
	// 다중 항목 질문("출생아 수와 사망자 수")에서 맞힌 축을 다른 조건으로 덮어써 라운드마다 뒤집히는 것을 막는다.
	satisfies := func(pk *aiPick, except string) bool {
		for _, v := range c.Values {
			if v != except && valueMatches(v, []*aiPick{pk}) {
				return true
			}
		}
		return false
	}
	fixed := map[*aiPick]bool{}
	for _, v := range c.Values {
		if valueMatches(v, plan.Picks) {
			continue
		}
		for _, pk := range plan.Picks {
			if pk.Axis == periodAxis || fixed[pk] || satisfies(pk, v) {
				continue
			}
			if code, ok := optionMatching(v, pk); ok && code != pk.Code {
				axisBad(store, plan.Table, pk, pk.Code, "평가 교정: 값 틀림")
				axisOK(store, plan.Table, pk, code)
				r.Learned = append(r.Learned, fmt.Sprintf("%s→%s", pk.Label, pk.Options[code]))
				fixed[pk] = true
				break
			}
		}
	}
	for _, pk := range plan.Picks {
		if !fixed[pk] && pk.Source == srcJev {
			axisOK(store, plan.Table, pk, pk.Code)
		}
	}
}

func runQuickEval(path string, rounds, limit int, learnFile, provider string) error {
	if rounds < 1 {
		rounds = 1
	}
	cases, err := loadEvalCases(path, limit)
	if err != nil {
		return err
	}

	_, selector, err := config.GetAIProvider(config.SelectorProvider)
	if err != nil || !selector.IsTypeSafe() || selector.ResolveKey() == "" {
		return fmt.Errorf("Jev 공급자·키가 필요합니다: kosis config ai-key jev <KEY>")
	}
	client, err := NewAPIClient()
	if err != nil {
		return err
	}

	tmp := learnFile == ""
	if tmp {
		dir, err := os.MkdirTemp("", ".kosis-eval-*")
		if err != nil {
			return err
		}
		defer os.RemoveAll(dir)
		learnFile = filepath.Join(dir, "learnings.yaml")
	}
	env := &aiEnv{
		client: client,
		jev:    nlp.Endpoint{Name: config.SelectorProvider, BaseURL: selector.BaseURL, Model: selector.Model, APIKey: selector.ResolveKey()},
		store:  learn.LoadFrom(learnFile),
		quiet:  true,
	}
	now := time.Now()

	fmt.Printf("📝 질문 %d개 × %d라운드 (학습 파일: %s)\n\n", len(cases), rounds, map[bool]string{true: "임시", false: learnFile}[tmp])

	// 해석은 질문당 1번 — 라운드 간 점수 변화가 학습 효과만 반영하도록
	intents := make([]*nlp.Intent, len(cases))
	for i, c := range cases {
		fmt.Printf("[해석 %2d/%d] %s\n", i+1, len(cases), c.Q)
		intents[i] = interpretRequest(provider, c.Q)
	}

	results := make([][]evalResult, rounds)
	for r := 0; r < rounds; r++ {
		fmt.Printf("\n━━━ 라운드 %d ━━━\n", r+1)
		results[r] = make([]evalResult, len(cases))
		for i, c := range cases {
			env.request = c.Q
			res := evalOne(env, c, intents[i], now)
			results[r][i] = res
			note := ""
			if len(res.Missed) > 0 {
				note = "  ✗ " + strings.Join(res.Missed, ", ")
			}
			if len(res.Learned) > 0 {
				note += "  📚 " + strings.Join(res.Learned, ", ")
			}
			fmt.Printf("  [%2d] %3d점  %s %s%s\n", i+1, res.Total, padWidth(res.Src, 4), padWidth(truncWidth(c.Q, 30), 30), note)
		}
		if err := env.store.Save(); err != nil {
			fmt.Fprintf(os.Stderr, "경고: 학습 저장 실패: %v\n", err)
		}
	}

	printEvalSummary(cases, results)
	return nil
}

// evalOne 질문 하나의 계획을 만들고 채점한 뒤 교정을 학습한다.
func evalOne(env *aiEnv, c evalCase, intent *nlp.Intent, now time.Time) evalResult {
	plan, err := env.chooseTable(intent, "")
	if err == nil {
		err = env.chooseAxes(plan)
		if err != nil && plan.TableSrc == srcLearned {
			tableBad(env.store, plan, plan.Table, "학습된 표 사용 불가")
			if plan, err = env.chooseTable(intent, ""); err == nil {
				err = env.chooseAxes(plan)
			}
		}
	}
	if err != nil {
		r := evalResult{Src: "-", Missed: []string{"오류: " + learn.Truncate(err.Error(), 50)}}
		if plan != nil {
			r.Chosen = plan.Table
			tableBad(env.store, plan, plan.Table, "평가: 조회 불가")
		}
		return r
	}
	res := scoreEval(c, plan, now)
	simulateCorrection(env.store, c, plan, &res)
	return res
}

func printEvalSummary(cases []evalCase, results [][]evalResult) {
	rounds := len(results)
	fmt.Printf("\n━━━ 결과 요약 ━━━\n\n")
	head := fmt.Sprintf("%3s  %-26s", "#", "질문")
	for r := 0; r < rounds; r++ {
		head += fmt.Sprintf("  R%d ", r+1)
	}
	fmt.Println(head + "  변화  남은 문제")

	sums := make([]int, rounds)
	improved, perfect := 0, 0
	for i, c := range cases {
		line := fmt.Sprintf("%3d  %s", i+1, padWidth(truncWidth(c.Q, 26), 26))
		for r := 0; r < rounds; r++ {
			line += fmt.Sprintf("  %3d", results[r][i].Total)
			sums[r] += results[r][i].Total
		}
		first, last := results[0][i].Total, results[rounds-1][i]
		delta := last.Total - first
		mark := "    ="
		if delta > 0 {
			mark = fmt.Sprintf("  +%3d", delta)
			improved++
		} else if delta < 0 {
			mark = fmt.Sprintf("  %4d", delta)
		}
		if last.Total == 100 {
			perfect++
		}
		rest := strings.Join(last.Missed, ", ")
		if rest == "" {
			rest = "-"
		}
		fmt.Println(line + mark + "  " + rest)
	}

	avg := fmt.Sprintf("\n%3s  %s", "", padWidth("평균", 26))
	for r := 0; r < rounds; r++ {
		avg += fmt.Sprintf("  %3d", sums[r]/len(cases))
	}
	fmt.Println(avg)
	fmt.Printf("\n학습으로 점수가 오른 질문 %d개 / 마지막 라운드 만점 %d개 / 전체 %d개\n", improved, perfect, len(cases))
	fmt.Println("채점: 통계표 40 + 값 40 + 주기 10 + 기간 10. 기간은 문장 해석 문제라 학습으로 바뀌지 않습니다.")
	fmt.Println("다중 지역·다중 항목 질문은 대상별로 따로 물어 값을 묶습니다(a+b). 묶음 신뢰도가 낮으면 하나만 고릅니다.")
}

// runeWidth 터미널 표시 폭. 한글·전각은 2칸.
// ponytail: 0x1100 이상을 모두 2칸으로 보는 근사. 이모지·결합 문자가 섞이면 어긋날 수 있다.
func runeWidth(r rune) int {
	if r >= 0x1100 {
		return 2
	}
	return 1
}

// truncWidth 표시 폭 기준으로 자르고 잘렸으면 … 를 붙인다.
func truncWidth(s string, width int) string {
	w := 0
	for i, r := range s {
		if w+runeWidth(r) > width-1 {
			total := 0
			for _, rr := range s {
				total += runeWidth(rr)
			}
			if total <= width {
				return s
			}
			return s[:i] + "…"
		}
		w += runeWidth(r)
	}
	return s
}

// padWidth 표시 폭 기준으로 오른쪽을 공백으로 채운다.
func padWidth(s string, width int) string {
	w := 0
	for _, r := range s {
		w += runeWidth(r)
	}
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}
