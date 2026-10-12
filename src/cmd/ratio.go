package cmd

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/clazic/kosis/internal/output"
	"github.com/spf13/cobra"
)

// kosis ratio — 서로 다른 두 표의 값을 이름·시점으로 맞춰 나눈다 (RSI R12·R20·R22 요청 4회).
// 각 쪽은 kosis d 인자 그대로 받아 이 실행 파일로 d 를 돌린다 — d 의 검사·안내를 그대로 쓴다.
var ratioCmd = &cobra.Command{
	Use:   "ratio",
	Short: "두 표의 값을 맞춰 나누기 (예: 농가인구 ÷ 총인구)",
	Long: `서로 다른 두 통계표의 값을 이름(지역 등)·시점으로 맞춰 나눕니다.

--num, --den 에는 kosis d 에 주는 인자를 그대로 씁니다 (ORG TBL -c… -i … -p … 시점).
행을 맞추는 기준:
  - 값이 여러 개인 분류 축의 이름 (시도별 등). 표마다 시도 이름이 달라도
    (강원도 / 강원특별자치도, 전라북도 / 전북특별자치도) 줄임말로 맞춘다
  - 양쪽 시점이 하나씩이면 시점은 무시 (다르면 stderr 로 알림), 여럿이면 같은 시점끼리
  - 이름이 겹치거나(여러 시도의 '중구') 항목(-i)이 둘 이상이면 exit 1

출력: 이름, 시점, 분자, 분모, 비율 (= 분자 ÷ 분모 × --scale). 맞지 않은 행은 양쪽 모두 stderr 로.`,
	Example: `  # 시도별 농가 비율(%) 상위 3 (level:1 에 섞인 전국·동부·읍부·면부는 !코드 로 뺀다)
  kosis ratio --num "101 DT_1AG25203 -c1 level:1+!00+!03+!04+!05 -c2 000 -i T00 -p F -l 1" \
              --den "101 DT_1IN1502 -c1 level:1+!00+!03+!04+!05 -i T100 -p Y -l 1" --sort desc --top 3

  # 주택 1호당 인구 (배수)
  kosis ratio --num "101 DT_1IN1502 -c1 00 -i T100 -p Y -l 1" --den "101 INH_1JU1501 -c1 00 -i T10 -p Y -l 1" --scale 1`,
	Args: cobra.NoArgs,
	RunE: runRatio,
}

var (
	ratioNum, ratioDen, ratioFormat, ratioSort string
	ratioScale                                 float64
	ratioTop                                   int
	ratioNoTotal                               bool
)

func init() {
	rootCmd.AddCommand(ratioCmd)
	ratioCmd.Flags().StringVar(&ratioNum, "num", "", "분자 쪽 kosis d 인자 (예 \"101 DT_1AG25203 -c1 level:1 -c2 000 -i T00 -p F -l 1\")")
	ratioCmd.Flags().StringVar(&ratioDen, "den", "", "분모 쪽 kosis d 인자")
	ratioCmd.Flags().Float64Var(&ratioScale, "scale", 100, "비율에 곱할 수 (100=%, 1=배수, 1000=천명당)")
	ratioCmd.Flags().StringVarP(&ratioFormat, "format", "f", "table", "출력 형식: table, json, csv, md")
	ratioCmd.Flags().StringVar(&ratioSort, "sort", "", "비율로 정렬: desc 또는 asc")
	ratioCmd.Flags().IntVar(&ratioTop, "top", 0, "--sort 뒤 앞 N행 (경계 동률 포함)")
	ratioCmd.Flags().BoolVar(&ratioNoTotal, "no-total", false, "이름이 전국·계·합계 등인 행을 뺀다")
}

type ratioSide struct {
	rows  []map[string]string
	key   string // 맞출 이름 열 (C1_NM 등), 없으면 ""
	multi bool   // 시점이 여럿
}

func runRatio(cmd *cobra.Command, args []string) error {
	cmd.SilenceUsage = true // 인자 오류도 한 줄 (사용법 전체로 묻지 않는다)
	if strings.TrimSpace(ratioNum) == "" || strings.TrimSpace(ratioDen) == "" {
		return fmt.Errorf("--num 과 --den 이 모두 필요합니다 (각각 kosis d 인자)")
	}
	if ratioSort != "" && ratioSort != "desc" && ratioSort != "asc" {
		return fmt.Errorf("--sort 는 desc 또는 asc 입니다")
	}
	if ratioTop != 0 && (ratioTop < 0 || ratioSort == "") {
		return fmt.Errorf("--top N 은 1 이상이고 --sort desc|asc 와 함께 씁니다")
	}
	for _, side := range []string{ratioNum, ratioDen} { // d 의 출력 모양을 바꾸는 옵션은 받지 않는다
		for _, a := range splitArgs(side) {
			for _, bad := range []string{"--pivot", "--fields", "-o", "--output", "--chart", "-f", "--format", "--sort", "--top", "--change", "--ratio"} {
				if a == bad || strings.HasPrefix(a, bad+"=") {
					return fmt.Errorf("--num/--den 에는 조회 인자(ORG TBL -c… -i … -p … 시점)만 — %s 는 뺍니다", bad)
				}
			}
		}
	}
	if ratioScale == 0 {
		return fmt.Errorf("--scale 은 0 이 아닌 수입니다")
	}
	num, err := ratioQuery("분자", ratioNum)
	if err != nil {
		return err
	}
	den, err := ratioQuery("분모", ratioDen)
	if err != nil {
		return err
	}
	joinPeriod := num.multi || den.multi
	if !joinPeriod {
		if pn, pd := num.rows[0]["PRD_DE"], den.rows[0]["PRD_DE"]; pn != pd {
			fmt.Fprintf(os.Stderr, "ℹ 분자 시점 %s, 분모 시점 %s — 시점이 하나씩이라 시점은 무시하고 맞췄습니다 (같은 해로 맞추려면 -s/-e)\n", pn, pd)
		}
	}
	keyOf := func(s ratioSide, r map[string]string) string { return ratioKey(s, r, joinPeriod) }
	for _, side := range []ratioSide{num, den} { // 이름이 겹치면(여러 시도의 '중구', 편입 전후 같은 이름) 짝이 틀린다 — 막는다
		seen := map[string]bool{}
		var dup []string
		for _, r := range side.rows {
			k := keyOf(side, r)
			if seen[k] {
				dup = append(dup, r[side.key])
			}
			seen[k] = true
		}
		if len(dup) > 0 {
			return fmt.Errorf("이름이 겹치는 행이 있어 짝을 맞출 수 없습니다: %s — 코드를 좁히거나(시도 하나의 시군구 등) d 로 따로 조회하세요", capList(dup, 5))
		}
	}
	denBy := map[string]map[string]string{}
	for _, r := range den.rows {
		denBy[keyOf(den, r)] = r
	}
	used := map[string]bool{}
	type outRow struct {
		name, period, n, d, ratio string
		v                         float64
		ok                        bool
	}
	var out []outRow
	var unmatched []string
	for _, r := range num.rows {
		d, ok := denBy[keyOf(num, r)] // 분자 행은 분자 쪽 이름 열로 (두 표의 축 번호가 달라도)
		if ok {
			used[keyOf(num, r)] = true
		}
		name := r[num.key]
		if num.key == "" {
			name = r["ITM_NM"]
		}
		if ratioNoTotal && totalName.MatchString(strings.TrimSpace(name)) {
			continue
		}
		if !ok {
			unmatched = append(unmatched, name)
			continue
		}
		nv, e1 := strconv.ParseFloat(strings.ReplaceAll(r["DT"], ",", ""), 64)
		dv, e2 := strconv.ParseFloat(strings.ReplaceAll(d["DT"], ",", ""), 64)
		o := outRow{name: name, period: r["PRD_DE"], n: r["DT"], d: d["DT"]}
		if e1 == nil && e2 == nil && dv != 0 {
			o.v, o.ok = nv/dv*ratioScale, true
			o.ratio = strconv.FormatFloat(o.v, 'f', 4, 64)
		}
		out = append(out, o)
	}
	if len(unmatched) > 0 {
		fmt.Fprintf(os.Stderr, "⚠ 분모에서 짝을 못 찾은 분자 행: %s — 이름·시점이 맞는지 확인하세요\n", capList(unmatched, 10))
	}
	var denOnly []string
	for _, r := range den.rows {
		if !used[keyOf(den, r)] && !(ratioNoTotal && totalName.MatchString(strings.TrimSpace(r[den.key]))) {
			denOnly = append(denOnly, r[den.key])
		}
	}
	if len(denOnly) > 0 && den.key != "" {
		fmt.Fprintf(os.Stderr, "⚠ 분자에서 짝을 못 찾은 분모 행: %s\n", capList(denOnly, 10))
	}
	if len(out) == 0 {
		return fmt.Errorf("맞춘 행이 없습니다 — 두 쪽의 이름 축·시점을 확인하세요")
	}
	fmt.Fprintf(os.Stderr, "ℹ 비율 = 분자(%s) ÷ 분모(%s) × %s\n", num.rows[0]["UNIT_NM"], den.rows[0]["UNIT_NM"], strconv.FormatFloat(ratioScale, 'f', -1, 64))
	if ratioSort != "" {
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].ok != out[j].ok {
				return out[i].ok
			}
			if ratioSort == "desc" {
				return out[i].v > out[j].v
			}
			return out[i].v < out[j].v
		})
		if ratioTop > 0 && len(out) > ratioTop {
			k := ratioTop
			for k < len(out) && out[k].ok && out[ratioTop-1].ok && out[k].v == out[ratioTop-1].v {
				k++
			}
			out = out[:k]
		}
	}
	data := make([]map[string]interface{}, len(out))
	for i, o := range out {
		data[i] = map[string]interface{}{"이름": o.name, "시점": o.period, "분자": o.n, "분모": o.d, "비율": o.ratio}
	}
	f, err := output.NewFormatter(ratioFormat)
	if err != nil {
		return err
	}
	return f.Format(data, output.FormatOptions{Columns: []string{"이름", "시점", "분자", "분모", "비율"}})
}

// ratioKey 행을 맞추는 키: 그 쪽(분자·분모) 이름 열의 줄임말 (+ 시점).
func ratioKey(s ratioSide, r map[string]string, withPeriod bool) string {
	k := ""
	if s.key != "" {
		k = nameKey(r[s.key])
	}
	if withPeriod {
		k += "|" + r["PRD_DE"]
	}
	return k
}

// ratioQuery 이 실행 파일로 kosis d … -f json --api-keys 를 돌려 행을 받는다. d 의 안내(stderr)는 앞에 [분자]/[분모] 를 붙여 넘긴다.
func ratioQuery(label, argLine string) (ratioSide, error) {
	self, err := os.Executable()
	if err != nil {
		return ratioSide{}, err
	}
	args := append(append([]string{"d"}, splitArgs(argLine)...), "-f", "json", "--api-keys")
	c := exec.Command(self, args...)
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	runErr := c.Run()
	for _, line := range strings.Split(strings.TrimRight(stderr.String(), "\n"), "\n") {
		if line != "" {
			fmt.Fprintf(os.Stderr, "[%s] %s\n", label, line)
		}
	}
	if runErr != nil {
		return ratioSide{}, fmt.Errorf("%s 쪽 조회 실패 (위 [%s] 줄 참고)", label, label)
	}
	var raw []map[string]interface{}
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil || len(raw) == 0 {
		return ratioSide{}, fmt.Errorf("%s 쪽 결과가 없습니다", label)
	}
	s := ratioSide{}
	periods := map[string]bool{}
	distinct := map[string]map[string]bool{}
	for _, m := range raw {
		r := map[string]string{}
		for k, v := range m {
			r[k] = fmt.Sprint(v)
			if strings.HasSuffix(k, "_NM") && k != "ITM_NM" && k != "UNIT_NM" {
				if distinct[k] == nil {
					distinct[k] = map[string]bool{}
				}
				distinct[k][r[k]] = true
			}
		}
		periods[r["PRD_DE"]] = true
		s.rows = append(s.rows, r)
	}
	s.multi = len(periods) > 1
	items := map[string]bool{}
	for _, r := range s.rows {
		items[r["ITM_ID"]] = true
		if _, ok := r["DT"]; !ok {
			return ratioSide{}, fmt.Errorf("%s 쪽 결과에 수치값(DT)이 없습니다", label)
		}
		if _, ok := r["PRD_DE"]; !ok {
			return ratioSide{}, fmt.Errorf("%s 쪽 결과에 시점(PRD_DE)이 없습니다", label)
		}
	}
	if len(items) > 1 {
		return ratioSide{}, fmt.Errorf("%s 쪽에 항목이 %d개입니다 — -i 는 하나만", label, len(items))
	}
	var varying []string
	for k, v := range distinct {
		if len(v) > 1 {
			varying = append(varying, k)
		}
	}
	sort.Strings(varying)
	if len(varying) > 1 {
		return ratioSide{}, fmt.Errorf("%s 쪽에 값이 여러 개인 분류 축이 %d개(%s)입니다 — 맞출 축 하나만 여러 코드로 두세요", label, len(varying), strings.Join(varying, ", "))
	}
	if len(varying) == 1 {
		s.key = varying[0]
	}
	return s, nil
}

// splitArgs 따옴표를 존중해 인자 문자열을 나눈다 ("-c1 \"11+26\"").
func splitArgs(s string) []string {
	var out []string
	var cur strings.Builder
	quote := rune(0)
	inTok := false
	for _, r := range s {
		switch {
		case quote != 0 && r == quote:
			quote = 0
		case quote == 0 && (r == '"' || r == '\''):
			quote, inTok = r, true
		case quote == 0 && (r == ' ' || r == '\t' || r == '\n'):
			if inTok {
				out = append(out, cur.String())
				cur.Reset()
				inTok = false
			}
		default:
			cur.WriteRune(r)
			inTok = true
		}
	}
	if inTok {
		out = append(out, cur.String())
	}
	return out
}

// nameKey 표마다 다른 시도 이름을 줄임말로 맞춘다 — 규칙만 쓴다(지역 목록 하드코딩 없음):
// '…특별자치도·특별자치시·특별시·광역시' 는 그 앞, 네 글자 '…도'(충청북도)는 1·3번째 글자, 세 글자 '…도'(경기도)는 앞 두 글자.
func nameKey(name string) string {
	n := strings.Join(strings.Fields(name), "")
	for _, suf := range []string{"특별자치도", "특별자치시", "특별시", "광역시"} {
		if strings.HasSuffix(n, suf) && utf8.RuneCountInString(n) > utf8.RuneCountInString(suf) {
			return strings.TrimSuffix(n, suf)
		}
	}
	r := []rune(n)
	if len(r) == 4 && r[3] == '도' {
		return string([]rune{r[0], r[2]})
	}
	if len(r) == 3 && r[2] == '도' {
		return string(r[:2])
	}
	return n
}
