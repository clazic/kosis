# KOSIS CLI 프로젝트 규칙

## 작업 규칙 (공통)

- 작업 시작 전 반드시 아래 파일을 모두 읽고 숙지할 것:
  - `SKILL.md` (사용 가이드 — 사실상의 설계 계약)
  - `references/13-reference.md` (명령·플래그 레퍼런스)
- `--help` 출력은 계약의 일부로 취급하며, `SKILL.md`·`references/` 기준에 맞게 정확히 구현·유지한다.
- 구현과 문서가 다르면 코드와 문서 중 하나를 반드시 맞춘다.

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
- 네이티브(darwin/arm64)는 `CGO=1`(SQLite 포함 완전 기능), 나머지는 `CGO=0`(SQLite 제외). 정식 완전 바이너리는 CI(`.github/workflows/release.yml`)가 생성한다.
- 현재 플랫폼만 빠르게 필요하면 `make native`.

## 참조 파일
- 사용 가이드(설계 계약): `SKILL.md`
- 명령 레퍼런스: `references/13-reference.md`
- 상세 가이드: `references/` (01~16 넘버링 문서)
- 학습 노트(오답노트): `LEARNINGS.md`
- CLI 소스: `src/` (cmd/, internal/)
