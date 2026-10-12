package cmd

import (
	"encoding/json"
	"fmt"
	"html"
	"os"
	"strings"

	"github.com/clazic/kosis/internal/api"
	"github.com/clazic/kosis/internal/interactive"
	"github.com/spf13/cobra"
)

var explainCmd = &cobra.Command{
	Use:     "explain <ORG_ID> <TBL_ID>",
	Aliases: []string{"ex"},
	Short:   "통계 조사 설명",
	Long: `KOSIS 통계 조사 설명

통계 조사의 작성목적, 조사대상, 조사방법, 조사기간 등
방법론 정보를 확인합니다.

통계표의 주석(m --type CMMT)과 원표·조사설명 페이지 링크도 함께 보여줍니다.
설명이 없는 통계표도 원표 링크(kosis.kr)는 항상 나옵니다.

사용법:
  kosis explain <ORG_ID> <TBL_ID> [flags]
  kosis ex <ORG_ID> <TBL_ID>
  kosis explain                        대화형 모드

파라미터:
  <ORG_ID>                 기관 코드 (없으면 대화형)
  <TBL_ID>                 통계표 ID (없으면 대화형)

플래그:
  -f, --format <type>      출력 형식: table(기본), json

예제:
  # 인구총조사 설명
  kosis ex 101 DT_1IN1502

  # JSON 형식으로 출력
  kosis ex 101 DT_1IN1502 -f json

  # 대화형 모드
  kosis explain

출력 항목 (table):
  통계명, 승인번호, 작성목적, 조사대상 범위, 조사지역, 조사단위, 조사항목, 공표범위,
  작성주기, 공표시기, 조사기간, 자료수집방법, 근거법령, 주요 용어, 이용시 유의사항, 작성부서,
  주석, 링크(원표 kosis.kr / 조사설명 narastat.kr — 승인번호가 있을 때)
  -f json: {org_id, tbl_id, links{table, survey}, notes[], explain{API 필드 전부}}

빈 결과 시 대안:
  kosis m <ORG_ID> <TBL_ID>     메타데이터로 분류/항목/수록정보 확인

관련 명령어:
  kosis search <키워드>      통계표 검색
  kosis meta <ORG> <TBL>     분류/항목 코드 확인
  kosis data <ORG> <TBL>     데이터 조회`,
	Args: cobra.RangeArgs(0, 2),
	RunE: runExplain,
}

var explainFormat string

func init() {
	rootCmd.AddCommand(explainCmd)

	explainCmd.Flags().StringVarP(&explainFormat, "format", "f", "table", "출력 형식: table=정렬표, json=파이프/jq용")
}

func runExplain(cmd *cobra.Command, args []string) error {
	var orgID, tblID string
	if len(args) == 1 {
		return fmt.Errorf("인자가 1개만 제공되었습니다. ORG_ID와 TBL_ID를 모두 입력하세요: kosis explain <ORG_ID> <TBL_ID>")
	}
	if len(args) == 2 {
		orgID = args[0]
		tblID = args[1]
	}

	// API 클라이언트 (검색·메타·목록·설명 응답은 캐시)
	client, err := newCmdClient()
	if err != nil {
		return err
	}

	if len(args) == 0 {
		stat, statErr := os.Stdin.Stat()
		if statErr != nil {
			return fmt.Errorf("stdin 상태 확인 실패: %w", statErr)
		}
		if (stat.Mode() & os.ModeCharDevice) == 0 {
			return fmt.Errorf("인자 없이 실행하려면 대화형 터미널에서 실행하세요: kosis explain <ORG_ID> <TBL_ID>")
		}

		keyword := interactive.Prompt("? 검색어를 입력하세요:")
		if keyword == "" {
			return fmt.Errorf("검색어가 입력되지 않았습니다")
		}

		fmt.Fprint(os.Stderr, "  검색 중...\n")
		searchResults, err := client.Search(keyword, api.SearchOptions{ResultCount: 20})
		if err != nil {
			return fmt.Errorf("검색 실패: %w", err)
		}
		if len(searchResults) == 0 {
			return fmt.Errorf("검색 결과가 없습니다")
		}

		var tableOptions []string
		for _, r := range searchResults {
			tableOptions = append(tableOptions, fmt.Sprintf("%s (%s/%s)", r.TblNM, r.OrgID, r.TblID))
		}

		selectedIdx, _ := interactive.Select("? 통계표를 선택하세요:", tableOptions)
		if selectedIdx < 0 {
			return fmt.Errorf("통계표를 선택해야 합니다")
		}

		selected := searchResults[selectedIdx]
		orgID = selected.OrgID
		tblID = selected.TblID
	}

	// 통계설명 조회 + 주석(CMMT) + 원표·조사설명 링크
	expl, err := client.Explain(orgID, tblID)
	if err != nil && strings.Contains(err.Error(), "[30]") {
		err = nil // 통계설명이 없는 표 — 실패가 아니다 (주석·링크는 아래에 나온다)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "경고: 통계설명 조회 실패: %v\n", err)
	}
	notes := []string{}
	if cmmt, cerr := client.MetaRaw(orgID, tblID, "CMMT"); cerr == nil {
		for _, c := range cmmt {
			if dc := unescapeText(strings.TrimSpace(fmt.Sprint(c["CMMT_DC"]))); dc != "" && c["CMMT_DC"] != nil {
				notes = append(notes, dc)
			}
		}
	}
	for k, v := range expl {
		expl[k] = unescapeText(v)
	}
	links := map[string]string{"table": api.TableURL(orgID, tblID)}
	if u := api.ExplainURL(expl["confmNo"]); u != "" {
		links["survey"] = u
	}

	if explainFormat == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		return enc.Encode(map[string]any{
			"org_id": orgID, "tbl_id": tblID, "links": links, "notes": notes, "explain": expl,
		})
	}

	if len(expl) == 0 {
		fmt.Fprintf(os.Stderr, "해당 통계표(%s/%s)의 통계설명이 없습니다. 원표 페이지에서 확인하세요.\n", orgID, tblID)
	}
	for _, f := range explainFields {
		if v := expl[f[0]]; v != "" {
			fmt.Printf("[%s]\n%s\n\n", f[1], v)
		}
	}
	if len(notes) > 0 {
		fmt.Println("[주석]")
		for _, n := range notes {
			fmt.Println(n)
		}
		fmt.Println()
	}
	fmt.Println("[링크]")
	fmt.Println("원표      ", links["table"])
	if links["survey"] != "" {
		fmt.Println("조사설명  ", links["survey"])
	}
	fmt.Fprintf(os.Stderr, "\n(전체 %d개 필드: kosis ex %s %s -f json)\n", len(expl), orgID, tblID)
	return nil
}

// explainFields table 출력 순서 [API 필드, 표시 이름]. 나머지 필드는 -f json 에 모두 있다.
var explainFields = [][2]string{
	{"statsNm", "통계명"}, {"statsKind", "통계종류"}, {"confmNo", "승인번호"},
	{"writingPurps", "작성목적"}, {"examinObjrange", "조사대상 범위"}, {"examinObjArea", "조사지역"},
	{"josaUnit", "조사단위"}, {"josaItm", "조사항목"}, {"pubExtent", "공표범위"},
	{"statsPeriod", "작성주기"}, {"pubDate", "공표시기"}, {"examinPd", "조사기간"},
	{"examinTrgetPd", "조사대상기간"}, {"dataCollectMth", "자료수집방법"}, {"basisLaw", "근거법령"},
	{"mainTermExpl", "주요 용어"}, {"dataUserNote", "이용시 유의사항"}, {"writingTel", "작성부서"},
}

// unescapeText KOSIS 설명·주석의 HTML 엔티티(&lt; 와 두 번 인코딩된 &amp;lt;)를 글자로 되돌리고 태그(<b> 등)는 지운다.
func unescapeText(s string) string {
	for i := 0; i < 3 && strings.Contains(s, "&"); i++ {
		u := html.UnescapeString(s)
		if u == s {
			break
		}
		s = u
	}
	return htmlTag.ReplaceAllString(s, "")
}
