# Module Context

이 패키지는 KOSIS Open API 호출과 데이터 분할 로직을 담당한다. CLI 명령은 이 패키지의 파라미터 계약과 에러 메시지에 직접 의존한다.

# Tech Stack & Constraints

- HTTP client
- KOSIS JSON API 응답 파싱
- 멀티 API 키, 자동분할, 비연속 시점 처리

Constraints:
- 엔드포인트 URL과 필수 파라미터는 설계서와 일치해야 한다.
- 데이터 조회 계열은 캐시 정책을 임의로 바꾸지 않는다.
- 입력 검증과 API 에러 감지는 사용자 메시지로 이어지므로 의미를 보존한다.

# Implementation Patterns

- 공통 파라미터 조립은 중복 없이 헬퍼로 묶는다.
- `Data`, `DataRegistered`, `DataWithPeriods`, `DataWithAutoSplit` 경계는 역할을 섞지 않는다.
- 자동분할 추정 로직을 바꾸면 실제 요청 수와 진행 메시지 영향까지 확인한다.

# Testing Strategy

- 전체 확인: `go test ./...`
- API 패키지 집중 검증:
  - `go test ./internal/api`
- 회귀 확인 포인트:
  - 비연속 시점 파싱
  - 멀티 키 요청 경로
  - 4만 셀 분할 경로

# Local Golden Rules

Do:
- API 응답 구조 변경 시 `types.go`와 호출부를 함께 수정한다.
- 새 분기 추가 시 `cmd/` help가 설명하는 동작과 맞는지 확인한다.

Don't:
- splitter의 데드 코드나 추정 로직을 방치한 채 계약만 바꾸지 않는다.
- 인증 오류, 파라미터 오류, 빈 결과를 같은 에러로 뭉개지 않는다.
