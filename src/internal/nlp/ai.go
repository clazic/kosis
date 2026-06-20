// Package nlp는 자연어 처리 및 AI 도구 연동을 담당합니다.
package nlp

import (
	"fmt"
	"os/exec"
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
// 작은따옴표 내부의 작은따옴표는 '\'' 패턴으로 이스케이프합니다.
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

// GetSKILLContent는 SKILL.md 파일 내용을 반환합니다 (프롬프트에 포함).
// 실행 파일 위치 기준 또는 내장 문자열로 핵심 사용법만 제공합니다.
func GetSKILLContent() string {
	return `kosis d <ORG_ID> <TBL_ID> --class1 <코드> --item <코드> --period <Y|M|Q|H> [--start <시작> --end <끝> | --latest <N> | --periods "2020,2022,2025"] [-o 파일]

자주 쓰는 통계표:
  미분양: 116 DT_MLTM_2086 (항목: T10=미분양)
  소비자물가: 101 DT_1J20001 (항목: T10=지수)
  GDP: 301 DT_200Y001 (항목: T01=경상가격)
  인구: 101 DT_1IN1502 (항목: T100=총인구)
  경제활동: 101 DT_1DA7002S
  주민등록인구: 101 DT_1YL20651E

지역코드: 00=전국, 11=서울, 21=부산, 22=대구, 23=인천, 24=광주, 25=대전, 26=울산, 29=세종, 31=경기, 32=강원, 33=충북, 34=충남, 35=전북, 36=전남, 37=경북, 38=경남, 39=제주`
}
