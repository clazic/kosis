# Module Context

이 패키지는 quick 명령의 규칙 기반 매칭과 외부 AI 도구 호출을 담당한다. 작은 변경도 사용자 체감에 직접 드러난다.

# Tech Stack & Constraints

- token/regex 기반 matcher
- 외부 AI CLI 실행

Constraints:
- shortcut 추가나 규칙 변경 시 `matcher_test.go` 기대값과 함께 관리한다.
- AI 경로는 생성 실패와 실행 실패를 구분해서 다뤄야 한다.
- 규칙 기반 fallback은 search/data 어느 쪽으로 가는지 명확해야 한다.

# Implementation Patterns

- shortcut은 범용 단어보다 오탐이 적은 표현을 우선한다.
- 기간 파싱 로직은 연속 범위, 비연속 시점, 주기 추론을 분리한다.
- AI 도구 명령 템플릿은 prompt 치환과 실패 메시지를 함께 다룬다.

# Testing Strategy

- `go test ./internal/nlp`
- quick 연동 회귀는 CLI 레벨에서:
  - `go run . quick --help`
  - `go run . quick "서울 미분양 최근 6개월"`

# Local Golden Rules

Do:
- 새 shortcut을 넣기 전 기존 테스트 기대값과 충돌 여부를 확인한다.
- 환경 제약으로 AI 성공 재현이 어려우면 커스텀 도구 시나리오로라도 실행 경로를 검증한다.

Don't:
- matcher 회귀를 테스트 깨진 채로 남기지 않는다.
- AI 실패를 단순 `exit status 1`만 보여주고 끝내지 않는다.
