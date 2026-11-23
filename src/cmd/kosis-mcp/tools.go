package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/clazic/kosis/cmd"
	"github.com/clazic/kosis/internal/api"
)

// maxResultBytes 도구 결과 텍스트 상한 (약 200KB). 넘으면 행을 줄이고 "잘림" 안내를 붙인다.
const maxResultBytes = 200_000

// param 도구 인자 하나. JSON Schema 와 검증이 같은 정의를 쓴다.
type param struct {
	Name     string
	Type     string // "string" | "integer"
	Desc     string
	Required bool
	Pattern  string
	MaxLen   int
	Min, Max int
	Enum     []string
	Default  any
	Schema   map[string]any           // 있으면 이 JSON Schema (객체·여러 형식 인자)
	Check    func(v any) (any, error) // 있으면 이 검사로 (Type 대신)

	re *regexp.Regexp
}

type tool struct {
	Name        string
	Title       string
	Description string
	Params      []*param
	NoKeys      bool // 저장된 결과만 읽는 도구 — API 키가 없어도 된다
	run         func(rc *runCtx, in args) (*toolResult, error)
}

// args 검증을 통과한 인자 (문자열은 string, 정수는 int).
type args map[string]any

func (a args) str(k string) string {
	s, _ := a[k].(string)
	return s
}

func (a args) flag(k string) bool {
	b, _ := a[k].(bool)
	return b
}

func (a args) num(k string) int {
	n, _ := a[k].(int)
	return n
}

// 형식 패턴
const (
	patOrgID   = `^[A-Za-z0-9_]{1,20}$`
	patTblID   = `^[A-Za-z0-9_.\-]{1,60}$`
	patCodes   = `^[A-Za-z0-9_.+*!:,\-]+$` // ALL, 00+11, 11*, level:1+!03, T10,T20(→+)
	patPeriod  = `^([YMQHSFymqhsf]|년|연|월|분기|반기|[0-9]{1,2}년)$`
	patPrdDe   = `^[0-9]{4,8}$`
	patPeriods = `^[0-9]{4,8}( *, *[0-9]{4,8})*$`
	patText    = `^[^\x00-\x1f\x7f]+$` // 제어 문자 없는 한 줄
	patCode    = `^[A-Za-z0-9_.\-]{1,40}$`
	patView    = `^MT_[A-Z0-9_]{1,20}$`
)

var tools = []*tool{
	{
		Name:  "kosis_search",
		Title: "KOSIS 통계표 검색",
		Description: "KOSIS 통계표를 이름으로 검색해 org_id(기관)·tbl_id(통계표)를 찾는다 (kosis s). " +
			"검색어에 연도·숫자 코드를 넣지 않는다(연도는 결과가 없을 때만 빼고 다시 검색). 0건이면 조사명(예 '광업제조업조사')이나 kosis_list 로 찾는다 — 못 찾았다고 KOSIS 에 없다고 단정하지 않는다. " +
			"다음 단계: kosis_meta 로 분류·항목 코드와 주기 확인.",
		Params: []*param{
			{Name: "keyword", Type: "string", Required: true, MaxLen: 100, Pattern: patText, Desc: "통계표 이름 검색어 (예: 인구, 미분양, 소비자물가)"},
			{Name: "limit", Type: "integer", Min: 1, Max: 100, Default: 20, Desc: "결과 개수 (1~100)"},
			{Name: "page", Type: "integer", Min: 1, Max: 1000, Default: 1, Desc: "결과 쪽 번호 (1부터)"},
			{Name: "sort", Type: "string", Enum: []string{"RANK", "DATE"}, Default: "RANK", Desc: "RANK=정확도, DATE=최신"},
			{Name: "org", Type: "string", MaxLen: 40, Pattern: patText, Desc: "기관 코드(101) 또는 기관명 일부(울산) — 서버에서 거른다"},
			{Name: "stat", Type: "string", MaxLen: 60, Pattern: patText, Desc: "통계조사명 일부로 거르기 (예: 광업제조업조사)"},
			{Name: "region", Type: "string", MaxLen: 40, Pattern: patText, Desc: "표 이름·기관·조사명·경로에 이 지역 이름이 든 것만 (전국 시도별 표는 빠짐)"},
		},
		run: runSearch,
	},
	{
		Name:  "kosis_meta",
		Title: "KOSIS 통계표 메타(분류·항목·주기)",
		Description: "통계표의 분류 축(class1…class8 에 넣을 코드), 항목(item), 수록주기·시점 범위(period)를 본다 (kosis m). kosis_data 전에 반드시 확인한다 — 표마다 축 수·코드가 다르고, 코드는 짐작하지 않는다(한글 이름을 넣으면 오류 21). " +
			"인자 없이: 요약(큰 축은 위 단계 50개까지). find/under/level/axis: 코드 고르기 — 결과의 data_params 를 kosis_data 에 그대로 넣는다. type: 원본 메타(CMMT=주석, UNIT=단위, SOURCE=출처, PRD=수록정보 …). " +
			"'특성별'처럼 최상위가 전체·성별·연령별인 묶음 축은 ALL 대신 under 로 하위만 고른다.",
		Params: []*param{
			{Name: "org_id", Type: "string", Required: true, Pattern: patOrgID, Desc: "기관 코드 (예: 101)"},
			{Name: "tbl_id", Type: "string", Required: true, Pattern: patTblID, Desc: "통계표 ID (예: DT_1IN1502)"},
			{Name: "type", Type: "string", Enum: []string{"ITM", "PRD", "TBL", "ORG", "CMMT", "UNIT", "SOURCE", "WGT", "NCD"}, Desc: "원본 메타 유형 (없으면 요약)"},
			{Name: "find", Type: "string", MaxLen: 200, Pattern: patText, Desc: "이름으로 코드 찾기 (쉼표로 여러 개, 공백 무시. 예: 울산,울주)"},
			{Name: "under", Type: "string", Pattern: patCode, Desc: "이 코드 바로 아래 코드들 (예: 26 → 울산의 구·군)"},
			{Name: "level", Type: "integer", Min: 1, Max: 20, Desc: "이 단계의 코드만 (1=최상위)"},
			{Name: "axis", Type: "string", MaxLen: 40, Pattern: patText, Desc: "이 축의 코드만: item, class1…class8, 또는 분류 이름 일부(예: 산업)"},
		},
		run: runMeta,
	},
	{
		Name:  "kosis_data",
		Title: "KOSIS 통계표 데이터 조회",
		Description: "통계표의 값을 조회한다 (kosis d). 먼저 kosis_meta 로 코드를 확인한다. 표의 분류 축 수만큼 class1…classN 이 모두 필요하다(빠지거나 없는 축이면 조회 전에 알려 준다). " +
			"코드 규칙: ALL=전체, \"00+11\"=여러 개, \"11*\"=하위 전체(상위 포함), level:N=그 축 N단계 전부(\"level:1+!03\" 은 03 제외). " +
			"시점은 하나만 고른다: latest(최근 N개) | start+end(연속) | periods(\"2020,2022\") — 없으면 최근 1개. 4만 셀을 넘으면 자동 분할 조회한다. " +
			"결과 notices 는 조용한 오답 경고(결과에 없는 코드·일찍 끊긴 코드·잠정치·전망 시점·모두 0·단위 없음)다 — 답에 반영한다. 안내 문구의 -cN=classN, -i=item, -p=period, kosis m=kosis_meta. " +
			"rows 는 API 키(C1·C1_NM … ITM_ID·ITM_NM·PRD_DE·DT·UNIT_NM), labels 는 이 표의 열 이름. X(비공개)·빈 값은 그대로 두고 추정하지 않는다. " +
			"계산은 도구에 맡긴다(눈으로 정렬하거나 손으로 빼고 나누지 않는다): 순위 sort=desc|asc + top=N (RANK 열, 합계 행은 no_total=true), 증감 change=yoy|prev (DT_CHG·DT_CHG_PCT 열, 비교 시점까지 받는다 — 월 yoy 는 latest 13), 같은 축 두 코드 비율 ratio=\"분자/분모\" (DT_RATIO_PCT, ×100). 다른 두 표의 비율은 kosis_ratio. 계산은 filter·aggregate·pivot 보다 먼저 한다. " +
			"큰 표(약 200KB 초과)는 저장되어 첫 쪽(format=columns)·next_cursor(kosis_result_page)·result_id(kosis_result_query)·CSV download 를 준다 — 큰 표는 먼저 latest/filter/aggregate/pivot 으로 줄이고, 전체가 필요하면 download 를 쓴다.",
		Params: append([]*param{
			{Name: "org_id", Type: "string", Required: true, Pattern: patOrgID, Desc: "기관 코드 (예: 101)"},
			{Name: "tbl_id", Type: "string", Required: true, Pattern: patTblID, Desc: "통계표 ID (예: DT_1IN1502)"},
			{Name: "class1", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류1 코드 (kosis_meta axes 의 class1)"},
			{Name: "class2", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류2 코드"},
			{Name: "class3", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류3 코드"},
			{Name: "class4", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류4 코드"},
			{Name: "class5", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류5 코드"},
			{Name: "class6", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류6 코드"},
			{Name: "class7", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류7 코드"},
			{Name: "class8", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류8 코드"},
			{Name: "item", Type: "string", Required: true, MaxLen: 4000, Pattern: patCodes, Desc: "항목 코드 (ALL, \"T10+T20\")"},
			{Name: "period", Type: "string", Required: true, Pattern: patPeriod, Desc: "수록주기: Y=년, M=월, Q=분기, H=반기, F=2년 이상 여러 해 (kosis_meta periods 의 period)"},
			{Name: "start", Type: "string", Pattern: patPrdDe, Desc: "시작 시점 (end 와 함께. 연 2020, 월 202401, 분기 202401)"},
			{Name: "end", Type: "string", Pattern: patPrdDe, Desc: "종료 시점 (start 와 함께)"},
			{Name: "latest", Schema: map[string]any{"oneOf": []any{map[string]any{"type": "integer", "minimum": 1, "maximum": 1000}, map[string]any{"type": "boolean"}}}, Check: checkLatest,
				Desc: "정수 N: 표의 최근 N개 시점을 받는다. true: 받은 뒤 계열(코드 조합)마다 가장 최근 시점 행만 남긴다 (start/end 와 함께 — 코드마다 끝 시점이 달라도)"},
			{Name: "periods", Type: "string", MaxLen: 2000, Pattern: patPeriods, Desc: "비연속 시점 (쉼표, 예: 2020,2022,2025)"},
			{Name: "fields", Type: "string", MaxLen: 500, Pattern: patText, Desc: "남길 열 (쉼표, API 키 또는 열 이름. 예: C1_NM,PRD_DE,DT) — 임의로 줄이지 않는다"},
			{Name: "sort", Type: "string", Enum: []string{"desc", "asc"}, Desc: "수치값으로 정렬하고 RANK(순위) 열을 붙인다 (항목·시점 묶음마다 따로, 동률은 같은 순위). change 와 함께면 증감 기준"},
			{Name: "top", Type: "integer", Min: 1, Max: 100000, Desc: "정렬 뒤 앞 N행 (sort 와 함께, 경계 동률은 함께)"},
			{Name: "sort_by", Type: "string", Enum: []string{"value", "change", "pct", "ratio"}, Desc: "sort 기준: value=수치값, change=증감, pct=증감률(%), ratio=비율(%)"},
			{Name: "change", Type: "string", Enum: []string{"yoy", "prev"}, Desc: "같은 계열의 비교 시점 대비 증감 DT_CHG·증감률 DT_CHG_PCT 열: yoy=1년 전 같은 달·분기·해, prev=직전 시점. 비교 기준으로만 쓴 앞 시점 행은 빠진다"},
			{Name: "ratio", Type: "string", MaxLen: 500, Pattern: `^[A-Za-z0-9_.+*,\-]+/[A-Za-z0-9_.*\-]+$`, Desc: "같은 축 두 코드의 비율 DT_RATIO_PCT(분자÷분모×100): \"T2/T1\", \"ALL/00\"(그 축 모든 행의 합계 대비), \"Y70+Y75/000\"(구간 합 — 합계 % 는 notices). 분모 코드도 조회에 넣는다"},
			{Name: "no_total", Type: "boolean", Desc: "true: 여러 코드를 준 축에서 이름이 전국·계·합계·총계·소계·전체인 행을 뺀다 (순위에서 합계 빼기)"},
		}, bigResultParams()...),
		run: runData,
	},
	{
		Name:  "kosis_result_page",
		Title: "저장된 결과의 다음 쪽",
		Description: "큰 결과(kosis_data·kosis_result_query)의 다음 쪽을 같은 형식으로 받는다 — 다시 조회하지 않는다. cursor 는 앞 응답의 next_cursor. 마지막 쪽에는 next_cursor 가 없다. " +
			"API 키가 필요 없다. 결과는 1시간 뒤 만료된다(그러면 다시 조회). 전체가 필요하면 쪽을 다 넘기기보다 download(CSV)를 쓴다.",
		NoKeys: true,
		Params: []*param{
			{Name: "cursor", Type: "string", Required: true, MaxLen: 300, Pattern: `^[A-Za-z0-9_-]+$`, Desc: "앞 응답의 next_cursor"},
			{Name: "limit", Type: "integer", Min: 1, Max: 100000, Desc: "이 쪽 최대 행 수 (없으면 약 200KB 에 드는 만큼)"},
		},
		run: runResultPage,
	},
	{
		Name:  "kosis_result_query",
		Title: "저장된 결과 줄이기·집계",
		Description: "저장된 결과(result_id)에 다시 조회 없이 filter → latest → aggregate → order·top → pivot 을 건다. 큰 표는 이것으로 먼저 줄여서 본다. API 키가 필요 없다. " +
			"결과가 크면 다시 저장해 새 result_id·next_cursor·CSV 를 준다.",
		NoKeys: true,
		Params: append([]*param{
			{Name: "result_id", Type: "string", Required: true, Pattern: `^[A-Za-z0-9_-]{22,64}$`, Desc: "앞 응답의 result_id"},
			{Name: "latest", Schema: map[string]any{"type": "boolean"}, Check: checkLatestBool, Desc: "true: 계열(코드 조합)마다 가장 최근 시점(PRD_DE) 행만"},
			{Name: "limit", Type: "integer", Min: 1, Max: 100000, Desc: "첫 쪽 최대 행 수"},
			{Name: "order", Type: "string", Enum: []string{"desc", "asc"}, Desc: "aggregate 뒤(pivot 앞)에 order_by 열의 숫자로 정렬 (숫자 아닌 값은 맨 뒤)"},
			{Name: "order_by", Type: "string", MaxLen: 80, Pattern: patText, Desc: "정렬할 열 (API 키 또는 열 이름, 기본 DT — aggregate 뒤에는 그 결과 값 열)"},
			{Name: "top", Type: "integer", Min: 1, Max: 100000, Desc: "정렬 뒤 앞 N행 (order 와 함께, 경계 동률은 함께)"},
		}, bigResultParams()...),
		run: runResultQuery,
	},
	{
		Name:  "kosis_batch",
		Title: "KOSIS 여러 조회를 한 번에",
		Description: "서로 독립인 kosis_data 조회 여러 개(최대 10)를 한 번에 보내 동시에 받는다 — 연쇄 질문에서 다음 단계가 앞 결과에 기대지 않는 조회(같은 지역의 인구·화재·출생아, 여러 시도의 값 등)는 한 호출로 묶어 왕복을 줄인다. " +
			"queries 의 각 항목은 kosis_data 와 같은 조회 인자(org_id·tbl_id·class1…·item·period·시점)와 계산 인자(sort·top·no_total·change·ratio·sort_by·fields). " +
			"results 는 순서대로 {index, org_id, tbl_id, rows(API 키), notices} 또는 {index, error} — 한 조회가 실패해도 나머지는 온다. 조회마다 행이 많으면 앞 200행만 (그 이상은 kosis_data). 앞 결과로 다음 조회가 정해지면 묶지 말고 차례로 부른다.",
		Params: []*param{
			{Name: "queries", Required: true, Schema: batchSchema(), Check: checkBatch, Desc: "조회 목록 (1~10개, 각 항목은 kosis_data 인자)"},
		},
		run: runBatch,
	},
	{
		Name:  "kosis_ratio",
		Title: "KOSIS 두 조회의 비율",
		Description: "두 조회(다른 두 표, 또는 같은 표의 두 코드·항목)의 값을 이름·시점으로 맞춰 나눈다 (kosis ratio) — 농가인구÷총인구, 종사자÷사업체, 미분양÷세대수처럼 손으로 나누지 않는다. " +
			"num·den 은 kosis_data 와 같은 조회 인자(org_id·tbl_id·class1…·item·period·시점). 각 쪽 item 은 하나, 여러 코드를 주는 축도 하나만(그 축 이름으로 짝을 맞춘다 — 시도 이름이 표마다 달라도 줄임말로 맞춤). " +
			"scale: 100=%(기본), 1=배수·1인당, 1000=천명당, 10000=만명당. sort·top 으로 순위(RANK). 짝을 못 찾은 행·이름이 겹쳐 못 맞추는 경우는 notices·오류로 알린다. " +
			"rows: NAME·PRD_DE·NUM·DEN·RATIO.",
		Params: []*param{
			{Name: "num", Required: true, Schema: querySchema(), Check: checkQuery, Desc: "분자 조회 (kosis_data 의 조회 인자)"},
			{Name: "den", Required: true, Schema: querySchema(), Check: checkQuery, Desc: "분모 조회 (kosis_data 의 조회 인자)"},
			{Name: "scale", Schema: map[string]any{"type": "number"}, Check: checkScale, Desc: "곱할 수: 100=%(기본), 1=배수, 1000=천명당"},
			{Name: "no_total", Type: "boolean", Desc: "true: 이름이 전국·계·합계인 행을 뺀다"},
			{Name: "sort", Type: "string", Enum: []string{"desc", "asc"}, Desc: "비율로 정렬하고 RANK 를 붙인다"},
			{Name: "top", Type: "integer", Min: 1, Max: 100000, Desc: "정렬 뒤 앞 N행 (sort 와 함께)"},
		},
		run: runRatio,
	},
	{
		Name:  "kosis_rules",
		Title: "KOSIS 검증된 규칙·표",
		Description: "kosis 로 조회하며 확인해 둔 지식을 찾는다 (kosis rules list): 통계표별 검증 코드·함정(kind=table), 작업·검색·시점·답변 규칙(kind=rule), 예전에 못 찾은 것(kind=gap). " +
			"kosis_search·kosis_meta·kosis_data 결과의 rules·notices 에 관련 규칙이 함께 오지만, 표를 고르기 전이나 지표 정의가 헷갈릴 때(보유율/점유율, 생산액/출하액, 상승률/등락률) 직접 찾아본다. " +
			"query 는 제목·본문 어디든 든 말, id 는 규칙 id 또는 표 ID(ORG/TBL) 와 정확히 같은 것. API 키가 필요 없다.",
		NoKeys: true,
		Params: []*param{
			{Name: "query", Type: "string", MaxLen: 100, Pattern: patText, Desc: "찾을 말 (예: 소상공인, 환율, 인허가)"},
			{Name: "id", Type: "string", MaxLen: 80, Pattern: `^[A-Za-z0-9_./\-]+$`, Desc: "규칙 id 또는 표 ID (예: 116/DT_MLTM_2086, meta.merged-region)"},
			{Name: "kind", Type: "string", Enum: []string{"rule", "table", "concept", "breakdown", "crosswalk", "gap"}, Desc: "종류"},
			{Name: "topic", Type: "string", MaxLen: 30, Pattern: `^[a-z]+$`, Desc: "rule 의 주제: workflow, search, meta, data, period, answer, output, error"},
		},
		run: runRules,
	},
	{
		Name:        "kosis_list",
		Title:       "KOSIS 통계목록 트리",
		Description: "KOSIS 통계목록을 한 단계씩 내려가며 본다 (kosis ls). parent 에 앞 결과의 LIST_ID 를 넣는다. 표 단계에 오면 ORG_ID·TBL_ID 가 나온다. 검색으로 못 찾을 때 쓴다.",
		Params: []*param{
			{Name: "view", Type: "string", Pattern: patView, Default: "MT_ZTITLE", Desc: "서비스뷰: MT_ZTITLE=주제별, MT_OTITLE=기관별, MT_RTITLE=국제, MT_BUKHAN=북한, MT_GTITLE01=e-지방지표 …"},
			{Name: "parent", Type: "string", Pattern: patCode, Desc: "펼칠 상위 LIST_ID (없으면 최상위)"},
		},
		run: runList,
	},
	{
		Name:        "kosis_explain",
		Title:       "KOSIS 통계조사 설명",
		Description: "통계표가 속한 조사의 설명(작성목적·조사대상 범위·공표범위·주기·승인번호 등), 표 주석, 원표·조사설명 링크 (kosis ex). 답에 조사명·범위·제외 대상·단위를 밝힐 때 쓴다.",
		Params: []*param{
			{Name: "org_id", Type: "string", Required: true, Pattern: patOrgID, Desc: "기관 코드"},
			{Name: "tbl_id", Type: "string", Required: true, Pattern: patTblID, Desc: "통계표 ID"},
		},
		run: runExplain,
	},
	{
		Name:  "kosis_indicator",
		Title: "KOSIS 주요지표(e-나라지표)",
		Description: "통계주요지표 조회 (kosis ind). action: search=지표명 검색(name), data=지표 값 시계열(name, limit=최근 시점 수), info=지표 설명(id), list=지표 목록(list_id, 없으면 최상위). " +
			"지표명은 좁다 — 없으면 kosis_search 로 통계표를 찾는다.",
		Params: []*param{
			{Name: "action", Type: "string", Required: true, Enum: []string{"search", "data", "info", "list"}, Desc: "search | data | info | list"},
			{Name: "name", Type: "string", MaxLen: 100, Pattern: patText, Desc: "지표명 (search·data)"},
			{Name: "id", Type: "string", Pattern: patCode, Desc: "지표 ID (info)"},
			{Name: "list_id", Type: "string", Pattern: patCode, Desc: "지표 목록 ID (list)"},
			{Name: "limit", Type: "integer", Min: 1, Max: 100, Default: 10, Desc: "결과 개수(search) / 최근 시점 수(data)"},
		},
		run: runIndicator,
	},
}

// queryParams kosis_ratio 의 num·den 안에 들어가는 조회 인자 (kosis_data 의 조회 부분과 같다).
var queryParams = []*param{
	{Name: "org_id", Type: "string", Required: true, Pattern: patOrgID, Desc: "기관 코드"},
	{Name: "tbl_id", Type: "string", Required: true, Pattern: patTblID, Desc: "통계표 ID"},
	{Name: "class1", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류1 코드"},
	{Name: "class2", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류2 코드"},
	{Name: "class3", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류3 코드"},
	{Name: "class4", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류4 코드"},
	{Name: "class5", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류5 코드"},
	{Name: "class6", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류6 코드"},
	{Name: "class7", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류7 코드"},
	{Name: "class8", Type: "string", MaxLen: 4000, Pattern: patCodes, Desc: "분류8 코드"},
	{Name: "item", Type: "string", Required: true, MaxLen: 200, Pattern: patCodes, Desc: "항목 코드 (하나)"},
	{Name: "period", Type: "string", Required: true, Pattern: patPeriod, Desc: "수록주기 Y·M·Q·H·F"},
	{Name: "start", Type: "string", Pattern: patPrdDe, Desc: "시작 시점 (end 와 함께)"},
	{Name: "end", Type: "string", Pattern: patPrdDe, Desc: "종료 시점"},
	{Name: "latest", Type: "integer", Min: 1, Max: 1000, Desc: "표의 최근 N개 시점"},
	{Name: "periods", Type: "string", MaxLen: 2000, Pattern: patPeriods, Desc: "비연속 시점 (쉼표)"},
}

// querySchema num·den 의 JSON Schema.
func querySchema() map[string]any {
	props, required := paramSchemas(queryParams)
	return map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}
}

// checkQuery num·den 객체를 queryParams 로 검사한다.
func checkQuery(v any) (any, error) {
	if _, ok := v.(map[string]any); !ok {
		return nil, fmt.Errorf("조회 인자 객체여야 합니다 (org_id, tbl_id, class1…, item, period, 시점)")
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return validateArgs(&tool{Params: queryParams}, raw)
}

// batchParams kosis_batch 의 한 조회 = 조회 인자 + 계산 인자.
var batchParams = append(append([]*param{}, queryParams...),
	&param{Name: "fields", Type: "string", MaxLen: 500, Pattern: patText, Desc: "남길 열 (쉼표)"},
	&param{Name: "sort", Type: "string", Enum: []string{"desc", "asc"}, Desc: "정렬·RANK"},
	&param{Name: "top", Type: "integer", Min: 1, Max: 100000, Desc: "정렬 뒤 앞 N행"},
	&param{Name: "sort_by", Type: "string", Enum: []string{"value", "change", "pct", "ratio"}, Desc: "정렬 기준"},
	&param{Name: "change", Type: "string", Enum: []string{"yoy", "prev"}, Desc: "증감"},
	&param{Name: "ratio", Type: "string", MaxLen: 500, Pattern: `^[A-Za-z0-9_.+*,\-]+/[A-Za-z0-9_.*\-]+$`, Desc: "같은 축 두 코드 비율"},
	&param{Name: "no_total", Type: "boolean", Desc: "합계 행 빼기"},
)

const maxBatchQueries, batchRowCap = 10, 200

func batchSchema() map[string]any {
	props, required := paramSchemas(batchParams)
	return map[string]any{"type": "array", "minItems": 1, "maxItems": maxBatchQueries,
		"items": map[string]any{"type": "object", "properties": props, "required": required, "additionalProperties": false}}
}

// checkBatch queries 배열을 항목마다 batchParams 로 검사한다.
func checkBatch(v any) (any, error) {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 || len(arr) > maxBatchQueries {
		return nil, fmt.Errorf("1~%d개의 조회 객체 배열이어야 합니다", maxBatchQueries)
	}
	out := make([]args, len(arr))
	for i, x := range arr {
		if _, ok := x.(map[string]any); !ok {
			return nil, fmt.Errorf("%d번째: 조회 인자 객체여야 합니다", i+1)
		}
		raw, err := json.Marshal(x)
		if err != nil {
			return nil, err
		}
		a, err := validateArgs(&tool{Params: batchParams}, raw)
		if err != nil {
			return nil, fmt.Errorf("%d번째: %v", i+1, err)
		}
		out[i] = a
	}
	return out, nil
}

func checkScale(v any) (any, error) {
	n, ok := v.(json.Number)
	if !ok {
		return nil, fmt.Errorf("숫자여야 합니다 (100=%%, 1=배수, 1000=천명당)")
	}
	f, err := n.Float64()
	if err != nil || f == 0 || math.IsInf(f, 0) || math.IsNaN(f) {
		return nil, fmt.Errorf("0 이 아닌 수여야 합니다 (받은 값: %s)", n)
	}
	return f, nil
}

func queryOf(in args) cmd.DataQuery {
	q := cmd.DataQuery{OrgID: in.str("org_id"), TblID: in.str("tbl_id"), Item: in.str("item"), Period: in.str("period"),
		Start: in.str("start"), End: in.str("end"), Latest: in.num("latest"), Periods: in.str("periods")}
	for i := range q.Classes {
		q.Classes[i] = in.str(fmt.Sprintf("class%d", i+1))
	}
	return q
}

func init() {
	for _, p := range batchParams { // queryParams 를 포함한다
		if p.Pattern != "" {
			p.re = regexp.MustCompile(p.Pattern)
		}
	}
	for _, t := range tools {
		for _, p := range t.Params {
			if p.Pattern != "" {
				p.re = regexp.MustCompile(p.Pattern)
			}
		}
	}
}

func (s *server) toolByName(name string) (*tool, bool) {
	for _, t := range s.tools {
		if t.Name == name {
			return t, true
		}
	}
	return nil, false
}

// toolList tools/list 응답의 도구 정의 (JSON Schema 포함).
func (s *server) toolList() []map[string]any {
	var out []map[string]any
	for _, t := range s.tools {
		props, required := paramSchemas(t.Params)
		out = append(out, map[string]any{
			"name":        t.Name,
			"title":       t.Title,
			"description": t.Description,
			"inputSchema": map[string]any{
				"type": "object", "properties": props, "required": required, "additionalProperties": false,
			},
			"annotations": map[string]any{"readOnlyHint": true, "openWorldHint": true, "title": t.Title},
		})
	}
	return out
}

// paramSchemas 인자 정의 → JSON Schema properties 와 required.
func paramSchemas(ps []*param) (map[string]any, []string) {
	props := map[string]any{}
	required := []string{}
	for _, p := range ps {
		s := map[string]any{"type": p.Type, "description": p.Desc}
		if p.Schema != nil {
			s = map[string]any{"description": p.Desc}
			for k, v := range p.Schema {
				s[k] = v
			}
		}
		if p.Pattern != "" {
			s["pattern"] = p.Pattern
		}
		if p.MaxLen > 0 {
			s["maxLength"] = p.MaxLen
		}
		if p.Type == "integer" && (p.Min != 0 || p.Max != 0) {
			s["minimum"], s["maximum"] = p.Min, p.Max
		}
		if len(p.Enum) > 0 {
			s["enum"] = p.Enum
		}
		if p.Default != nil {
			s["default"] = p.Default
		}
		props[p.Name] = s
		if p.Required {
			required = append(required, p.Name)
		}
	}
	return props, required
}

// validateArgs JSON Schema 와 같은 규칙으로 인자를 검사하고 기본값을 채운다.
func validateArgs(t *tool, raw json.RawMessage) (args, error) {
	in := map[string]any{}
	if len(bytes.TrimSpace(raw)) > 0 && string(bytes.TrimSpace(raw)) != "null" {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if err := dec.Decode(&in); err != nil {
			return nil, fmt.Errorf("arguments 는 JSON 객체여야 합니다")
		}
	}
	known := map[string]*param{}
	for _, p := range t.Params {
		known[p.Name] = p
	}
	var unknown []string
	for k := range in {
		if known[k] == nil {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		var names []string
		for _, p := range t.Params {
			names = append(names, p.Name)
		}
		return nil, fmt.Errorf("모르는 인자: %s (쓸 수 있는 인자: %s)", strings.Join(unknown, ", "), strings.Join(names, ", "))
	}
	out := args{}
	for _, p := range t.Params {
		v, ok := in[p.Name]
		if !ok || v == nil {
			if p.Required {
				return nil, fmt.Errorf("%s 는 필수입니다", p.Name)
			}
			if p.Default != nil {
				out[p.Name] = p.Default
			}
			continue
		}
		if p.Check != nil {
			cv, err := p.Check(v)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", p.Name, err)
			}
			if cv != nil {
				out[p.Name] = cv
			}
			continue
		}
		switch p.Type {
		case "boolean":
			b, ok := v.(bool)
			if !ok {
				return nil, fmt.Errorf("%s 는 true/false 여야 합니다", p.Name)
			}
			out[p.Name] = b
		case "string":
			s, ok := v.(string)
			if !ok {
				return nil, fmt.Errorf("%s 는 문자열이어야 합니다", p.Name)
			}
			s = strings.TrimSpace(s)
			if s == "" {
				if p.Required {
					return nil, fmt.Errorf("%s 는 비울 수 없습니다", p.Name)
				}
				continue
			}
			if p.MaxLen > 0 && utf8.RuneCountInString(s) > p.MaxLen {
				return nil, fmt.Errorf("%s 는 %d자 이하여야 합니다", p.Name, p.MaxLen)
			}
			if len(p.Enum) > 0 {
				hit := ""
				for _, e := range p.Enum {
					if strings.EqualFold(e, s) {
						hit = e
					}
				}
				if hit == "" {
					return nil, fmt.Errorf("%s 는 %s 중 하나여야 합니다 (받은 값: %q)", p.Name, strings.Join(p.Enum, ", "), s)
				}
				s = hit
			}
			if p.re != nil && !p.re.MatchString(s) {
				return nil, fmt.Errorf("%s 형식이 맞지 않습니다 (받은 값: %q, 형식: %s)", p.Name, s, p.Pattern)
			}
			out[p.Name] = s
		case "integer":
			n, ok := v.(json.Number)
			if !ok {
				return nil, fmt.Errorf("%s 는 정수여야 합니다", p.Name)
			}
			f, err := n.Float64()
			if err != nil || f != math.Trunc(f) || math.Abs(f) > 1e9 {
				return nil, fmt.Errorf("%s 는 정수여야 합니다 (받은 값: %s)", p.Name, n)
			}
			i := int(f)
			if (p.Min != 0 || p.Max != 0) && (i < p.Min || i > p.Max) {
				return nil, fmt.Errorf("%s 는 %d~%d 이어야 합니다 (받은 값: %d)", p.Name, p.Min, p.Max, i)
			}
			out[p.Name] = i
		}
	}
	return out, nil
}

// ── 결과 텍스트 ────────────────────────────────────────────────

func encodeJSON(v any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
	return strings.TrimRight(buf.String(), "\n")
}

// fitResult obj[listKey] 목록을 maxResultBytes 안에 들도록 앞에서부터 자르고 "truncated" 안내를 붙인다.
func fitResult(obj map[string]any, listKey string, rows []any, hint string) string {
	obj[listKey] = rows
	text := encodeJSON(obj)
	if len(text) <= maxResultBytes {
		return text
	}
	lo, hi := 0, len(rows) // 들어가는 가장 긴 앞부분 (이분 탐색)
	for lo < hi {
		mid := (lo + hi + 1) / 2
		obj[listKey] = rows[:mid]
		obj["truncated"] = truncNote(len(rows), mid, hint)
		if len(encodeJSON(obj)) <= maxResultBytes {
			lo = mid
		} else {
			hi = mid - 1
		}
	}
	obj[listKey] = rows[:lo]
	obj["truncated"] = truncNote(len(rows), lo, hint)
	text = encodeJSON(obj)
	if len(text) > maxResultBytes { // 목록 밖 내용만으로도 크다
		return cutText(text)
	}
	return text
}

func truncNote(total, kept int, hint string) string {
	return fmt.Sprintf("잘림: 결과가 약 %dKB 를 넘어 %d행 중 앞 %d행만 담았습니다. 범위를 좁히세요 (%s)", maxResultBytes/1000, total, kept, hint)
}

// cutText 글자 경계에서 자르고 안내를 붙인다 (JSON 이 깨지므로 마지막 수단).
func cutText(text string) string {
	cut := maxResultBytes - 200
	for cut > 0 && !utf8.RuneStart(text[cut]) {
		cut--
	}
	return text[:cut] + "\n…\n잘림: 결과가 너무 큽니다. 범위를 좁히세요"
}

func toAny[T any](xs []T) []any {
	out := make([]any, len(xs))
	for i, x := range xs {
		out[i] = x
	}
	return out
}

// ── 도구 실행 ─────────────────────────────────────────────────

func runSearch(rc *runCtx, in args) (*toolResult, error) {
	c := rc.client
	ans, err := cmd.MCPSearch(c, cmd.SearchQuery{
		Keyword: in.str("keyword"), Limit: in.num("limit"), Page: in.num("page"), Sort: in.str("sort"),
		Org: in.str("org"), Stat: in.str("stat"), Region: in.str("region"),
	})
	if err != nil {
		return nil, err
	}
	obj := map[string]any{"total": ans.Total, "count": len(ans.Results)}
	if len(ans.Notices) > 0 {
		obj["notices"] = ans.Notices
	}
	return textResult(fitResult(obj, "results", toAny(ans.Results), "limit 을 줄이세요")), nil
}

func runMeta(rc *runCtx, in args) (*toolResult, error) {
	c := rc.client
	org, tbl, typ := in.str("org_id"), in.str("tbl_id"), in.str("type")
	find, under, level, axis := in.str("find"), in.str("under"), in.num("level"), in.str("axis")
	selecting := find != "" || under != "" || level > 0 || axis != ""
	if selecting && typ != "" && typ != "ITM" {
		return nil, fmt.Errorf("find/under/level/axis 는 분류·항목 코드에만 씁니다 — type %s 와 함께 쓸 수 없습니다", typ)
	}
	if selecting {
		obj := ""
		if strings.EqualFold(axis, "item") {
			obj = "ITEM"
		}
		rows, err := c.MetaCodes(org, tbl, obj)
		if err != nil {
			return nil, fmt.Errorf("메타데이터 조회 실패: %w", err)
		}
		out, params, err := cmd.MCPFilterCodes(rows, find, under, level, axis)
		if err != nil {
			return nil, err
		}
		res := map[string]any{"org_id": org, "tbl_id": tbl, "count": len(out), "data_params": params}
		return textResult(fitResult(res, "rows", toAny(out), "find·under·level·axis 로 더 좁히세요")), nil
	}
	if typ == "" {
		summary, err := c.MetaSummary(org, tbl)
		if err != nil {
			return nil, fmt.Errorf("메타데이터 조회 실패: %w", err)
		}
		res := cmd.MCPMetaSummary(summary, org, tbl)
		if cm, cerr := c.MetaRaw(org, tbl, "CMMT"); cerr == nil && len(cm) > 0 {
			res["comments"] = fmt.Sprintf("주석 %d개 — 조사 대상·정의·잠정 여부: kosis_meta type=CMMT", len(cm))
		}
		items, _ := res["items"].([]map[string]string)
		return textResult(fitResult(res, "items", toAny(items), "항목은 kosis_meta axis=item find=<이름> 으로 고르세요")), nil
	}
	rows, err := c.MetaRaw(org, tbl, typ)
	if err != nil && strings.Contains(err.Error(), "[30]") {
		rows, err = nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("메타데이터 조회 실패: %w", err)
	}
	res := map[string]any{"org_id": org, "tbl_id": tbl, "type": typ, "count": len(rows)}
	if len(rows) == 0 {
		res["notice"] = fmt.Sprintf("'%s' 타입의 메타데이터가 없습니다", typ)
	}
	return textResult(fitResult(res, "rows", toAny(rows), "type=ITM 은 find·under·level·axis 로 고르세요")), nil
}

func runData(rc *runCtx, in args) (*toolResult, error) {
	q := cmd.DataQuery{
		OrgID: in.str("org_id"), TblID: in.str("tbl_id"),
		Item: in.str("item"), Period: in.str("period"),
		Start: in.str("start"), End: in.str("end"), Latest: in.num("latest"),
		Periods: in.str("periods"), Fields: in.str("fields"),
		Derive: cmd.DataDerive{Sort: in.str("sort"), Top: in.num("top"), SortBy: in.str("sort_by"),
			Change: in.str("change"), Ratio: in.str("ratio"), NoTotal: in.flag("no_total")},
	}
	for i := range q.Classes {
		q.Classes[i] = in.str(fmt.Sprintf("class%d", i+1))
	}
	if rc.prog != nil {
		q.Progress = func(done, total int) { rc.prog.step(done, total, "분할 조회 조각") }
	}
	tr, err := transformArgs(in)
	if err != nil {
		return nil, err
	}
	ans, err := cmd.MCPData(rc.client, q)
	if err != nil {
		return nil, err
	}
	t := &table{}
	for _, c := range ans.Columns {
		t.Columns = append(t.Columns, column{Key: c.Key, Label: c.Label})
	}
	for _, r := range ans.Rows {
		row := make([]string, len(t.Columns))
		for i, c := range t.Columns {
			row[i] = r[c.Key]
		}
		t.Rows = append(t.Rows, row)
	}
	notices := ans.Notices
	if !tr.empty() {
		before := len(t.Rows)
		var notes []string
		if t, notes, err = tr.apply(t); err != nil {
			return nil, err
		}
		notices = append(notices, notes...)
		notices = append(notices, fmt.Sprintf("서버에서 줄임: 받은 %d행 → %d행 (filter·latest·aggregate·pivot)", before, len(t.Rows)))
	}
	head := []kv{{"org_id", ans.OrgID}, {"tbl_id", ans.TblID}}
	if len(notices) > 0 {
		head = append(head, kv{"notices", notices})
	}
	return rc.respondTable(t, in.str("format"), in.flag("save"), 0, head)
}

func runBatch(rc *runCtx, in args) (*toolResult, error) {
	qs, _ := in["queries"].([]args)
	results := make([]map[string]any, len(qs))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 4) // KOSIS 에 한꺼번에 몰리지 않게
	for i, a := range qs {
		wg.Add(1)
		go func(i int, a args) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			q := queryOf(a)
			q.Fields = a.str("fields")
			q.Derive = cmd.DataDerive{Sort: a.str("sort"), Top: a.num("top"), SortBy: a.str("sort_by"),
				Change: a.str("change"), Ratio: a.str("ratio"), NoTotal: a.flag("no_total")}
			res := map[string]any{"index": i + 1, "org_id": q.OrgID, "tbl_id": q.TblID}
			ans, err := cmd.MCPData(rc.client, q)
			if err != nil {
				res["error"] = err.Error()
				results[i] = res
				return
			}
			rows := ans.Rows
			if len(rows) > batchRowCap {
				res["truncated"] = fmt.Sprintf("행 %d개 중 앞 %d개만 — 전부는 kosis_data 로", len(rows), batchRowCap)
				rows = rows[:batchRowCap]
			}
			res["rows"] = rows
			if len(ans.Notices) > 0 {
				res["notices"] = ans.Notices
			}
			results[i] = res
		}(i, a)
	}
	wg.Wait()
	failed := 0
	for _, r := range results {
		if _, ok := r["error"]; ok {
			failed++
		}
	}
	obj := map[string]any{"count": len(results), "failed": failed}
	return textResult(fitResult(obj, "results", toAny(results), "조회를 나누거나 fields·top 으로 줄이세요")), nil
}

func runRatio(rc *runCtx, in args) (*toolResult, error) {
	num, _ := in["num"].(args)
	den, _ := in["den"].(args)
	scale := 100.0
	if f, ok := in["scale"].(float64); ok {
		scale = f
	}
	ans, err := cmd.MCPRatio(rc.client, queryOf(num), queryOf(den), scale, in.flag("no_total"), in.str("sort"), in.num("top"))
	if err != nil {
		return nil, err
	}
	obj := map[string]any{"count": len(ans.Rows), "scale": scale}
	if len(ans.Notices) > 0 {
		obj["notices"] = ans.Notices
	}
	return textResult(fitResult(obj, "rows", toAny(ans.Rows), "코드를 좁히거나 top 을 쓰세요")), nil
}

func runRules(rc *runCtx, in args) (*toolResult, error) {
	query, id, kind, topic := in.str("query"), in.str("id"), in.str("kind"), in.str("topic")
	if query == "" && id == "" && kind == "" && topic == "" {
		return nil, fmt.Errorf("query·id·kind·topic 중 하나는 주세요 (규칙 전체는 너무 깁니다)")
	}
	rows, err := cmd.MCPRules(query, id, kind, topic, "")
	if err != nil {
		return nil, err
	}
	obj := map[string]any{"count": len(rows)}
	if len(rows) == 0 {
		obj["notice"] = "맞는 규칙이 없습니다 — 그 표·주제의 규칙이 아직 없다는 뜻입니다. kosis_search·kosis_meta 로 확인하세요"
	}
	return textResult(fitResult(obj, "rules", toAny(rows), "id·kind·topic 으로 더 좁히세요")), nil
}

func runList(rc *runCtx, in args) (*toolResult, error) {
	c := rc.client
	rows, notices, err := cmd.MCPList(c, in.str("view"), in.str("parent"))
	if err != nil {
		return nil, err
	}
	obj := map[string]any{"view": in.str("view"), "count": len(rows)}
	if p := in.str("parent"); p != "" {
		obj["parent"] = p
	}
	if len(notices) > 0 {
		obj["notices"] = notices
	}
	return textResult(fitResult(obj, "rows", toAny(rows), "parent 로 한 단계 내려가세요")), nil
}

func runExplain(rc *runCtx, in args) (*toolResult, error) {
	c := rc.client
	res, err := cmd.MCPExplain(c, in.str("org_id"), in.str("tbl_id"))
	if err != nil {
		return nil, err
	}
	notes, _ := res["notes"].([]string)
	return textResult(fitResult(res, "notes", toAny(notes), "주석이 많습니다 — kosis_meta type=CMMT 로 보세요")), nil
}

func runIndicator(rc *runCtx, in args) (*toolResult, error) {
	c := rc.client
	action, name, limit := in.str("action"), in.str("name"), in.num("limit")
	var (
		res []api.IndicatorResult
		err error
	)
	switch action {
	case "search", "data":
		if name == "" {
			return nil, fmt.Errorf("action=%s 에는 name(지표명)이 필요합니다", action)
		}
		if action == "search" {
			res, err = c.IndicatorSearchByName(name, api.PaginationOptions{NumOfRows: limit})
		} else {
			res, err = c.IndicatorData(name, api.PaginationOptions{NumOfRows: limit})
		}
	case "info":
		if in.str("id") == "" {
			return nil, fmt.Errorf("action=info 에는 id(지표 ID)가 필요합니다")
		}
		res, err = c.IndicatorSearchByID(in.str("id"), api.PaginationOptions{})
	case "list":
		res, err = c.IndicatorList(in.str("list_id"), api.PaginationOptions{})
	}
	if err != nil && strings.Contains(err.Error(), "[30]") { // 결과 없음
		res, err = nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("지표 조회 실패: %w", err)
	}
	obj := map[string]any{"action": action, "count": len(res)}
	if len(res) == 0 {
		obj["notice"] = "결과가 없습니다. 지표(e-나라지표)는 이름이 좁습니다 — 통계표는 kosis_search 로 찾으세요"
	}
	return textResult(fitResult(obj, "results", toAny(res), "limit 을 줄이세요")), nil
}
