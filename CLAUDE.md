# KOSIS CLI 프로젝트 규칙

## 프로젝트 팀 구성

- **PM (opus)**: 업무배정 및 관리, 체크리스트 작성, 느슨함 없이 관리
- **개발자 4명 (haiku)**: 개발 및 자료검색, 병렬 작업 수행
- **평가자 3명 (haiku)**: 코드리뷰, 평가, 테스트 (100점 만점 기준)

## 작업 규칙

### 공통
- 작업 시작 전 반드시 아래 파일을 모두 읽고 숙지할 것:
  - `docs/superpowers/specs/2026-03-31-kosis-cli-design.md` (설계서)
  - `skills/kosis-cli/SKILL.md` (사용 가이드)

### PM
- 먼저 작업 업무배정 계획을 세우고, 업무의 체크리스트를 만들어 작업을 분배한다
- 작업이 느슨함이 없게 관리한다
- 개발자 3명에게 병렬로 작업을 분배하여 효율을 극대화한다

### 개발자
- PM의 업무계획에 따라 작업을 진행한다
- 작업 완료 후 평가자에게 평가를 받는다
- 평가 점수가 100점일 때까지 반복해서 수정하고 다시 평가를 받는다
- --help 출력도 설계서 기준에 맞게 정확히 구현한다

### 평가자
- PM의 업무계획을 잘 보고 개발자들의 업무에 맞는 체크리스트를 만든다
- 개발자가 평가를 받으려고 하면 높은 기준으로 평가한다
- 100점이 될 때까지 계속 반복한다
- --help도 설계서 기준에 맞는지 정확히 확인한다
- 코드 품질, 에러 처리, 테스트, 설계서 준수 여부를 모두 확인한다

## Playwright 사용 규칙 (필수)
- Playwright(`playwright-cli` / MCP) 산출물은 **프로젝트 루트의 `.playwright/` 폴더 안에만** 저장한다.
- `playwright-cli`는 출력 폴더(`.playwright-cli/`)와 스냅샷을 **실행 위치(cwd) 바로 아래에 생성**하므로, 모든 playwright 명령은 실행 전 반드시 `cd <프로젝트루트>/.playwright` 후 실행한다.
- `snapshot --filename` 등 파일 저장 옵션의 경로도 항상 `.playwright/` 내부로 지정한다.
- 결과 확인도 `.playwright/` 폴더만 본다. `src/` 등 프로젝트의 다른 위치에 스냅샷·yaml·로그를 절대 떨어뜨리지 않는다.
- `.playwright/`, `.playwright-cli/`, `.playwright-mcp/`는 git에서 무시된다(`.gitignore` 등록 완료).

## 계획 수립 규칙 (필수)
- 모든 계획서·설계 문서는 프로젝트 루트의 **`.plan/` 폴더**에 작성한다.
- 파일명 형식: `.plan/YYYY-MM-DD-HH:MM:SS-제목.md` (예: `.plan/2026-06-20-09:30:00-스킬-구조-정리.md`)
- `.plan/`은 스킬 개발 중에만 쓰는 폴더이며, 스킬 배포 산출물에는 포함하지 않는다.

## 빌드 규칙 (필수)
- CLI(`src/`)를 개발·수정한 뒤에는 **항상 멀티플랫폼 전체를 빌드**한다: `cd src && make build`
- 빌드 산출물은 `src/bin/` 과 스킬 루트 `apps/` **두 곳 모두**에 자동 배치된다 (Makefile이 처리).
- 파일명 컨벤션: `kosis-<os>-<arch>` (darwin-arm64 / darwin-amd64 / linux-amd64 / linux-arm64 / windows-amd64.exe)
- 네이티브(darwin/arm64)는 `CGO=1`(SQLite 포함 완전 기능), 나머지는 `CGO=0`(SQLite 제외). 정식 완전 바이너리는 CI(`src/.github/workflows/release.yml`)가 생성한다.
- 현재 플랫폼만 빠르게 필요하면 `make native`.

## 참조 파일
- 설계서: `docs/superpowers/specs/2026-03-31-kosis-cli-design.md`
- 사용 가이드: `skills/kosis-cli/SKILL.md`
- 프로젝트 소스: `kosis-cli/` 디렉토리
