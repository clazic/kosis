# Module Context

이 패키지는 조회 이력을 저장하고 replay 대상 조회에 필요한 데이터를 제공한다. `~/.kosis/history.yaml` 구조와 최대 100개 유지 정책이 핵심이다.

# Tech Stack & Constraints

- YAML 저장
- 증가 ID
- 최대 개수 제한

Constraints:
- 최대 100개 유지 정책을 임의로 바꾸지 않는다.
- ID는 append 기준 단조 증가를 유지해야 한다.
- `List`, `GetByID`, `Clear`는 replay UX와 연결되므로 반환 규칙을 안정적으로 유지한다.

# Implementation Patterns

- `Add`는 현재 시각과 결과 개수를 함께 기록한다.
- `List(limit)`는 최근 항목 기준으로 슬라이스를 자른다.
- `Clear`는 파일 삭제가 아니라 빈 구조 저장 정책을 유지한다.

# Testing Strategy

- 현재 테스트가 없다면 변경 시 limit, maxHistoryEntries, GetByID 경계조건을 우선 검토한다.
- 전체 영향 확인: `go test ./...`

# Local Golden Rules

Do:
- history ID 정책을 바꿀 때 replay 경로 영향을 먼저 확인한다.
- 최대 개수 초과 시 오래된 항목이 잘리는 순서를 유지한다.

Don't:
- timestamp 포맷을 이유 없이 바꾸지 않는다.
- 빈 이력과 존재하지 않는 ID 오류를 같은 경로로 처리하지 않는다.
