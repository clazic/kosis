// Package interactive provides terminal-based interactive UI for KOSIS CLI.
// 사용자 입력을 위한 대화형 모드를 제공합니다.
package interactive

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Prompt는 사용자 텍스트 입력을 받고 응답을 반환합니다.
// 입력이 비어있으면 다시 입력을 요청합니다.
func Prompt(label string) string {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print(label + " (취소: q) ")
		text, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("입력 오류가 발생했습니다. 다시 시도하세요.")
			continue
		}
		text = strings.TrimSpace(text)
		lower := strings.ToLower(text)
		if lower == "q" || lower == "quit" || lower == "exit" {
			return ""
		}
		if text == "" {
			fmt.Println("입력이 비어있습니다. 다시 입력하세요.")
			continue
		}
		return text
	}
}

// Select는 옵션 목록을 제시하고 선택된 인덱스와 값을 반환합니다.
// 0부터 len(options)-1 범위의 숫자만 유효합니다.
func Select(label string, options []string) (int, string) {
	if len(options) == 0 {
		fmt.Println("선택 가능한 옵션이 없습니다.")
		return -1, ""
	}

	fmt.Println(label)
	for i, opt := range options {
		fmt.Printf("  [%d] %s\n", i, opt)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Printf("선택 (0~%d): ", len(options)-1)
		input, err := reader.ReadString('\n')
		if err != nil {
			fmt.Println("입력 오류가 발생했습니다. 다시 시도하세요.")
			continue
		}

		input = strings.TrimSpace(input)
		if input == "" {
			fmt.Println("선택을 입력하세요.")
			continue
		}

		var idx int
		_, err = fmt.Sscanf(input, "%d", &idx)
		if err != nil {
			fmt.Println("유효한 숫자를 입력하세요.")
			continue
		}

		if idx < 0 || idx >= len(options) {
			fmt.Printf("0부터 %d 사이의 숫자를 입력하세요.\n", len(options)-1)
			continue
		}

		return idx, options[idx]
	}
}

// MultiSelect presents a list of options and returns selected indices.
// User selects by entering space-separated numbers.
func MultiSelect(label string, options []string) []int {
	if len(options) == 0 {
		fmt.Println("선택 가능한 옵션이 없습니다.")
		return []int{}
	}

	fmt.Println(label)
	for i, opt := range options {
		fmt.Printf("  [%d] %s\n", i, opt)
	}

	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Print("선택 (공백/쉼표 구분 번호, 예: 0 2 3 또는 0,2,3, 취소: q): ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		lower := strings.ToLower(input)
		if lower == "q" || lower == "quit" || lower == "exit" {
			return []int{}
		}

		if input == "" {
			fmt.Println("최소 하나 이상 선택해야 합니다.")
			continue
		}

		normalized := strings.ReplaceAll(input, ",", " ")
		parts := strings.Fields(normalized)
		var indices []int
		seen := make(map[int]bool)
		valid := true

		for _, part := range parts {
			var idx int
			_, err := fmt.Sscanf(part, "%d", &idx)
			if err != nil || idx < 0 || idx >= len(options) {
				fmt.Printf("'%s'는 유효하지 않은 선택입니다.\n", part)
				valid = false
				break
			}
			if !seen[idx] {
				indices = append(indices, idx)
				seen[idx] = true
			}
		}

		if !valid {
			continue
		}

		return indices
	}
}

// Confirm asks a yes/no question with a default answer.
func Confirm(label string, defaultYes bool) bool {
	reader := bufio.NewReader(os.Stdin)
	defaultStr := "y/N"
	if defaultYes {
		defaultStr = "Y/n"
	}

	for {
		fmt.Printf("%s [%s] ", label, defaultStr)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input == "" {
			return defaultYes
		}

		if input == "y" || input == "yes" || input == "예" || input == "네" || input == "ㅇ" {
			return true
		}
		if input == "n" || input == "no" || input == "아니오" || input == "아니요" || input == "ㄴ" {
			return false
		}

		fmt.Println("Y/N (또는 예/아니오)을 입력하세요.")
	}
}

// SelectInt asks user to input an integer with a default value.
func SelectInt(label string, defaultValue int) int {
	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Printf("%s [%d]: ", label, defaultValue)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input == "" {
			return defaultValue
		}

		var value int
		_, err := fmt.Sscanf(input, "%d", &value)
		if err != nil || value <= 0 {
			fmt.Println("올바른 양의 정수를 입력하세요.")
			continue
		}

		return value
	}
}
