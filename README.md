# kosis

KOSIS(국가통계포털) OpenAPI CLI/TUI 도구 — 한국 통계 데이터를 터미널에서 검색·조회·시각화합니다.

```bash
kosis s "미분양"                              # 통계표 검색
kosis m 116 DT_MLTM_2086                     # 메타 확인 (분류코드, 수록주기)
kosis d 116 DT_MLTM_2086 -c1 ALL -i ALL -p Y -l 5   # 데이터 조회
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
- `~/.claude/skills/kosis/` 에 스킬 파일 설치 (SKILL.md, references, templates) — 기본 대상은 Claude, `KOSIS_TARGET=claude,codex,opencode,hermes` 또는 `all` 로 변경 가능
- 해당 OS 바이너리 다운로드 후 `~/.local/bin/kosis` 에 배치
- PATH에 `~/.local/bin`이 없으면 추가 방법 안내 출력

**Windows 설치 위치:**
- 스킬 파일: `%USERPROFILE%\.claude\skills\kosis\`
- 바이너리: `%LOCALAPPDATA%\Programs\kosis\kosis.exe`
- PATH: 사용자 환경변수에 자동 등록 — **새 터미널부터** `kosis` 명령 인식

**Windows 참고:**
- `tar` 명령이 필요합니다 (Windows 10 1803 이상 기본 내장)
- 이전 Windows 버전은 [방법 3](#방법-3-windows-powershell)를 사용하세요.

---

### 방법 2: macOS / Linux (curl)

```bash
curl -fsSL https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.sh | sh
```

실행 시 설치 대상(Claude Code / Codex·OpenCode / Hermes, 여러 개 선택 가능)과 범위(전역/프로젝트, Hermes는 전역만)를 대화형으로 선택합니다. 비대화형(CI) 환경은 `KOSIS_TARGET`, `KOSIS_CLAUDE_SCOPE`, `KOSIS_CODEX_SCOPE` 환경변수로 지정합니다.

설치 위치:
- 스킬 파일 (선택한 대상마다):
  | 대상 | 위치 |
  |---|---|
  | Claude Code | `~/.claude/skills/kosis/` |
  | Codex · OpenCode | `~/.agents/skills/kosis/` (두 도구가 같은 곳을 읽음. OpenCode는 `~/.claude/skills`도 읽음) |
  | Hermes Agent | `~/.hermes/skills/kosis/` |
- 바이너리: `~/.local/bin/kosis`

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

실행 시 설치 대상(Claude Code / Codex·OpenCode / Hermes, 여러 개 선택 가능)과 범위(전역/프로젝트, Hermes는 전역만)를 대화형으로 선택합니다. 비대화형 환경은 `KOSIS_TARGET`, `KOSIS_CLAUDE_SCOPE`, `KOSIS_CODEX_SCOPE` 환경변수로 지정합니다.

설치 위치:
- 스킬 파일: `%USERPROFILE%\.claude\skills\kosis\`, `%USERPROFILE%\.agents\skills\kosis\`(Codex·OpenCode), `%USERPROFILE%\.hermes\skills\kosis\`(Hermes) 중 선택한 곳
- 바이너리: `%LOCALAPPDATA%\Programs\kosis\kosis.exe`
- PATH: 사용자 환경변수에 자동 등록 — **새 터미널부터** `kosis` 명령 인식

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
| Windows Defender 경고 | `Add-MpPreference -ExclusionPath "$env:LOCALAPPDATA\Programs\kosis"` |
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

### PES 중계 모드 (pes-bridge)

PES 의 외부 AI(pes-bridge)가 `kosis` 를 돌릴 때는 CLI 가 키를 받지 않습니다. 브리지가 아래 환경변수를 주면 `kosis` 는 **중계 모드**가 됩니다.

| 환경변수 | 설명 |
|----------|------|
| `PES_RELAY_URL` | 중계 주소 (예: `http://127.0.0.1:53999`). 있으면 중계 모드 |
| `PES_RELAY_TOKEN` | 이번 실행에만 통하는 토큰 — 요청마다 `X-PES-Relay-Token` 머리글로 보냄 |
| `PES_RELAY_KOSIS_KEYS` | PES 에 등록된 KOSIS 키 수 |

- `~/.kosis/config.yaml` 의 키와 `KOSIS_API_KEY` 는 쓰지 않습니다. 키 풀은 자리표시 `__PES_KEY_0__` … 이고(병렬 · 돌려 쓰기 · 429 쿨다운은 그대로), 중계가 실제 키로 바꿉니다.
- KOSIS 로 가는 모든 요청은 `{PES_RELAY_URL}/kosis/<scheme>/<host>/<path>?<query>` 로 갑니다 (예: `…/kosis/https/kosis.kr/openapi/statisticsSearch.do?…`).
- 키 수가 0 이거나 변수가 없으면 기관을 부르지 않고 `PES 에 KOSIS 키가 없어요. PES 「도구」에서 키를 넣으세요.` 를 알리고 0 이 아닌 코드로 끝납니다.
- 중계가 낸 오류(`X-PES-Relay-Error: 1`)는 그 글을 그대로 보여 주고 0 이 아닌 코드로 끝납니다 (다른 키로 다시 해 보지 않음).
- 업데이트 확인 알림은 꺼지고, `kosis config setup` 은 쓸 수 없습니다. `kosis config key-list` 는 PES 키 수를 알려 줍니다.
- `kosis-mcp` 서버는 중계 모드를 쓰지 않습니다 (지금처럼 자기 키로 KOSIS 를 직접 부름).

---

## 빠른 시작

```bash
# 1. 통계표 검색
kosis s "미분양"

# 2. 메타 확인 (분류코드, 항목코드, 수록주기 확인 필수)
kosis m 116 DT_MLTM_2086

# 3. 데이터 조회
kosis d 116 DT_MLTM_2086 -c1 ALL -c2 ALL -i ALL -p Y -l 5

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

## MCP 서버 (kosis-mcp)

같은 저장소에 **stdio MCP 서버** `kosis-mcp` 가 있습니다. CLI(`kosis`)와 같은 API 클라이언트·키 풀(429 쿨다운)·4만 셀 자동 분할·파일 캐시·조회 전후 검사(빠진/없는 분류 축, 없는 주기, 결과에 없는 코드, 잠정치·전망 시점 안내 등)를 함수로 직접 씁니다. 도구는 모두 **읽기 전용**이고, 파일은 캐시(`~/.kosis/cache`) 말고는 쓰지 않습니다. CLI 명령·동작은 그대로입니다.

### 설치 (릴리스 자산)

릴리스마다 CLI 와 같은 태그로 `kosis-mcp-<os>-<arch>` 가 올라가고, 같은 `SHA256SUMS` 에 들어 있습니다.

| OS / ARCH | 자산 |
|-----------|------|
| macOS Apple Silicon | `kosis-mcp-darwin-arm64` |
| macOS Intel | `kosis-mcp-darwin-amd64` |
| Linux | `kosis-mcp-linux-amd64`, `kosis-mcp-linux-arm64` |
| Windows | `kosis-mcp-windows-amd64.exe` |

```bash
# 예: macOS Apple Silicon
VER=v0.6.0   # 원하는 릴리스 태그
BASE=https://github.com/clazic/kosis/releases/download/$VER
curl -fsSLO $BASE/kosis-mcp-darwin-arm64 && curl -fsSLO $BASE/SHA256SUMS
grep ' kosis-mcp-darwin-arm64$' SHA256SUMS | shasum -a 256 -c -
install -m 755 kosis-mcp-darwin-arm64 ~/.local/bin/kosis-mcp
kosis-mcp --version
```

소스에서: `cd src && make mcp` (→ `src/bin/kosis-mcp-<os>-<arch>`) 또는 `go build -o kosis-mcp ./cmd/kosis-mcp`.

### 연결 (stdio)

Claude Code:

```bash
claude mcp add kosis --env KOSIS_API_KEYS=<키1>,<키2> -- ~/.local/bin/kosis-mcp
```

또는 `.mcp.json`:

```json
{
  "mcpServers": {
    "kosis": { "command": "/Users/me/.local/bin/kosis-mcp", "env": { "KOSIS_API_KEYS": "<키1>,<키2>" } }
  }
}
```

Codex (`~/.codex/config.toml`):

```toml
[mcp_servers.kosis]
command = "/Users/me/.local/bin/kosis-mcp"
env = { KOSIS_API_KEYS = "<키1>,<키2>" }
```

`KOSIS_API_KEYS` 를 빼면 `~/.kosis/config.yaml`(`kosis config set-key`/`add-key` 로 넣은 키, `KOSIS_API_KEY` 환경변수 포함)을 씁니다.

### HTTP 모드 (Streamable HTTP, 게이트웨이 없이)

```bash
kosis-mcp --http                      # 127.0.0.1:8080, 경로 /mcp
kosis-mcp --http 127.0.0.1:9100
KOSIS_MCP_TOKEN=<긴 무작위 값> kosis-mcp --http 0.0.0.0:9100   # loopback 이 아니면 토큰 필수 (없으면 시작 거부)
```

| 환경변수 | 설명 |
|----------|------|
| `KOSIS_MCP_TOKEN` | 있으면 모든 요청에 `Authorization: Bearer <토큰>` 필수 (상수 시간 비교, 틀리면 401) |
| `KOSIS_MCP_ALLOWED_ORIGINS` | 허용할 `Origin` (쉼표). `Origin` 머리글이 오면 loopback 출처(`http://localhost:*`, `http://127.0.0.1:*`) 또는 이 목록만 허용, 아니면 403 |
| `KOSIS_API_KEYS` | 기본 키 (stdio 와 같음) |

- MCP 2025-06-18 Streamable HTTP: `POST /mcp`(JSON-RPC 하나 또는 배치 10개까지, 본문 1MB 상한), `GET /mcp`(세션의 서버 알림 SSE, 15초마다 `: ping`), `DELETE /mcp`(세션 끝).
- `initialize` 응답의 `Mcp-Session-Id` 머리글을 이후 요청에 보낸다. 없으면 400, 모르는(끝났거나 1시간 쓰지 않은) 세션이면 404. `MCP-Protocol-Version` 머리글이 모르는 값이면 400.
- `tools/call` 에 `_meta.progressToken` 이 있고 `Accept` 에 `text/event-stream` 이 있으면 SSE 로 답한다: `notifications/progress` 이벤트들(시작, `kosis_data` 분할 조회 조각 n/전체 — 1초에 한 번까지, 끝) → 마지막에 응답 이벤트. 아니면 `application/json`.
- 요청별 키: 머리글 `X-Api-Keys: k1,k2` → 그 POST 의 `tools/call` 모두에 `_meta["kr.pes/keys"]` 와 같게 적용 (그 호출에 `_meta` 키가 있으면 `_meta` 가 먼저). 기본 키로 넘어가지 않는다.
- 취소: `notifications/cancelled {requestId}` 를 같은 세션으로 POST 하거나, 응답 중인 POST 연결이 끊기면 그 요청의 KOSIS 호출·분할 조회를 멈추고 답하지 않는다.
- 응답·SSE 이벤트 모두 키를 지운다. stderr 접근 기록은 메서드·도구 이름·걸린 시간만.

Claude Code (HTTP):

```bash
claude mcp add --transport http kosis http://127.0.0.1:8080/mcp --header "Authorization: Bearer <토큰>"
```

Codex (`~/.codex/config.toml`, HTTP):

```toml
[mcp_servers.kosis]
url = "http://127.0.0.1:8080/mcp"
bearer_token_env_var = "KOSIS_MCP_TOKEN"
```

curl 로 확인:

```bash
U=http://127.0.0.1:8080/mcp
SID=$(curl -s -D - -o /dev/null -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' $U \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"curl","version":"0"}}}' \
  | awk -F': ' 'tolower($1)=="mcp-session-id"{print $2}' | tr -d '\r')
curl -sN -H 'Content-Type: application/json' -H 'Accept: application/json, text/event-stream' -H "Mcp-Session-Id: $SID" $U \
  -d '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"kosis_data","arguments":{"org_id":"101","tbl_id":"DT_1B040A3","class1":"ALL","item":"T20","period":"M","start":"201601","end":"202512"},"_meta":{"progressToken":"p1"}}}'
```

### 진행 알림 · 취소 (stdio·HTTP 공통)

- `tools/call` 의 `params._meta.progressToken` 이 있으면 `notifications/progress {progressToken, progress, total?, message}` 를 보낸다 (stdio 는 stdout 한 줄, HTTP 는 그 POST 의 SSE). 없으면 보내지 않는다.
- `notifications/cancelled {requestId}` 를 받으면 그 요청을 멈추고 답하지 않는다.

### 키

| 방법 | 설명 |
|------|------|
| `KOSIS_API_KEYS` | 기본 키. 쉼표로 여러 개 (여러 개면 병렬 분할 조회·429 쿨다운에 고루 쓰임) |
| `~/.kosis/config.yaml` | `KOSIS_API_KEYS` 가 없을 때 기본 키 (CLI 설정 그대로) |
| `params._meta["kr.pes/keys"]` | `tools/call` 마다 줄 키 (문자열 배열). 있으면 **그 키들만** 그 요청 전용 키 풀로 쓰고 기본 키로 넘어가지 않습니다. 빈 배열·형식 오류는 오류 |

```json
{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{
  "name":"kosis_search","arguments":{"keyword":"인구","limit":1},
  "_meta":{"kr.pes/keys":["<요청별 키>"]}}}
```

**키별 사용량** — 요청별 키(`_meta["kr.pes/keys"]` 또는 HTTP `X-Api-Keys`)로 한 `tools/call` 이 KOSIS 에 요청을 보냈으면, 도구 결과(`result`, `content`·`isError` 옆)의 `_meta["kr.pes/key-usage"]` 에 키별 요청 수가 붙습니다 (부른 쪽이 키마다 사용량을 세는 데 씁니다).

```json
{"jsonrpc":"2.0","id":7,"result":{
  "content":[{"type":"text","text":"…"}],"isError":false,
  "_meta":{"kr.pes/key-usage":[{"i":0,"calls":3,"errors":0,"limited":1},{"i":2,"calls":1,"errors":1,"limited":0}]}}}
```

- `i`: 받은 요청별 키 목록에서 그 키의 자리(0부터). 같은 키가 여러 번 오면 처음 자리, `X-Api-Keys` 는 빈 칸을 뺀 자리. 키 값은 어떤 꼴로도 넣지 않습니다.
- `calls`: 그 키로 KOSIS 에 보낸 HTTP 요청 수(429 재시도도 한 번씩). `errors`: 그중 실패(전송 오류·HTTP 오류·KOSIS 오류 본문). `limited`: 그중 HTTP 429 (`errors` 에 포함).
- 요청을 보내지 않은 키는 빠지고, 요청이 하나도 없으면(캐시에서 답함, `kosis_result_page`·`kosis_result_query`) 항목 자체가 없습니다. 성공·`isError: true` 결과 모두에 붙고, stdio·HTTP(JSON·SSE) 같습니다.
- 기본 키(`KOSIS_API_KEYS`·설정 파일)로 한 호출에는 붙지 않습니다.

키가 하나도 없으면 도구 결과가 `isError: true` 와 "API 키가 없어요" 안내입니다. 키는 인자·로그·stderr·결과·오류 어디에도 원문으로 나가지 않습니다 — 응답 직전 모든 텍스트에서 요청 키와 기본 키(그대로·URL 인코딩)를 `***<끝4자리>` 로, 남은 `apiKey=값` 은 `apiKey=***` 로 바꿉니다. 파일 캐시의 키·파일 이름·내용에도 API 키가 들어가지 않습니다(응답에 키가 섞여 오면 캐시하지 않음).

### 도구

| 도구 | 하는 일 | 주요 인자 |
|------|---------|-----------|
| `kosis_search` | 통계표 검색 (`kosis s`) | `keyword`(필수), `limit`, `page`, `sort`(RANK/DATE), `org`, `stat`, `region` |
| `kosis_meta` | 분류 축·항목·주기 (`kosis m`) — 요약, 코드 고르기(`data_params` 를 `kosis_data` 에 그대로), 원본 메타 | `org_id`·`tbl_id`(필수), `type`, `find`, `under`, `level`, `axis` |
| `kosis_data` | 값 조회 (`kosis d`) — 조회 전 검사, 4만 셀·URL 길이 자동 분할, 결과 `notices`(조용한 오답 경고), 큰 결과는 저장·쪽 나누기 | `org_id`·`tbl_id`·`item`·`period`(필수), `class1`…`class8`, `start`+`end` \| `latest`(정수 N 또는 `true`) \| `periods`, `fields`, `format`, `save`, `filter`, `aggregate`, `pivot` |
| `kosis_batch` | 서로 독립인 `kosis_data` 조회 여러 개(최대 10)를 한 번에 — 동시에 받아 `results`(조회마다 `rows`·`notices` 또는 `error`) 하나로. 왕복을 줄인다 | `queries`(필수, `kosis_data` 인자 객체 배열) |
| `kosis_ratio` | 두 조회의 비율 (`kosis ratio`) — 다른 두 표(또는 같은 표의 두 코드·항목)를 이름·시점으로 맞춰 나눔. 결과 `rows`: `NAME`·`PRD_DE`·`NUM`·`DEN`·`RATIO`(+`RANK`) | `num`·`den`(필수, `kosis_data` 의 조회 인자 객체), `scale`(100=%, 1=배수), `no_total`, `sort`, `top` |
| `kosis_rules` | 검증된 규칙·표·못 찾은 기록 (`kosis rules list`, **키 불필요**) — 바이너리에 내장돼 스킬 폴더 없이도 동작 | `query`, `id`(규칙 id·`ORG/TBL`), `kind`, `topic` 중 하나 이상 |
| `kosis_list` | 통계목록 트리 (`kosis ls`) | `view`, `parent` |
| `kosis_explain` | 조사 설명·주석·원표 링크 (`kosis ex`) | `org_id`·`tbl_id`(필수) |
| `kosis_indicator` | 주요지표 (`kosis ind`) | `action`(search/data/info/list, 필수), `name`, `id`, `list_id`, `limit` |
| `kosis_result_page` | 저장된 큰 결과의 다음 쪽 (다시 조회 안 함, **키 불필요**) | `cursor`(필수, 앞 응답의 `next_cursor`), `limit` |
| `kosis_result_query` | 저장된 결과에 filter → latest → aggregate → order·top → pivot (**키 불필요**) | `result_id`(필수), `latest`, `filter`, `aggregate`, `order`, `order_by`, `top`, `pivot`, `format`, `save`, `limit` |

**계산은 도구가 합니다** — `kosis_data` 에 `sort`(desc·asc)·`top`·`no_total` 을 주면 순위(`RANK` 열, 동률은 같은 순위), `change`(yoy·prev)면 증감 `DT_CHG`·증감률 `DT_CHG_PCT`, `ratio`(`"분자/분모"`)면 같은 축 두 코드의 비율 `DT_RATIO_PCT` 가 붙습니다 (`kosis d --sort/--top/--no-total/--change/--ratio` 와 같은 계산, `sort_by` 로 정렬 기준). 계산은 `filter`·`aggregate`·`pivot` 보다 먼저 합니다. 저장된 결과는 `kosis_result_query` 의 `order`·`order_by`·`top` 으로 정렬합니다(aggregate 뒤, pivot 앞). 다른 두 표의 비율은 `kosis_ratio`.

**규칙이 결과에 함께 옵니다** — `kosis_search` 의 `notices` 에 검색어에 맞는 검증된 표와 못 찾은 기록, `kosis_meta` 의 `rules` 와 `kosis_data` 의 `notices` 에 그 표의 규칙(`규칙(배포 2026-10-13): …`, 최대 8줄)이 붙습니다. 같은 말로 불리지만 표·정의가 다른 지표(자가보유율/점유율, 생산액/출하액, 산업용/제조업 전력 등)는 `규칙 혼동 주의(…): ① … / ② …` 줄로 각 지표의 표·코드·확인한 값을 함께 보여 줍니다(`kosis_rules kind=concept` 로 목록). 규칙은 바이너리에 내장돼 있고(`KOSIS_RULES_FILE` → 설치된 스킬 폴더 → 내장 순), `~/.kosis/rules.d` 의 로컬 규칙이 앞에 옵니다.

결과는 JSON 텍스트입니다. 검색·메타·목록 등은 약 200KB 를 넘으면 앞부분만 담고 `truncated`("잘림: … 범위를 좁히세요")를 붙입니다(표 결과는 아래 "큰 결과"). `tools/call` 여러 개가 동시에 와도 따로 처리합니다. 대량 다운로드·차트·설정·업데이트·즐겨찾기·이력 명령은 MCP 도구에 없습니다.

### 큰 결과 (압축 형식 · 쪽 나누기 · 서버 집계 · 전체 파일)

도구 응답 상한(약 200KB, AI 맥락 보호)은 그대로 두고, 표 결과(`kosis_data`, `kosis_result_query`)를 더 촘촘히 · 나눠서 · 줄여서 · 파일로 받습니다.

- **요약 칸**(응답 맨 앞): `total_rows`, `returned_rows`, `truncated`, `format`, `result_id`, `next_cursor`, `download`.
- **작은 결과**(상한 안, `save` 아님): 예전처럼 바로, 형식 기본 `json`(`rows` = 객체 배열, `labels` = 열 이름).
- **큰 결과** 또는 `save: true`: 결과 저장소에 두고 첫 쪽만 돌려준다. 형식 기본 `columns`:
  `{"columns":["C1","ITM_ID","PRD_DE","DT",…], "rows":[["11","T20","202401","9384512"],…], "dict":{"C1":{"11":"서울특별시",…},"ITM_ID":{"T20":"총인구수",…}}}` — 이름 열(C1_NM·ITM_NM)은 `dict` 로 한 번만. `format: "csv"` 는 `csv` 칸에 머리줄+행.
- **다음 쪽**: `kosis_result_page {cursor: next_cursor}` — 같은 형식, 마지막 쪽엔 `next_cursor` 없음. 만료되면 "결과가 만료됐어요 — 다시 조회하세요".
- **서버 집계**: `kosis_data`·`kosis_result_query` 에 `filter`(`{"C1":["11","26"],"PRD_DE":{"from":"2020","to":"2024"}}`), `latest: true`(계열마다 가장 최근 시점), `aggregate`(`{"group_by":["C1","PRD_DE"],"op":"sum|avg|min|max|count","value":"DT"}` — 숫자 아닌 값은 건너뛰고 알림), `pivot`(`{"rows":["C1"],"cols":"PRD_DE","value":"DT"}`). 순서는 filter → latest → aggregate → pivot.
- **전체 파일**: 저장된 결과는 `resource_link`(`kosis://results/<id>.csv`, `text/csv`, 크기·행 수) 로 알린다. `resources/read {uri}` 로 받거나(20MB 넘으면 오류 — 내려받기를 쓰라고), HTTP 모드에서는 `GET /results/<id>.csv`(같은 Bearer 토큰, Content-Length·Content-Disposition). CSV 는 UTF-8(BOM), 머리줄은 API 키.
- 저장 파일: `<id>.jsonl`(쪽 나누기) · `<id>.csv` · `<id>.meta.json`. id 는 무작위 144bit(맞히기 불가 = 읽기 권한). 키·토큰은 파일에 넣지 않는다. 임시 파일 → rename 이라 여러 프로세스가 같은 폴더를 써도 된다.

| 환경변수 | 기본 | 설명 |
|----------|------|------|
| `MCP_RESULTS_DIR` | `~/.kosis/cache/mcp-results` | 결과 저장 폴더 (게이트웨이가 정해 넘기면 게이트웨이가 같은 폴더에서 직접 내려준다) |
| `MCP_RESULTS_TTL` | `1h` | 보관 기간 (초 또는 `90m` 같은 기간). 시작할 때와 10분마다 정리 |
| `MCP_RESULTS_MAX_BYTES` | `1073741824` (1GB) | 폴더 상한 — 넘으면 오래된 결과부터 지운다 |

실측(2026-11-16, 101/DT_1B040A3 주민등록인구 월별, 시군구 ALL × 3항목 × 2011.01~2025.12 = 153,531행, 60조각 분할): `fields: "C1,PRD_DE,DT"` 일 때 첫 쪽 `columns` 7,263행 / `json` 4,291행, 전체 열이면 `columns` 3,426행 · 45쪽 합계 153,531행, CSV 10,093,238바이트, `kosis_result_query` 집계(C1=00, ITM_ID·PRD_DE 별 sum → 540행) 0.08초.

### smoke (게이트웨이 자동 업그레이드용)

1. 키 없이: `initialize` → `notifications/initialized` → `tools/list` 가 성공하고 도구 11개(`kosis_search`, `kosis_meta`, `kosis_data`, `kosis_result_page`, `kosis_result_query`, `kosis_batch`, `kosis_ratio`, `kosis_rules`, `kosis_list`, `kosis_explain`, `kosis_indicator`)가 나와야 합니다.
2. 키가 있으면: 도구 `kosis_search`, 인자 `{"keyword":"인구","limit":1}` → `isError: false`, `results` 1개.

```bash
printf '%s\n' \
  '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke","version":"0"}}}' \
  '{"jsonrpc":"2.0","method":"notifications/initialized"}' \
  '{"jsonrpc":"2.0","id":2,"method":"tools/list"}' \
  '{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"kosis_search","arguments":{"keyword":"인구","limit":1}}}' \
  | KOSIS_API_KEYS=<키> kosis-mcp
```

---

## 개발 & 릴리스

> 기여자·메인테이너용. 이 repo에는 **CLI 코드(`src/`)와 스킬 문서(`SKILL.md`·`references/`·`LEARNINGS.md`)가 함께** 들어 있습니다.

### 저장소 구조

| 경로 | 내용 |
|------|------|
| `src/` | Go CLI 소스 (cmd/, internal/) · MCP 서버 `src/cmd/kosis-mcp/` |
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
| `build` | 5개 플랫폼 바이너리 빌드 (CLI `kosis-*` + MCP 서버 `kosis-mcp-*`) |
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
