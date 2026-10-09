// Package nlp는 자연어 처리 및 AI 연동을 담당합니다.
//
// quick --ai의 역할 분담:
//   - 글생성(openai 호환, 기본 hermes): 요청 문장 → 검색 키워드·조건·기간 (ParseRequest)
//   - 선택(TypeSafe Jev): 실제 API가 돌려준 후보 목록 중 하나 고르기 (Choose)
//
// KOSIS 코드는 학습으로 알 수 없으므로 AI가 코드를 "만들지" 않고 실제 목록에서 "고르게" 한다.
package nlp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

// aiHTTPClient 에이전트형 서버(Hermes 등)는 수십 초 걸릴 수 있어 넉넉히 잡는다.
var aiHTTPClient = &http.Client{Timeout: 180 * time.Second}

// Endpoint AI API 접속 정보.
type Endpoint struct {
	Name    string // 표시용 공급자 이름
	BaseURL string // .../v1
	Model   string
	APIKey  string
}

// PeriodSpec 요청에서 해석한 기간.
type PeriodSpec struct {
	Kind   string   `json:"kind"` // latest | range | list | none
	Latest int      `json:"latest"`
	Start  string   `json:"start"`
	End    string   `json:"end"`
	List   []string `json:"list"`
	Unit   string   `json:"unit"` // latest N 의 단위: Y | M | Q | H | ""
	// Freq 사용자가 원하는 출력 주기("분기별로", "1분기", "월별"). Unit 과 따로 둔다 —
	// "최근 2년간 분기별로"는 Unit=Y(2년) 이지만 Freq=Q 다. 한 칸에 담으면 둘 중 하나가 사라진다.
	Freq string `json:"freq"`
}

// Intent 요청 해석 결과.
type Intent struct {
	English    string   `json:"english"` // Jev는 영어 질문만 이해하므로 요청의 영어 풀이를 받아 둔다
	Keyword    string   `json:"keyword"`
	Conditions []string `json:"conditions"`
	// Measures 요청이 묻는 측정 대상들. "종사자수와 생산액"처럼 2개 이상이면 항목을 여러 개 고른다.
	Measures []string   `json:"measures"`
	Period   PeriodSpec `json:"period"`
}

func postJSON(ep Endpoint, path string, payload any, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	endpoint := strings.TrimRight(ep.BaseURL, "/") + path
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("요청 생성 실패: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if ep.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+ep.APIKey)
	}

	resp, err := aiHTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("%s 연결 실패 (%s): %w", ep.Name, endpoint, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))

	if resp.StatusCode != http.StatusOK {
		msg := strings.TrimSpace(string(raw))
		if len(msg) > 300 {
			msg = msg[:300] + "..."
		}
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("%s 인증 실패 (HTTP %d): API 키를 확인하세요 — %s", ep.Name, resp.StatusCode, msg)
		}
		return fmt.Errorf("%s 응답 오류 (HTTP %d): %s", ep.Name, resp.StatusCode, msg)
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("%s 응답 형식을 해석하지 못했습니다: %w", ep.Name, err)
	}
	return nil
}

// ParseRequest 글생성 공급자(OpenAI 호환 /chat/completions)로 요청을 해석한다.
func ParseRequest(ep Endpoint, userRequest string, today time.Time) (*Intent, error) {
	var resp struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	err := postJSON(ep, "/chat/completions", map[string]any{
		"model":       ep.Model,
		"temperature": 0,
		"messages": []map[string]string{
			{"role": "system", "content": parsePrompt(today)},
			{"role": "user", "content": userRequest},
		},
	}, &resp)
	if err != nil {
		return nil, err
	}
	if len(resp.Choices) == 0 {
		return nil, fmt.Errorf("%s 응답에 choices가 없습니다", ep.Name)
	}
	return parseIntentJSON(resp.Choices[0].Message.Content)
}

func parsePrompt(today time.Time) string {
	return `도구를 사용하지 말고 JSON 객체 하나만 출력해. 설명·코드펜스 금지.
사용자의 한국 통계(KOSIS) 조회 요청을 분석해 아래 형식으로 답해:
{"english": "요청 전체를 영어로 풀어 쓴 한 문장 (지역·조건은 영어와 한글 병기, 예: Ulsan (울산))",
 "keyword": "KOSIS 통계표 검색어. 지역·기간을 뺀 짧은 한국어 통계 용어 (예: 출생아수, 소비자물가지수, 아파트 매매가격지수). 요청이 영어여도 반드시 한국어로",
 (conditions 의 지역명도 한국어로: Ulsan → 울산)
 "conditions": ["지역·성별·연령·산업 등 분류 조건 단어. 비교·나열된 대상은 각각 따로 (부산과 대구 → [\"부산\", \"대구\"]). 없으면 빈 배열"],
 "measures": ["요청이 묻는 측정 대상. 여러 개면 각각 따로 (종사자수와 생산액 → [\"종사자수\", \"생산액\"]). 하나면 하나"],
 "period": {"kind": "latest|range|list|none", "latest": 정수, "start": "YYYY 또는 YYYYMM", "end": "YYYY 또는 YYYYMM", "list": ["YYYY"...], "unit": "Y|M|Q|H 또는 빈 문자열", "freq": "Y|M|Q|H 또는 빈 문자열"}}
규칙: "최근 N년/개월"은 kind=latest 이고 unit 은 N의 단위(년=Y, 개월=M, 분기=Q)로 반드시 채운다.
freq 는 사용자가 원하는 출력 주기다: "분기별", "1분기" → Q / "월별", "N월" → M / "연간" → Y. 언급이 없으면 빈 문자열.
특정 분기("2025년 1분기")는 kind=range, start·end 를 그 분기의 첫 달·끝 달 YYYYMM 으로, freq=Q. "작년"·"올해" 같은 상대 표현은 오늘 날짜 기준 절대 연도로 바꿔 range로. 기간 언급이 없으면 kind=none.
오늘: ` + today.Format("2006-01-02")
}

// parseIntentJSON 코드펜스나 앞뒤 잡문이 섞여도 첫 '{' ~ 마지막 '}'를 해석한다.
func parseIntentJSON(text string) (*Intent, error) {
	i, j := strings.Index(text, "{"), strings.LastIndex(text, "}")
	if i < 0 || j <= i {
		return nil, fmt.Errorf("해석 결과에 JSON이 없습니다: %.120s", text)
	}
	var it Intent
	if err := json.Unmarshal([]byte(text[i:j+1]), &it); err != nil {
		return nil, fmt.Errorf("해석 결과 JSON 오류: %w", err)
	}
	it.Keyword = strings.TrimSpace(it.Keyword)
	if it.Keyword == "" {
		return nil, fmt.Errorf("해석 결과에 검색 키워드가 없습니다")
	}
	return &it, nil
}

// ChoiceQuestion Jev Choice 질문 하나. Options: 키(코드) → 설명.
type ChoiceQuestion struct {
	Instructions string
	Options      map[string]string
}

// ChoiceAnswer Jev 답.
type ChoiceAnswer struct {
	Choice        string             `json:"choice"`
	Confidence    float64            `json:"confidence"`
	Probabilities map[string]float64 `json:"probabilities"`
}

// Ranked 확률 높은 순 옵션 키.
func (a ChoiceAnswer) Ranked() []string {
	keys := make([]string, 0, len(a.Probabilities))
	for k := range a.Probabilities {
		keys = append(keys, k)
	}
	sort.SliceStable(keys, func(i, j int) bool {
		if a.Probabilities[keys[i]] != a.Probabilities[keys[j]] {
			return a.Probabilities[keys[i]] > a.Probabilities[keys[j]]
		}
		return keys[i] < keys[j]
	})
	return keys
}

// MaxChoiceOptions Jev Choice 한 질문의 최대 옵션 수.
const MaxChoiceOptions = 255

// Choose TypeSafe System One(Jev)에 여러 Choice 질문을 한 번에 보낸다 (서버가 병렬 평가).
func Choose(ep Endpoint, state string, questions map[string]ChoiceQuestion) (map[string]ChoiceAnswer, error) {
	qs := make(map[string]any, len(questions))
	for id, q := range questions {
		if len(q.Options) == 0 || len(q.Options) > MaxChoiceOptions {
			return nil, fmt.Errorf("질문 %s의 옵션 수가 잘못됐습니다: %d (1~%d)", id, len(q.Options), MaxChoiceOptions)
		}
		crit := make(map[string]any, len(q.Options))
		for k, desc := range q.Options {
			if desc == "" {
				crit[k] = nil
			} else {
				crit[k] = desc
			}
		}
		qs[id] = map[string]any{"type": "choice", "instructions": q.Instructions, "criteria": crit}
	}

	var resp struct {
		Answers map[string]ChoiceAnswer `json:"answers"`
	}
	if err := postJSON(ep, "/systemone", map[string]any{
		"model": ep.Model, "state": state, "questions": qs,
	}, &resp); err != nil {
		return nil, err
	}
	for id := range questions {
		a, ok := resp.Answers[id]
		if !ok || a.Choice == "" {
			return nil, fmt.Errorf("%s 응답에 질문 %s의 답이 없습니다", ep.Name, id)
		}
		if _, valid := questions[id].Options[a.Choice]; !valid {
			return nil, fmt.Errorf("%s가 목록 밖의 값을 골랐습니다: %s", ep.Name, a.Choice)
		}
	}
	return resp.Answers, nil
}

// uniqueShortcuts Shortcuts에서 통계표별로 1개씩만, 순서를 고정해 반환합니다.
// ("미분양"과 "미분양주택"처럼 여러 키가 같은 통계표를 가리킨다)
func uniqueShortcuts() []TableShortcut {
	seen := make(map[string]bool)
	var out []TableShortcut
	for _, key := range sortedShortcutKeys() {
		sc := Shortcuts[key]
		id := sc.OrgID + "/" + sc.TblID
		if seen[id] {
			continue
		}
		seen[id] = true
		out = append(out, sc)
	}
	return out
}

func sortedShortcutKeys() []string {
	keys := make([]string, 0, len(Shortcuts))
	for k := range Shortcuts {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
