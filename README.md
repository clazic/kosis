# kosis

KOSIS(국가통계포털) OpenAPI CLI/TUI 도구 — 한국 통계 데이터를 터미널에서 검색·조회·시각화합니다.

```bash
kosis s "미분양"                              # 통계표 검색
kosis m 116 DT_MLTM_2086                     # 메타 확인 (분류코드, 수록주기)
kosis d 116 DT_MLTM_2086 -c1 ALL -i ALL -p Y -l 5   # 데이터 조회
kosis q "서울 미분양 최근 5년"                # 자연어 한 줄 조회
```

---

## 설치

> 모든 방법은 **sudo/관리자 권한 없이** user scope에 설치됩니다.

### 방법 1: npm (macOS / Windows / Linux)

Node.js 14 이상이 설치된 환경:

```bash
npm install -g @clazic/kosis
```

설치 시 자동으로:
- `~/.claude/skills/kosis/` 에 스킬 파일 설치 (SKILL.md, references, templates)
- `~/.codex/skills/kosis/` 에 동일하게 설치
- 해당 OS 바이너리 다운로드 및 배치
- macOS/Linux: `~/.local/bin/kosis` symlink 생성

**Windows 설치 위치:**
- 스킬 파일: `%USERPROFILE%\.claude\skills\kosis\`, `%USERPROFILE%\.codex\skills\kosis\`
- 바이너리: `%USERPROFILE%\.claude\skills\kosis\apps\kosis-windows-amd64.exe`
- `kosis` 명령 실행: npm이 설치한 `bin\kosis` shim 사용 (별도 PATH 불필요)

**Windows 참고:**
- `tar` 명령이 필요합니다 (Windows 10 1803 이상 기본 내장)
- 이전 Windows 버전은 [방법 3](#방법-3-windows-powershell)를 사용하세요.

---

### 방법 2: macOS / Linux (curl)

```bash
curl -fsSL https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.sh | sh
```

설치 위치:
- 스킬 파일: `~/.claude/skills/kosis/`, `~/.codex/skills/kosis/`
- 바이너리: `~/.claude/skills/kosis/apps/kosis-<os>-<arch>`
- PATH 등록: `~/.local/bin/kosis` symlink

PATH가 등록되지 않은 경우 다음을 `~/.zshrc` 또는 `~/.bashrc`에 추가:
```bash
export PATH="$HOME/.local/bin:$PATH"
```

특정 버전 설치:
```bash
KOSIS_VERSION=v0.4.0 curl -fsSL https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.sh | sh
```

**Windows에서는 방법 3을 사용하세요.**

---

### 방법 3: Windows (PowerShell)

PowerShell에서:

```powershell
irm https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.ps1 | iex
```

설치 위치:
- 스킬 파일: `%USERPROFILE%\.claude\skills\kosis\`, `%USERPROFILE%\.codex\skills\kosis\`
- 바이너리: `%USERPROFILE%\.claude\skills\kosis\apps\kosis-windows-amd64.exe`
- PATH: 별도 등록 없이 `kosis.cmd` shim이 직접 호출

특정 버전 설치:
```powershell
$env:KOSIS_VERSION="v0.4.0"
irm https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.ps1 | iex
```

**Windows 트러블슈팅:**

| 증상 | 해결 방법 |
|------|-----------|
| 실행 정책 오류 | `Set-ExecutionPolicy RemoteSigned -Scope CurrentUser` |
| 한글 깨짐 | `chcp 65001` 실행 후 터미널 재시작 |
| `kosis` 명령 인식 안 됨 | 새 터미널 창 열기 (PATH 갱신 필요) |
| Windows Defender 경고 | `Add-MpPreference -ExclusionPath "$env:USERPROFILE\.claude\skills\kosis\apps"` |
| `tar` 명령 없음 | Windows 10 1803 미만 — [GitHub Releases](https://github.com/clazic/kosis/releases)에서 직접 다운로드 |

---

## API 키 설정

KOSIS OpenAPI 키가 필요합니다. [https://kosis.kr/openapi/](https://kosis.kr/openapi/) 에서 무료 발급.

```bash
# 대화형 설정 (권장) — 키 입력 후 자동 검증
kosis config setup

# 직접 입력
kosis config set-key <API_KEY>

# 환경변수 (CI/서버 환경)
export KOSIS_API_KEY="<API_KEY>"        # macOS/Linux (bash/zsh)
$env:KOSIS_API_KEY = "<API_KEY>"        # Windows PowerShell
setx KOSIS_API_KEY "<API_KEY>"          # Windows (영구 설정, 새 터미널에서 적용)
```

---

## 빠른 시작

```bash
# 1. 통계표 검색
kosis s "미분양"

# 2. 메타 확인 (분류코드, 항목코드, 수록주기 확인 필수)
kosis m 116 DT_MLTM_2086

# 3. 데이터 조회
kosis d 116 DT_MLTM_2086 -c1 ALL -c2 ALL -i ALL -p Y -l 5

# 자연어 조회
kosis q "서울 미분양 최근 5년"

# 차트 생성 (HTML, 브라우저에서 열기)
kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 10 --chart line --chart-format html --open

# TUI 대시보드
kosis
```

---

## 주요 명령어

| 명령어 | 별칭 | 설명 |
|--------|------|------|
| `kosis search <키워드>` | `s` | 통계표 검색 |
| `kosis meta <ORG> <TBL>` | `m` | 통계표 메타 확인 (분류/항목/주기) |
| `kosis data <ORG> <TBL>` | `d` | 통계 데이터 조회 |
| `kosis quick <요청>` | `q` | 자연어 조회 |
| `kosis chart` | | 차트 시각화 (파이프/파일 입력) |
| `kosis list` | `ls` | 통계목록 탐색 |
| `kosis explain <ORG> <TBL>` | `ex` | 통계표 설명 |
| `kosis config setup` | | 대화형 API 키 설정 |
| `kosis bookmark` | `bm` | 즐겨찾기 관리 |
| `kosis history` | `hi` | 조회 이력 |
| `kosis` | | TUI 대시보드 |

---

## 출력 형식

```bash
kosis d ... -f table    # 터미널 테이블 (기본)
kosis d ... -f md       # Markdown
kosis d ... -f json     # JSON
kosis d ... -f csv      # CSV
kosis d ... -o data.xlsx   # Excel 저장
kosis d ... -o data.db     # SQLite 저장
```

---

## 지원 플랫폼

| OS | 아키텍처 | 설치 방법 |
|----|---------|---------|
| macOS (Apple Silicon) | arm64 | 방법 1, 2 |
| macOS (Intel) | amd64 | 방법 1, 2 |
| Linux | amd64, arm64 | 방법 1, 2 |
| Windows 10 1803+ | amd64 | 방법 1, 3 |
| Windows (구버전) | amd64 | 방법 3 또는 직접 다운로드 |

---

## 개발 & 릴리스

> 기여자·메인테이너용. 이 repo에는 **CLI 코드(`src/`)와 스킬 문서(`SKILL.md`·`references/`·`LEARNINGS.md`)가 함께** 들어 있습니다.

### 저장소 구조

| 경로 | 내용 |
|------|------|
| `src/` | Go CLI 소스 (cmd/, internal/) |
| `SKILL.md` · `references/` · `LEARNINGS.md` | AI 스킬 문서 |
| `Makefile` | 멀티플랫폼 빌드 |
| `npm/` | npm 패키지(`@clazic/kosis`) — install/uninstall 스크립트 |
| `.github/workflows/release.yml` | 태그 기반 릴리스 CI |

### 로컬 개발

```bash
cd src
go build ./... && go test ./internal/...   # 빌드·테스트 (커밋 전 필수)
make build       # 5개 플랫폼 빌드 후 src/bin/, apps/ 배치
make native      # 현재 OS만 빠르게 빌드
```

### 문서(md)만 수정

```bash
# SKILL.md / references/*.md / LEARNINGS.md 편집
git add SKILL.md references/ LEARNINGS.md
git commit -m "docs: 설명 (vX.Y.Z)"
git push origin main
```

### CLI(src) 수정

```bash
cd src && go build ./... && go test ./internal/...   # 통과 확인
cd .. && git add src/ && git commit -m "feat: 설명 (vX.Y.Z)" && git push origin main
```

### 릴리스 배포 (md + CLI 함께)

릴리스는 **`v*` 태그 push 한 번으로 CI가 전부 자동 수행**합니다. 버전은 태그명에서 자동 주입되므로(`-X main.version`, npm version, `VERSION` 파일) 파일에 손으로 버전을 박지 않아도 됩니다.

```bash
git tag -a v0.6.0 -m "v0.6.0: 설명"
git push origin v0.6.0          # ← 이 한 줄이 릴리스 전체를 트리거
```

| CI Job | 동작 |
|--------|------|
| `build` | 5개 플랫폼 바이너리 빌드 |
| `skill-package` | SKILL.md·LEARNINGS.md·references 등 스킬 tarball 생성 + `VERSION` 갱신 |
| `release` | GitHub Release 생성 (바이너리 + 스킬 tarball + SHA256SUMS) |
| `npm-publish` | `@clazic/kosis@<버전>` publish (OIDC) |

**커밋/버전 규칙**: 메시지는 `type: 설명 (vX.Y.Z)` 형식(`feat`/`fix`/`docs`/`chore`/`refactor`/`perf`). 기능 추가는 minor(`feat`), 버그 수정은 patch(`fix`).

### ⚠️ 심링크 개발 환경 주의

개발 머신에서 `~/.claude/skills/kosis`를 이 repo로 **심볼릭 링크**해 두면, repo에서 md를 편집하는 즉시 스킬에 반영되어 편리합니다. 단 이 경우:

- **`kosis update`를 실행하지 마세요.** update는 릴리스의 스킬 파일을 스킬 디렉토리에 덮어쓰므로, 심링크 상태에서는 **repo 작업본을 덮어쓸 위험**이 있습니다.
- 개발 머신에서는 `kosis update` 대신 **`git pull`**로 동기화하세요. `kosis update`는 일반(비심링크) 설치 사용자용입니다.

---

## 관련 링크

- 스킬 가이드: [SKILL.md](SKILL.md)
- 상세 문서: [references/](references/)
- 키 발급: [https://kosis.kr/openapi/](https://kosis.kr/openapi/)
- 릴리스: [GitHub Releases](https://github.com/clazic/kosis/releases)
- 이슈/피드백: [GitHub Issues](https://github.com/clazic/kosis/issues)

---

## 라이선스

MIT
