# Module Context

이 디렉토리는 사용자에게 직접 노출되는 Cobra 명령 계층이다. `root.go`와 각 명령 파일은 사용법, help, 플래그, 예제, 대화형 진입 정책을 결정한다.

# Tech Stack & Constraints

- Cobra command tree
- 일부 short flag는 Cobra 제약을 우회하기 위해 루트 전처리를 사용한다.

Constraints:
- help에 적은 명령은 실제로 파싱 가능해야 한다.
- `root.go`의 전처리 정책과 각 하위 명령 help는 서로 모순되면 안 된다.
- 인자 부족 시 대화형 진입과 명시 오류 정책을 파일별로 일관되게 유지한다.

# Implementation Patterns

- help 섹션은 가능한 한 다음 순서를 유지한다:
  - 한줄 설명
  - 상세 설명
  - 사용법
  - 파라미터
  - 플래그
  - 예제
  - 다음 단계 또는 관련 명령
- 플래그 상호배타 규칙은 help와 실행 로직 양쪽에 반영한다.
- 재사용 원라인 명령을 출력하는 명령은 실제 지원 플래그 체계와 동일하게 출력한다.

# Testing Strategy

- 개별 help 검증:
  - `go run . --help`
  - `go run . search --help`
  - `go run . meta --help`
  - `go run . data --help`
  - `go run . explain --help`
  - `go run . indicator --help`
  - `go run . quick --help`
- 대표 파싱 검증:
  - `go run . data 101 DT_TEST -c1 11 -i T10 -p M`

# Local Golden Rules

Do:
- help 문자열과 실행 정책을 함께 수정한다.
- 루트 정규화에 의존하는 short flag가 있으면 help에 그 사실을 일관되게 반영한다.
- 비대화형 stdin에서 블로킹이 생기지 않는지 확인한다.
- `search/meta/explain/list`는 탐색 계층, `data`는 조회 계층, `quick`는 실행 계층, `bookmark/history`는 기록 계층으로 구분해 수정한다.
- `bulk/config` 같은 운영성 명령은 설정/파일 저장 경로를 실제로 검증한다.

Don't:
- Cobra 기본 동작을 그대로 믿고 설계서 포맷 요구를 무시하지 않는다.
- 실제로 실패하는 예제를 help에 남기지 않는다.
