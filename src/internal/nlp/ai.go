// Package nlp는 자연어 처리 및 AI 도구 연동을 담당합니다.
package nlp

import (
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// AIResult는 AI 도구가 생성한 명령어를 나타냅니다.
type AIResult struct {
	Command string // 생성된 kosis 명령어 (예: "kosis d 116 DT_MLTM_2086 ...")
	Tool    string // 사용된 AI 도구 이름
	Error   error
}

// DetectAITool은 시스템에 설치된 AI CLI 도구를 감지합니다.
// 우선순위: claude > gemini > codex
func DetectAITool() string {
	for _, tool := range []string{"claude", "gemini", "codex"} {
		if _, err := exec.LookPath(tool); err == nil {
			return tool
		}
	}
	return ""
}

// shellEscape는 문자열을 작은따옴표로 감싸 셸 인젝션을 방지합니다.
// 작은따옴표 내부의 작은따옴표는 '\” 패턴으로 이스케이프합니다.
func shellEscape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

// GenerateCommand는 AI 도구를 사용하여 자연어를 kosis 명령어로 변환합니다.
func GenerateCommand(toolName, toolCmd, userRequest, skillContent string) (*AIResult, error) {
	// 1. 프롬프트 구성
	prompt := buildPrompt(userRequest, skillContent)

	// 2. 도구 명령어에 프롬프트 삽입 (셸 이스케이프 적용)
	//    toolCmd 예: "claude -p '{prompt}'"
	//    → {prompt}를 셸 이스케이프된 프롬프트로 치환
	//    기존 따옴표로 감싸진 '{prompt}' 패턴을 이스케이프된 값으로 교체
	escapedPrompt := shellEscape(prompt)
	// '{prompt}' (따옴표 포함) 패턴이 있으면 통째로 교체
	fullCmd := strings.ReplaceAll(toolCmd, "'{prompt}'", escapedPrompt)
	// "{prompt}" (쌍따옴표 포함) 패턴이 있으면 통째로 교체
	fullCmd = strings.ReplaceAll(fullCmd, "\"{prompt}\"", escapedPrompt)
	// 따옴표 없는 {prompt} 패턴이 남아있으면 이스케이프된 값으로 교체
	fullCmd = strings.ReplaceAll(fullCmd, "{prompt}", escapedPrompt)

	// 3. exec.Command로 실행
	//    셸을 통해 실행 (파이프 등 지원)
	cmd := exec.Command("sh", "-c", fullCmd)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return &AIResult{Tool: toolName, Error: err}, err
	}

	// 4. 결과 파싱 (kosis로 시작하는 줄 추출)
	command := extractKosisCommand(string(output))

	return &AIResult{Command: command, Tool: toolName}, nil
}

// buildPrompt는 AI에게 보낼 프롬프트를 구성합니다.
func buildPrompt(userRequest, skillContent string) string {
	return fmt.Sprintf(`다음 사용자 요청을 kosis CLI 명령어로 변환해줘.
명령어만 한 줄로 출력해. 설명은 불필요.

[사용법]
%s

[요청]
%s`, skillContent, userRequest)
}

// extractKosisCommand는 출력에서 kosis 명령어를 추출합니다.
func extractKosisCommand(output string) string {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "kosis ") {
			return line
		}
	}
	// kosis로 시작하는 줄이 없으면 전체 출력을 명령어로 간주
	return strings.TrimSpace(output)
}

// GetSKILLContent는 AI에게 줄 통계표 치트시트를 Shortcuts 사전에서 생성합니다.
// 손으로 관리하는 두 번째 사본을 두지 않는 것이 핵심 — 사본을 두면 반드시 어긋난다.
// 코드 조각 목록이 아니라 "완성된 명령 템플릿"을 주어야 AI가 플래그를 빠뜨리지 않는다.
func GetSKILLContent() string {
	var b strings.Builder

	b.WriteString(`kosis d <ORG_ID> <TBL_ID> --class1 <코드> [--class2 <코드>] --item <코드> --period <Y|M|Q|H> (--latest <N> | --start <시작> --end <끝> | --periods "2020,2022,2025") [-o 파일]

⚠ 반드시 지킬 규칙
1. 아래 목록에 있는 통계표만 사용한다. 목록에 없으면 명령 대신 정확히 "SEARCH: <키워드>" 한 줄만 출력한다.
2. 각 통계표의 템플릿에 있는 플래그를 하나도 빼지 않는다. 값을 모른다고 플래그를 생략하지 않는다.
3. 분류 코드는 해당 통계표 블록 안의 코드만 쓴다. 통계표끼리 코드를 옮겨 쓰면 반드시 틀린다.
4. 각 통계표가 지원하는 주기 밖의 --period를 쓰지 않는다.

`)

	for _, sc := range uniqueShortcuts() {
		fmt.Fprintf(&b, "■ %s (org=%s tbl=%s) 주기: %s\n",
			sc.Label, sc.OrgID, sc.TblID, strings.Join(sc.Periods, ","))

		// 완성된 템플릿 한 줄
		tmpl := fmt.Sprintf("  템플릿: kosis d %s %s", sc.OrgID, sc.TblID)
		for _, flag := range sortedMapKeys(sc.Fixed) {
			tmpl += fmt.Sprintf(" %s %s", flag, sc.Fixed[flag])
		}
		if sc.Region != nil {
			tmpl += fmt.Sprintf(" %s <지역코드>", sc.Region.Flag)
		}
		tmpl += fmt.Sprintf(" --item %s --period %s --latest 5", sc.Item, sc.Periods[0])
		b.WriteString(tmpl + "\n")

		if sc.Region != nil {
			b.WriteString(fmt.Sprintf("  지역(%s): ", sc.Region.Flag))
			for _, name := range regionOrder {
				if code, ok := sc.Region.Codes[name]; ok {
					fmt.Fprintf(&b, "%s=%s ", name, code)
				}
			}
			b.WriteString("\n")
		} else {
			// 지역이 없는 표에서 AI가 class1을 비우는 것을 막는 결정적 지시.
			b.WriteString("  ※ 지역별 분류 없음. 요청에 지역명이 있어도 무시하고 위 고정 코드를 그대로 쓴다.\n")
		}
	}

	return b.String()
}

// regionOrder 치트시트 출력 순서 고정용 (맵 순회는 순서가 불확정).
var regionOrder = []string{"전국", "서울", "부산", "대구", "인천", "광주", "대전", "울산",
	"세종", "경기", "강원", "충북", "충남", "전북", "전남", "경북", "경남", "제주"}

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

func sortedMapKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
