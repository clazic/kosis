# Module Context

이 디렉토리는 향후 TUI 대시보드 구현 컨텍스트다. 현재는 구현 밀도가 낮지만, 루트 no-arg 경험과 직접 연결되는 예정 영역이다.

# Tech Stack & Constraints

- future TUI module
- root no-arg entry와 연결 예정

Constraints:
- 실제 TUI 구현을 시작하면 root의 임시 대시보드 정책과 충돌하지 않게 점진적으로 교체해야 한다.
- 패널 구조가 생기면 `panels/`에도 별도 규칙을 추가한다.

# Implementation Patterns

- 먼저 최소 대시보드 탐색 구조를 만들고, 그다음 검색/조회 패널로 분리한다.
- 루트에서 직접 처리 중인 no-arg UX는 TUI가 자리 잡으면 이 디렉토리로 이동시킨다.

# Testing Strategy

- TUI 구현 전에는 root no-arg 재현을 확인한다.
- 구현 후에는 별도 진입 테스트와 렌더링 smoke test를 추가한다.

# Local Golden Rules

Do:
- 현재는 “준비 중” 상태와 임시 루트 대시보드의 역할을 문서로 명확히 둔다.

Don't:
- 미구현 상태에서 TUI가 완성된 것처럼 help를 바꾸지 않는다.
