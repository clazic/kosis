# Module Context

이 디렉토리는 CLI 하위의 실제 구현 패키지 모음이다. `api`, `config`, `output`, `interactive`, `nlp`, `cache`, `bookmark`, `history`, `tui`가 포함된다.

# Tech Stack & Constraints

- `api/`: KOSIS Open API 호출
- `output/`: 포맷터 및 파일 저장
- `interactive/`: CLI 대화형 입력 유틸리티
- `nlp/`: quick 규칙 기반/AI 보조 처리
- `config/`: 설정 및 AI 도구 등록

Constraints:
- 공개 계약을 바꾸는 변경은 `cmd/` help와 함께 반영해야 한다.
- 테스트가 있는 패키지는 기존 기대값을 보존하거나, 정책 변경 시 테스트도 함께 갱신한다.
- 출력 패키지는 stdout 포맷과 파일 저장 경로를 섞지 않는다.

# Implementation Patterns

- API 패키지는 입력 검증, 응답 파싱, 에러 메시지를 분리해 둔다.
- output 패키지는 확장자 감지와 저장 경로를 단일 진입점으로 유지한다.
- interactive 패키지는 비대화형 환경에서 무한 대기를 만들지 않도록 방어한다.
- nlp 패키지는 shortcut/규칙 추가 시 테스트 회귀를 먼저 확인한다.

# Testing Strategy

- 전체: `go test ./...`
- 패키지별:
  - `go test ./internal/api`
  - `go test ./internal/output`
  - `go test ./internal/nlp`
  - `go test ./internal/interactive`
  - `go test ./internal/config`

# Local Golden Rules

Do:
- 규칙 기반 파서 수정 시 관련 테스트를 같이 확인한다.
- 파일 저장 기능 수정 시 존재 여부와 실패 경로를 함께 검증한다.

Don't:
- 테스트 기대값이 깨졌는데 원인 설명 없이 덮어두지 않는다.
- 한 패키지의 정책을 다른 패키지 help에 반영하지 않고 끝내지 않는다.

# Context Map

- **[KOSIS API 클라이언트](./api/AGENTS.md)** — 엔드포인트, 파라미터, 자동분할, API 에러 처리 수정 시.
- **[출력 포맷터와 파일 저장](./output/AGENTS.md)** — table/json/csv/xlsx/sqlite/parquet 저장 및 stdout 분기 수정 시.
- **[자연어 매칭과 AI 실행](./nlp/AGENTS.md)** — shortcut, 기간 파싱, AI 명령 생성 규칙 수정 시.
- **[대화형 입력 유틸리티](./interactive/AGENTS.md)** — Prompt/Select/MultiSelect/Confirm UX와 비TTY 처리 수정 시.
- **[설정과 AI 도구 등록](./config/AGENTS.md)** — API 키, 기본 포맷, AI 도구 설정/테스트 수정 시.
- **[파일 기반 캐시](./cache/AGENTS.md)** — TTL, 만료 정리, 원자적 쓰기, 캐시 테스트 수정 시.
- **[즐겨찾기 저장소](./bookmark/AGENTS.md)** — bookmarks.yaml CRUD와 이름/인덱스 규칙 수정 시.
- **[조회 이력 저장소](./history/AGENTS.md)** — history.yaml 기록, 최대 개수, replay 대상 조회 수정 시.
