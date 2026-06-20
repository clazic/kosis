# Module Context

이 패키지는 CLI 대화형 입력 유틸리티를 제공한다. `cmd/`의 interactive mode 품질과 비TTY 안전성이 여기서 결정된다.

# Tech Stack & Constraints

- stdin/stdout 기반 prompt
- 숫자 선택, 다중 선택, 확인 입력

Constraints:
- 비TTY 환경에서 무한 대기하면 안 된다.
- 취소 입력과 빈 입력 처리 규칙은 일관돼야 한다.
- UX 보강이 테스트 가능한 범위를 넘어서면 `cmd/`와 동작을 함께 조정한다.

# Implementation Patterns

- `Prompt`, `Select`, `MultiSelect`, `Confirm`의 취소 규칙을 통일한다.
- 파싱된 인덱스는 중복 제거와 범위 검증을 한다.
- 기본값이 있는 입력은 빈 입력 시 명시적으로 fallback 한다.

# Testing Strategy

- `go test ./internal/interactive`
- 비대화형 회귀는 관련 `cmd/` 재현과 함께 확인한다.

# Local Golden Rules

Do:
- 입력 포맷 확장 시 기존 숫자 입력 흐름을 깨지 않게 유지한다.
- 취소/종료 입력은 함수별로 다르게 정의하지 않는다.

Don't:
- 프롬프트 문구 변경만 하고 실제 파싱 규칙을 맞추지 않는다.
- 테스트 없이 선택 입력 규칙을 크게 바꾸지 않는다.
