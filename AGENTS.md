# Project Context & Operations

이 저장소는 KOSIS CLI 프로젝트 연구 및 구현 워크스페이스다. 핵심 산출물은 `kosis-cli/` 아래의 Go 기반 CLI이며, 설계 문서와 스킬 문서를 기준으로 명령어, help 출력, 대화형 흐름, 출력 포맷, 평가 문서를 함께 관리한다.

기술 스택:
- Go 1.26.x
- Cobra CLI
- Viper 설정 관리
- SQLite, XLSX, Parquet 출력
- Markdown 기반 운영 문서

Operational Commands:
- 작업 디렉토리 진입: `cd src`
- 의존성 확인: `go mod tidy`
- **빌드(필수): `make build`** — 멀티플랫폼 전체를 빌드해 `src/bin/` 과 스킬 루트 `apps/` 두 곳에 배치한다. CLI 수정 후에는 항상 실행한다. (현재 플랫폼만은 `make native`)
- 테스트: `go test ./...`
- 대표 help 검증: `go run . --help`
- 대표 명령 검증: `go run . data --help`

# Golden Rules

Immutable:
- 작업 시작 전 반드시 `docs/superpowers/specs/2026-03-31-kosis-cli-design.md`와 `skills/kosis-cli/SKILL.md`를 읽는다.
- `kosis-cli/`에서 구현과 문서가 다르면 코드와 규칙 중 하나를 반드시 맞춘다.
- `--help`는 설계서 계약의 일부로 취급한다.
- API 키, 비밀값, 사용자 환경 파일을 하드코딩하지 않는다.
- 사용자가 만든 변경이나 현재 워크트리의 기존 변경을 임의로 되돌리지 않는다.

Do:
- 계획서·설계 문서는 프로젝트 루트의 `.plan/` 폴더에 `YYYY-MM-DD-HH:MM:SS-제목.md` 형식으로 작성한다. `.plan/`은 개발 전용이며 스킬 배포 산출물에 포함하지 않는다.
- 변경 전 현재 파일과 인접 모듈을 읽고 맥락을 확인한다.
- 빌드, 테스트, help 재현처럼 실행 가능한 검증을 남긴다.
- PM 문서와 평가 문서를 실제 상태에 맞게 갱신한다.
- 다중 라운드 작업이면 점수와 잔여 리스크를 문서에 기록한다.

Don't:
- 추상적 조언만 남기고 파일 반영 없이 끝내지 않는다.
- 설계서에 없는 새 계약을 조용히 도입하지 않는다.
- 루트 규칙을 하위 폴더에 복붙하지 말고, 하위 컨텍스트에 맞게 좁혀 쓴다.

# Standards & References

참조 문서:
- 설계서: `./docs/superpowers/specs/2026-03-31-kosis-cli-design.md`
- 사용 가이드: `./skills/kosis-cli/SKILL.md`

코딩 기준:
- Go 코드는 `gofmt` 기준을 따른다.
- help 문자열, 예제, 실제 파싱/실행 경로를 함께 수정한다.
- 테스트가 가능한 변경이면 `go test ./...` 또는 더 좁은 패키지 테스트를 실행한다.

Git 전략:
- 작은 단위로 의도적인 변경을 만든다.
- 커밋 메시지는 범위가 드러나게 작성한다. 예: `cmd: align data help with flag parsing`

Maintenance Policy:
- 규칙과 실제 코드가 어긋나면 규칙 문서 또는 코드를 즉시 함께 갱신한다.
- 하위 폴더에 고유 컨텍스트가 생기면 해당 폴더에 새 `AGENTS.md` 추가를 제안하거나 직접 반영한다.

# Context Map (Action-Based Routing)

- **[KOSIS CLI 앱 전체 작업](./kosis-cli/AGENTS.md)** — Go CLI 구현, 테스트, 실행, help 계약 수정 시.
- **[CLI 명령어 수정](./kosis-cli/cmd/AGENTS.md)** — Cobra 명령, 플래그, help, 대화형 진입 경로 수정 시.
- **[내부 모듈 수정](./kosis-cli/internal/AGENTS.md)** — API, config, output, interactive, nlp 등 내부 패키지 수정 시.
- **[프로젝트 운영 문서](./kosis-cli/docs/AGENTS.md)** — PM 체크리스트, 점수표, 평가 체크리스트 수정 시.



# Module Context

이 디렉토리는 실제 KOSIS CLI 애플리케이션 루트다. `main.go`, `cmd/`, `internal/`, `docs/`가 함께 있으며, 설계서 요구사항을 코드와 운영 문서로 구현한다.

의존 관계:
- 엔트리포인트: `main.go`
- CLI 표면: `cmd/`
- 구현 세부: `internal/`
- 작업 운영 문서: `docs/`

# Tech Stack & Constraints

- Go module: `github.com/clazic/kosis-cli`
- CLI 프레임워크: Cobra
- 설정: Viper
- 출력 포맷: JSON, CSV, XLSX, SQLite, Parquet

Constraints:
- help 문자열은 설계서와 실제 파싱 동작이 일치해야 한다.
- `go test ./...`를 통과하지 못하는 변경은 미완으로 본다.
- API 실호출이 어려운 경우에도 help, 파싱, 파일 저장 경로는 로컬에서 재현 검증한다.

# Implementation Patterns

- 명령어 표면 수정은 먼저 `cmd/`에서 처리하고, 필요한 구현만 `internal/`로 내려보낸다.
- `--output` 동작은 stdout 경로와 파일 저장 경로를 명확히 분리한다.
- help 변경 시 예제 명령이 실제로 파싱되는지 반드시 확인한다.
- 설계서 예시와 Cobra 제약이 충돌하면 루트/명령 전처리 또는 안내 문구로 일관되게 해소한다.

# Testing Strategy

- 전체 테스트: `go test ./...`
- help 검증:
  - `go run . --help`
  - `go run . data --help`
  - `go run . quick --help`
  - `go run . indicator --help`
- 대표 파싱 검증:
  - `go run . data 101 DT_TEST -c1 11 -i T10 -p M`

# Local Golden Rules

Do:
- 변경이 `cmd/` 중심인지 `internal/` 중심인지 먼저 구분한다.
- 작업 후 `docs/pm-scorecard.md` 같은 운영 문서 반영 필요 여부를 확인한다.

Don't:
- 생성 산출물 바이너리(`kosis`, `kosis-cli`, `kosis_final`, `kosis_test`)를 기준 코드처럼 수정하지 않는다.
- `.DS_Store` 같은 비본질 파일을 건드리지 않는다.
- Playwright 스냅샷·yaml·로그를 이 디렉토리(`src/`)에 떨어뜨리지 않는다. playwright 명령은 반드시 프로젝트 루트의 `.playwright/`로 `cd` 한 뒤 실행하고, 산출물도 그 안에만 저장·확인한다. (`.playwright/`, `.playwright-cli/`, `.playwright-mcp/`는 `.gitignore` 등록됨)

# Context Map

- **[CLI 명령어 계층](./cmd/AGENTS.md)** — Cobra 명령, help, 플래그, root 진입 경로 수정 시.
- **[내부 구현 패키지](./internal/AGENTS.md)** — API, 출력, NLP, interactive, config 등 세부 로직 수정 시.
- **[운영 문서와 평가](./docs/AGENTS.md)** — PM/리뷰어 체크리스트와 점수표 수정 시.
