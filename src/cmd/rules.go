package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/clazic/kosis/internal/rules"
	"github.com/spf13/cobra"
)

var rulesCmd = &cobra.Command{
	Use:   "rules",
	Short: "AI 에이전트용 규칙 보기·점검",
	Long: `kosis 를 쓰는 AI 에이전트가 s(검색) → m(메타) → d(데이터)로 판단할 때 쓰는 규칙을 다룹니다.

규칙은 두 곳에 있습니다:
  배포 규칙  스킬의 references/rules.yaml — 모든 사용자 공통, 검증된 것만 (저장소에서 관리)
  로컬 규칙  ~/.kosis/rules.d/*.yaml   — 이 컴퓨터에서 AI 가 확인한 사실 (규칙 하나 = 파일 하나)

로컬 규칙 파일 형식 (파일 이름은 YYYYMMDD-<8hex>.yaml, ASCII 만):
  kind: rule | table | concept | breakdown | crosswalk | gap
  title: 한 줄 제목 (한글 가능)
  verified: "YYYY-MM-DD"        확인 전이면 생략하고 confidence: guess
  evidence: 무엇을 실행해 무엇을 봤는지
  confidence: guess | tendency  (선택)
  status: wrong                 (선택, 틀린 것으로 판명 — 지우지 않음)
  … 종류별 필드 (배포 rules.yaml 의 해당 절 항목과 같음. table 은 table: "ORG/TBL")

하위 명령:
  list [찾을 말]   배포·로컬 규칙 목록 (-f json 이면 본문까지)
  add [파일]       규칙 한 항목을 rules.d 에 저장 (YAML, 파일을 안 주면 표준입력)
  sync             rules.d 를 비공개 git 저장소와 합치기 (여러 컴퓨터)
  validate         로컬 규칙과 배포 규칙의 형식 점검 (문제가 있으면 종료 코드 1)

예제:
  kosis rules list
  kosis rules list 울산
  kosis rules list --kind table -f json
  kosis rules add < rule.yaml
  kosis rules sync
  kosis rules validate

참고:
  - 배포 규칙 위치는 스킬 폴더에서 찾습니다. 다른 파일을 쓰려면 KOSIS_RULES_FILE 환경변수.
  - 이전 형식 ~/.kosis/rules.local.yaml 은 처음 실행 때 rules.d 로 옮겨지고 .migrated 로 남습니다.`,
	PersistentPreRun: func(cmd *cobra.Command, args []string) {
		if n, err := rules.MigrateLegacy(rules.LegacyLocalFile(), rules.Dir(), time.Now()); err != nil {
			fmt.Fprintln(os.Stderr, "경고:", err)
		} else if n > 0 {
			fmt.Fprintf(os.Stderr, "ℹ 이전 rules.local.yaml 의 %d개 항목을 %s 로 옮겼습니다 (원본: rules.local.yaml.migrated)\n", n, rules.Dir())
		}
	},
}

// shippedRulesPath 배포 rules.yaml 위치: KOSIS_RULES_FILE → 설치된 스킬 폴더. 못 찾으면 "".
func shippedRulesPath() string {
	if p := os.Getenv("KOSIS_RULES_FILE"); p != "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	for _, d := range collectSkillDirs(home) {
		p := filepath.Join(d, "references", "rules.yaml")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// loadAllRules 배포 + 로컬 규칙. 배포 파일을 못 찾으면 로컬만.
func loadAllRules() (shipped, local []rules.Entry, shippedPath string, localProbs []rules.FileProblem, err error) {
	shippedPath = shippedRulesPath()
	if shippedPath != "" {
		if shipped, err = rules.LoadShipped(shippedPath); err != nil {
			return nil, nil, shippedPath, nil, err
		}
	}
	local, localProbs = rules.LoadDir(rules.Dir())
	return shipped, local, shippedPath, localProbs, nil
}

var rulesListCmd = &cobra.Command{
	Use:   "list [찾을 말]",
	Short: "배포·로컬 규칙 목록",
	Long: `배포 규칙과 로컬 규칙을 함께 보여줍니다. 로컬이 더 최신입니다.

파라미터:
  [찾을 말]   제목·근거·본문에 든 말 (공백·대소문자 무시). 생략하면 전부.

플래그:
  --kind <종류>   rule | table | concept | breakdown | crosswalk | gap
  -f json         본문까지 JSON 으로 (AI 가 읽기 좋음)

표시:
  [배포]/[로컬] 출처, (추정) confidence: guess, (경향) tendency, (틀림) status: wrong

예제:
  kosis rules list 울산
  kosis rules list --kind rule
  kosis rules list DT_1FS1101 -f json`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		kind, _ := cmd.Flags().GetString("kind")
		format, _ := cmd.Flags().GetString("format")
		query := ""
		if len(args) == 1 {
			query = args[0]
		}
		shipped, local, shippedPath, _, err := loadAllRules()
		if err != nil {
			fmt.Println("오류:", err)
			return
		}
		var all []rules.Entry
		for _, e := range append(local, shipped...) { // 로컬 먼저 (더 최신)
			if (kind == "" || e.Kind == kind) && rules.Matches(e, query) {
				all = append(all, e)
			}
		}
		if format == "json" {
			type row struct {
				Source string         `json:"source"`
				File   string         `json:"file"`
				Entry  map[string]any `json:"rule"`
			}
			out := make([]row, 0, len(all))
			for _, e := range all {
				m := map[string]any{"kind": e.Kind, "title": e.Title, "verified": e.Verified, "evidence": e.Evidence}
				for k, v := range map[string]string{"confidence": e.Confidence, "status": e.Status, "host": e.Host} {
					if v != "" {
						m[k] = v
					}
				}
				for k, v := range e.Body {
					m[k] = v
				}
				out = append(out, row{Source: e.Source, File: e.File, Entry: m})
			}
			enc := json.NewEncoder(os.Stdout)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			_ = enc.Encode(out)
			return
		}
		if shippedPath == "" {
			fmt.Println("⚠ 배포 규칙(references/rules.yaml)을 찾지 못했습니다. 스킬을 설치하거나 KOSIS_RULES_FILE 을 지정하세요.")
		}
		for _, e := range all {
			src := "배포"
			if e.Source == "local" {
				src = "로컬"
			}
			var flags []string
			switch e.Confidence {
			case "guess":
				flags = append(flags, "추정")
			case "tendency":
				flags = append(flags, "경향")
			}
			if e.Status == "wrong" {
				flags = append(flags, "틀림")
			}
			mark := ""
			if len(flags) > 0 {
				mark = " (" + strings.Join(flags, ",") + ")"
			}
			fmt.Printf("[%s] %-9s %s%s  %s\n", src, e.Kind, e.Title, mark, e.Verified)
		}
		fmt.Printf("\n%d개 (배포 %d · 로컬 %d)", len(all), len(shipped), len(local))
		if shippedPath != "" {
			fmt.Printf("  배포: %s", shippedPath)
		}
		fmt.Printf("  로컬: %s\n", rules.Dir())
	},
}

var rulesAddCmd = &cobra.Command{
	Use:   "add [파일]",
	Short: "규칙 한 항목 저장",
	Long: `규칙 한 항목(YAML)을 ~/.kosis/rules.d 에 새 파일로 저장합니다.
파일 이름(YYYYMMDD-<8hex>.yaml)·host·created 는 kosis 가 정합니다. 형식이 틀리면 저장하지 않고 종료 코드 1.

파라미터:
  [파일]   YAML 파일. 생략하거나 - 이면 표준입력.

입력 형식 (kosis rules --help 참고):
  kind: table
  title: 울산 구군 코드 (사업체조사)
  verified: "2026-10-12"
  evidence: kosis m 101 DT_1FS1101 --class1 에서 확인
  table: "101/DT_1FS1101"
  … 종류별 필드

같은 종류에 제목이 같거나 같은 표를 가리키는 규칙이 있으면 알려 줍니다 (저장은 함).
기존 규칙이 틀렸으면 지우지 말고 그 파일에 status: wrong 을 적고, 맞는 내용을 새로 add 합니다.

예제:
  kosis rules add rule.yaml
  kosis rules add <<'EOF'
  kind: rule
  title: 분기 시점 형식
  when: 분기(--period Q) 조회
  do: 시점을 YYYY0Q 로 쓴다 (예 202501)
  verified: "2026-10-12"
  evidence: --start 20251 → [30], 202501 → 정상
  EOF`,
	Args: cobra.MaximumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		var data []byte
		var err error
		if len(args) == 0 || args[0] == "-" {
			data, err = io.ReadAll(os.Stdin)
		} else {
			data, err = os.ReadFile(args[0])
		}
		if err != nil {
			fmt.Fprintln(os.Stderr, "❌ 입력을 읽지 못했습니다:", err)
			os.Exit(1)
		}
		e, err := rules.Parse(data)
		if err != nil {
			fmt.Fprintln(os.Stderr, "❌", err)
			os.Exit(1)
		}
		shipped, local, _, _, _ := loadAllRules()
		path, err := rules.Write(rules.Dir(), e, time.Now())
		if err != nil {
			fmt.Fprintln(os.Stderr, "❌", err)
			os.Exit(1)
		}
		fmt.Println("✅ 저장:", path)
		for _, x := range rules.Similar(e, append(local, shipped...)) {
			src := "배포"
			if x.Source == "local" {
				src = "로컬 " + filepath.Base(x.File)
			}
			fmt.Fprintf(os.Stderr, "ℹ 비슷한 규칙 [%s] %s — 겹치면 낡은 쪽에 status: wrong\n", src, x.Title)
		}
	},
}

var rulesSyncCmd = &cobra.Command{
	Use:   "sync",
	Short: "로컬 규칙을 여러 컴퓨터와 합치기",
	Long: `~/.kosis/rules.d 를 비공개 git 저장소와 합칩니다: 로컬 변경 commit → 받아서 합치기 → 올리기.
규칙이 파일 하나씩이라 합치기는 합집합입니다. API 키·캐시·이력은 동기화하지 않습니다.

플래그:
  --init <저장소 URL>   처음 한 번: rules.d 를 그 저장소와 연결 (이미 있는 규칙도 합침)

준비 (처음 한 번, 저장소는 비공개로):
  gh repo create <계정>/kosis-rules --private
  kosis rules sync --init https://github.com/<계정>/kosis-rules.git

같은 규칙 파일을 두 컴퓨터에서 다르게 고쳤으면 합치지 않고 그 파일과 직접 합치는 방법을 알려 줍니다 (종료 코드 1).
git 이 필요하고, 인증은 git 설정(gh auth 등)을 그대로 씁니다.

예제:
  kosis rules sync --init https://github.com/clazic/kosis-rules.git
  kosis rules sync`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		initURL, _ := cmd.Flags().GetString("init")
		r, err := rules.Sync(rules.Dir(), initURL)
		if err != nil {
			fmt.Fprintln(os.Stderr, "❌", err)
			os.Exit(1)
		}
		fmt.Printf("✅ 동기화: %s — 올림 %d개, 받음 %d개\n", r.Remote, r.Committed, len(r.Pulled))
		if _, probs := rules.LoadDir(rules.Dir()); len(probs) > 0 {
			fmt.Fprintf(os.Stderr, "⚠ 형식 문제 %d건 — kosis rules validate\n", len(probs))
		}
	},
}

var rulesValidateCmd = &cobra.Command{
	Use:   "validate",
	Short: "규칙 형식 점검",
	Long: `로컬 규칙(~/.kosis/rules.d)과 배포 규칙의 형식을 점검합니다.
AI 가 로컬 규칙 파일을 쓴 뒤 실행해 형식을 확인합니다. 문제가 있으면 종료 코드 1.

점검: kind·title·evidence 필수, verified 는 YYYY-MM-DD(확인 전이면 confidence: guess),
      종류별 필수 필드(rule: when·do, table: table=ORG/TBL …), 파일 이름 YYYYMMDD-<8hex>.yaml

예제:
  kosis rules validate`,
	Args: cobra.NoArgs,
	Run: func(cmd *cobra.Command, args []string) {
		shipped, local, shippedPath, localProbs, err := loadAllRules()
		if err != nil {
			fmt.Println("❌ 배포 규칙을 읽지 못했습니다:", err)
			os.Exit(1)
		}
		bad := 0
		for _, p := range localProbs {
			bad++
			fmt.Printf("❌ %s\n", p.File)
			for _, m := range p.Problems {
				fmt.Printf("   - %s\n", m)
			}
		}
		for _, e := range shipped {
			if p := rules.Validate(e); len(p) > 0 {
				bad++
				fmt.Printf("❌ [배포] %s %q\n", e.Kind, e.Title)
				for _, m := range p {
					fmt.Printf("   - %s\n", m)
				}
			}
		}
		fmt.Printf("로컬 %d개, 배포 %d개 점검", len(local), len(shipped))
		if shippedPath == "" {
			fmt.Print(" (배포 규칙 파일 못 찾음)")
		}
		if bad > 0 {
			fmt.Printf(" — 문제 %d건\n", bad)
			os.Exit(1)
		}
		fmt.Println(" — 문제 없음")
	},
}

func init() {
	rootCmd.AddCommand(rulesCmd)
	rulesCmd.AddCommand(rulesListCmd)
	rulesCmd.AddCommand(rulesAddCmd)
	rulesCmd.AddCommand(rulesSyncCmd)
	rulesCmd.AddCommand(rulesValidateCmd)
	rulesSyncCmd.Flags().String("init", "", "처음 연결할 비공개 git 저장소 URL")
	rulesListCmd.Flags().String("kind", "", "종류: rule|table|concept|breakdown|crosswalk|gap")
	rulesListCmd.Flags().StringP("format", "f", "table", "출력 형식: table | json")
}
