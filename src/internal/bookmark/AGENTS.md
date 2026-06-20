# Module Context

이 패키지는 사용자 즐겨찾기 저장을 담당한다. `~/.kosis/bookmarks.yaml` 형식과 이름/인덱스 기반 조작 규칙이 핵심 계약이다.

# Tech Stack & Constraints

- YAML 저장
- config.ConfigDir 기반 파일 위치

Constraints:
- 파일 경로는 설정 디렉토리 정책과 일치해야 한다.
- 이름 중복 정책과 자동 이름 생성 규칙을 임의로 바꾸지 않는다.
- 비어 있는 파일과 파일 없음 케이스를 동일하게 안전 처리해야 한다.

# Implementation Patterns

- `Load`와 `Save`는 YAML 구조체 경계를 유지한다.
- `Add`는 `orgID_tblID` 자동 이름 규칙을 보존한다.
- `Remove`는 숫자 인덱스와 이름 입력을 모두 지원하되, 모호성 없이 처리한다.

# Testing Strategy

- 현재 테스트가 없다면 변경 시 최소한 로컬 재현이나 새 테스트 추가를 고려한다.
- 전체 영향 확인: `go test ./...`

# Local Golden Rules

Do:
- 파일 저장 전 설정 디렉토리 준비를 보장한다.
- 사용자 메시지는 이름 기준 실패와 인덱스 기준 실패를 구분 가능하게 유지한다.

Don't:
- 저장 형식 키 이름을 조용히 바꾸지 않는다.
- 이름 중복 검사를 빼지 않는다.
