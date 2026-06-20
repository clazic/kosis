---
name: kosis
description: KOSIS(국가통계포털) OpenAPI로 한국 통계 데이터를 검색·조회·내보내기하는 CLI/TUI 도구. "인구 통계", "GDP 조회", "미분양 현황", "소비자물가 월별" 등 통계 데이터 요청 시 사용.
user-invocable: true
argument-hint: "<통계 키워드 또는 명령>"
---

# KOSIS

## 학습 루프 (AI 필독 — 스킬이 진화하는 방식)

**KOSIS 작업을 시작하면 가장 먼저 [LEARNINGS.md](LEARNINGS.md)를 읽는다.** 과거의 실패·함정·성공 쿼리가 누적된 "오답노트"다.

- **조회 전 READ**: `LEARNINGS.md`의 `테이블별 함정`·`오류코드 대응`을 확인해 같은 실수를 반복하지 않는다.
- **작업 후 APPEND**: 예상 못한 실패나 처음 통한 쿼리·해결한 함정을 겪으면 `LEARNINGS.md`의 해당 섹션에 **한 줄 이상** 추가한다("왜"를 반드시 포함). 이렇게 쓸수록 스킬이 똑똑해진다.

---

## First-Run Onboarding (AI 필독)

KOSIS 명령 실행 전 반드시 다음 순서를 따를 것:

0. **바이너리 확인**: 스킬 폴더에 OS에 맞는 `apps/kosis-<os>-<arch>` 바이너리가 없으면 `sh scripts/install-binary.sh`를 실행해 GitHub Release에서 자동 다운로드한다. (별도 런처 없이 SKILL.md가 직접 관리)
1. `kosis config key-list` 실행 → 키 존재 여부 확인
2. 키 0개이고 `KOSIS_API_KEY` 환경변수도 없으면 → **실행 중단 후 안내:**
   - "KOSIS API 키가 필요합니다. https://kosis.kr/openapi/ 에서 발급 후 알려주시면 자동으로 등록합니다."
3. 사용자가 키 제공 → `kosis config set-key <KEY>` 자동 실행
4. `kosis s "인구" -n 1` 으로 유효성 검증
5. 검증 실패 시 → 키 오류 안내 + 재발급 URL 제시

---

## 설치

| 방법 | 명령 |
|------|------|
| npm | `npm install -g kosis` |
| macOS/Linux | `curl -fsSL https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.sh \| sh` |
| Windows | `irm https://raw.githubusercontent.com/clazic/kosis/master/scripts/install.ps1 \| iex` |

## 업데이트

```bash
kosis update          # 최신 버전으로 업데이트 (바이너리 + 스킬 파일 함께)
kosis update --check  # 업데이트 확인만 (설치 안 함)
kosis update --force  # 같은 버전이어도 강제 재설치
```

GitHub 최신 릴리스에서 OS별 바이너리와 스킬 파일(SKILL.md, references/, LEARNINGS.md, templates 등)을 함께 내려받아 `~/.claude/skills/kosis`(및 `~/.codex/skills/kosis`)에 반영합니다. macOS/Linux/Windows 모두 지원합니다.

---

## 빠른 시작

```bash
kosis config set-key <API_KEY>          # API 키 설정
kosis s "미분양"                         # 1. 검색
kosis m 116 DT_MLTM_2086               # 2. 메타 확인
kosis d 116 DT_MLTM_2086 -c1 ALL -c2 ALL -i ALL -p Y -l 5   # 3. 데이터 조회
kosis q "서울 미분양 최근 5년"            # 또는 자연어 한 줄
```

> API 키: https://kosis.kr/openapi/ 에서 발급

---

## 환경

**배포 바이너리 경로 (스킬 루트 기준, 파일명 컨벤션 `kosis-<os>-<arch>`):**

| OS / ARCH | 경로 |
|-----------|------|
| macOS arm64 | `apps/kosis-darwin-arm64` |
| macOS amd64 | `apps/kosis-darwin-amd64` |
| Linux amd64 | `apps/kosis-linux-amd64` |
| Linux arm64 | `apps/kosis-linux-arm64` |
| Windows amd64 | `apps/kosis-windows-amd64.exe` |

**빌드 (개발 시 — 항상 멀티플랫폼):**

```bash
cd src
make build      # 5개 플랫폼 빌드 후 src/bin/ 과 apps/ 두 곳에 자동 배치
# make native   # 현재 플랫폼(darwin/arm64)만 빠르게
```

> 정식 완전 바이너리(SQLite 포함)는 CI(`.github/workflows/release.yml`, CGO=1)가 생성한다.
> 로컬 `make build`는 네이티브(darwin/arm64)만 CGO=1이고 나머지는 CGO=0이라 SQLite 출력 기능이 제외된다.

**바이너리 탐색 순서 (실행 시):**

| 순서 | 경로 |
|------|------|
| 1. 스킬 apps | `apps/kosis-<os>-<arch>` (없으면 `scripts/install-binary.sh`가 Release에서 다운로드) |
| 2. 로컬 빌드 | `src/bin/kosis-<os>-<arch>` |
| 3. 전역 PATH | `kosis` |

**설정/데이터 경로:**

| 항목 | 경로 | 비고 |
|------|------|------|
| 설정 | `~/.kosis/config.yaml` | API 키, AI 도구, 기본 포맷, 캐시 TTL |
| 즐겨찾기 | `~/.kosis/bookmarks.yaml` | |
| 이력 | `~/.kosis/history.yaml` | 최대 100개 |
| 캐시 | `~/.kosis/cache/` | 메타/검색만, TTL 설정 가능 |

> Windows: `~` → `%USERPROFILE%`, 한글 깨짐 시 `chcp 65001`  
> 환경변수 `KOSIS_API_KEY` 설정 시 config.yaml보다 우선 적용

---

## 핵심 규칙

- **meta 먼저** — 통계표마다 분류코드·항목코드·수록주기가 다름. `meta` 확인 필수
- **시점 지정 필수** — `data` 조회 시 `-l`, `-s`/`-e`, `--periods` 중 하나 반드시 지정. 미지정 시 에러 30
- **수록주기(-p) 정확히 지정** — `meta` 결과의 `prdSe=` 값을 그대로 사용
- **분류코드는 코드로** — `-c1 ~ -c8`, `-i` 에는 반드시 **코드**를 사용. 한글 이름 사용 시 에러 21
- **4만 셀 제한** — `ALL` 사용 시 초과 가능 → 자동 분할(기간 축) + 병렬 처리. `--concurrency N`으로 동시성 제어(키 1개여도 병렬). 단일 시점이 4만 셀 초과인 와이드 테이블은 **분류 축 2차 분할**로 자동 조회(느릴 수 있음 → 범위 축소 권장)
- **HTTPS 전용** — HTTP 미지원. 분당 1,000회 호출 제한

---

## 꼭 지켜야 할 사항

- **meta 먼저 확인**: 조회 전 반드시 `meta`로 분류/항목/주기 코드 확인
- **ALL 우선 사용**: 분류 코드 나열이 길면 명령어가 잘림. 전체 조회는 `ALL`, 하위 전체는 `"11*"`, 짧은 조합은 `"00+11"` 사용
- **AI의 가공 행위 명시**: 원본 데이터에 없는 요소(빈 줄 삽입, 정렬 변경 등)를 추가한 경우 표 위/아래에 반드시 명시
- **에러 즉시 보고**: 에러 발생 시 숨기지 말고 사용자에게 알림
- **데이터 원본 그대로**: 조회된 행/컬럼을 AI가 임의로 줄이거나 생략하지 않음 (`--fields` 명시 시만 예외)
- **markdown 출력 권장**: AI가 사용자에게 데이터를 보여줄 때 `-f md` 사용

## 꼭 하면 안 되는 사항

- **한글 이름 사용 금지**: `-c1`~`-c8`, `-i` 에 한글 이름 사용 금지 → 에러 21. 코드는 `meta`로 확인
- **긴 분류값 나열 금지**: 코드를 10개 이상 `+`로 나열하지 말 것 → 명령어 잘림. `ALL` 또는 `"접두사*"` 사용
- **존재하지 않는 시점 요청 금지**: 에러 30 발생 시 `-l 5`로 최근 데이터 먼저 확인
- **검색어에 연도 포함 금지**: `search`에 연도 포함 시 자동 제거 후 재검색됨
- **데이터 임의 축소 금지**: 조회 결과 행/컬럼을 AI가 임의로 줄이지 말 것. `--fields`를 임의로 추가 금지

---

## 명령어

| 명령어 | 별칭 | 예시 |
|--------|------|------|
| `search <키워드>` | `s` | `kosis s "인구" -n 50` |
| `meta <ORG> <TBL>` | `m` | `kosis m 101 DT_1IN1502` |
| `data <ORG> <TBL>` | `d` | `kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 5` |
| `list` | `ls` | `kosis ls --parent A_4` |
| `explain <ORG> <TBL>` | `ex` | `kosis ex 101 DT_1IN1502` |
| `bulk <userStatsId>` | | `kosis bulk "myid/..." -o data.xls` |
| `quick <요청>` | `q` | `kosis q "GDP 최근 5년" --ai claude` |
| `chart` | | `kosis d ... -f json \| kosis chart --type line` |
| `ind search <이름>` | `ind s` | `kosis ind s "GDP"` |
| `ind info <ID>` | | `kosis ind info 404` |
| `ind data <이름>` | `ind d` | `kosis ind d "GDP(명목)"` |
| `ind list <목록ID>` | `ind ls` | `kosis ind ls I01` (목록ID 필수) |
| `config set-key/add-key/show` | | `kosis config set-key <KEY>` |
| `bookmark add/ls/remove` | `bm` | `kosis bm add 101 DT_1IN1502 --name "인구"` |
| `history` | `hi` | `kosis hi replay 3` |
| (인자 없음) | | `kosis` → TUI 대시보드 |

**서비스뷰 코드** (`list --view` 플래그):

| 코드 | 서비스뷰 |
|------|----------|
| `MT_ZTITLE` | 주제별 (기본) |
| `MT_OTITLE` | 기관별 |
| `MT_RTITLE` | 국제통계 |
| `MT_BUKHAN` | 북한통계 |
| `MT_GTITLE01` | e-지방지표(주제별) |
| `MT_GTITLE02` | e-지방지표(지역별) |
| `MT_CHOSUN_TITLE` | 광복이전 |
| `MT_HANKUK_TITLE` | 대한민국통계연감 |
| `MT_STOP_TITLE` | 작성중지 |
| `MT_TM1_TITLE` | 대상별 |
| `MT_TM2_TITLE` | 이슈별 |
| `MT_ETITLE` | 영문 |

---

## data 플래그

```
kosis d <ORG_ID> <TBL_ID> -c1 <분류> -i <항목> -p <주기> [시점] [옵션]
```

| 구분 | 플래그 | 설명 | 예시 |
|------|--------|------|------|
| 분류 | `-c1`~`-c8` | 분류값 (`ALL`, `"00+11"`, `"11*"`) | `-c1 ALL` |
| 항목 | `-i` | 항목 코드 | `-i T100` |
| 주기 | `-p` | 수록주기 (아래 표 참조) | `-p M` |
| 시점 (택1) | `-s` + `-e` | 범위 | `-s 2020 -e 2024` |
| | `-l` | 최근 N개 | `-l 5` |
| | `--periods` | 비연속 | `--periods "2020,2022,2024"` |
| 출력 | `-f` | 형식 (table/json/csv/md) | `-f md` |
| | `-o` | 파일 (.xlsx/.db/.parquet/.csv/.json) | `-o gdp.xlsx` |
| | `--fields` | 필드 선택 (API키 또는 한글 라벨) | `--fields "시점,수치값"` |
| 기타 | `--user-id` | userStatsId 직접 조회 | |
| | `--no-auto-split` | 4만 셀 자동 분할 비활성화 | |
| | `--concurrency` | 동시 조회 워커 수 (0=자동: max(키,2)) | `--concurrency 4` |
| 차트 | `--chart` | 차트 타입 (line/bar/pie) | `--chart line` |
| | `--chart-format` | 포맷 (terminal/png/svg/pdf/html/excel/mermaid) | `--chart-format html` |
| | `--title` | 차트 제목 | `--title "인구추이"` |
| | `--subtitle` | 차트 부제목 | |
| | `--source` | 출처 표기 | |
| | `--note` | 주석 | |
| | `--template` | HTML 템플릿 이름 | `--template comparison` |
| | `--open` | 생성 후 자동 열기 | `--open` |

**시점 형식:** Y=`2024` M=`202401` Q=`20241` H=`20241` F=`2015`(5년)

**수록주기(-p) 코드 매핑:**

| meta 표시 | `-p` 값 | 비고 |
|-----------|---------|------|
| `prdSe=년` | `-p Y` 또는 `-p 년` | 연간 |
| `prdSe=월` | `-p M` 또는 `-p 월` | 월간 |
| `prdSe=분기` | `-p Q` 또는 `-p 분기` | 분기 |
| `prdSe=반기` | `-p H` 또는 `-p 반기` | 반기 |
| `prdSe=5년` | `-p F` 또는 `-p "5년"` | 5년 주기 (경제총조사 등) |

> ⚠ `prdSe=5년` 통계표에 `-p Y` 사용 시 에러 30 발생

---

## meta --type 옵션

```bash
kosis m <ORG> <TBL>              # 요약 (분류+항목+수록정보 통합)
kosis m <ORG> <TBL> --type ITM   # 분류/항목
kosis m <ORG> <TBL> --type PRD   # 수록정보
kosis m <ORG> <TBL> --type TBL   # 통계표명
kosis m <ORG> <TBL> --type ORG   # 기관명
kosis m <ORG> <TBL> --type CMMT  # 주석
kosis m <ORG> <TBL> --type UNIT  # 단위
kosis m <ORG> <TBL> --type SOURCE # 출처
kosis m <ORG> <TBL> --type WGT   # 가중치
kosis m <ORG> <TBL> --type NCD   # 갱신일
kosis m <ORG> <TBL> -f json      # JSON 출력 (요약 모드 포함)
```

---

## 차트 시각화

### data 명령에서 바로 차트 생성

```bash
kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 10 --chart line --title "전국 인구추이"
kosis d 101 DT_1IN1502 -c1 "11+21+26" -i T100 -p Y -l 5 --chart bar
kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 10 --chart line --chart-format png -o 인구.png
kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 10 --chart line --chart-format html -o 인구.html --open
kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 5 --chart line --chart-format mermaid
```

### kosis chart 독립 명령 (파이프/파일 입력)

```bash
# 파이프
kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 10 -f json | kosis chart
kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 10 -f json | kosis chart --format html -o 인구.html --open
kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 5 -f json | kosis chart --format mermaid

# 파일 입력
kosis chart -i data.json --type bar --format png -o chart.png
kosis chart -i data.json --format excel -o chart.xlsx
```

**chart 명령 플래그:**

| 플래그 | Short | 기본 | 설명 |
|--------|-------|------|------|
| `--input` | `-i` | stdin | JSON/CSV 입력 파일 |
| `--type` | `-t` | line | line / bar / pie |
| `--format` | | terminal | terminal/png/svg/pdf/html/excel/mermaid |
| `--output` | `-o` | | 출력 파일 |
| `--title` | | | 차트 제목 |
| `--subtitle` | | | 부제목 |
| `--source` | | | 출처 |
| `--note` | | | 주석 |
| `--template` | | | HTML 템플릿 이름 |
| `--width` | | 0 | 차트 너비 (px) |
| `--height` | | 0 | 차트 높이 (px) |
| `--open` | | false | 생성 후 자동 열기 |

**포맷별 용도:**

| 포맷 | 용도 |
|------|------|
| `terminal` (기본) | 터미널에서 바로 확인 |
| `png` / `svg` / `pdf` | 이미지/보고서 저장 |
| `html` | 인터랙티브 차트 (확대/축소/툴팁) |
| `excel` | 데이터+차트 함께 내보내기 |
| `mermaid` | Markdown 문서 삽입용 (GitHub/Notion 렌더링) |

### HTML 템플릿 (`--template` / `--chart-format template`)

| 템플릿 | 용도 |
|--------|------|
| `line-chart` | 시계열 추이 |
| `comparison` | 비교 분석 (차트+테이블+비중) |
| `bar-rank` | 항목별 순위 (가로 막대, 자동 정렬) |
| `bar-vertical` | 세로 막대 비교 (다중 시리즈 지원) |
| `stacked-bar` | 누적 막대 (구성비 분석) |
| `pie-share` | 구성비 (도넛 차트) |
| `area` | 영역 차트 (면적 강조) |
| `dual-axis` | 이중 Y축 (단위 다른 지표 비교) |
| `heatmap` | 히트맵 (지역×연도 색상 매핑) |
| `treemap` | 트리맵 (비중 계층 표시) |
| `dashboard` | 대시보드 (여러 차트 한 페이지) |

`--template` 미지정 시 자동 선택: 다중 시리즈→comparison, 단일→line-chart, bar→bar-rank, pie→pie-share

---

## config 서브명령

| 서브명령 | 설명 |
|----------|------|
| `set-key <KEY>` | 기존 키 모두 제거 후 새 키 하나만 설정 |
| `add-key <KEY>` | 키 추가 (중복 체크) |
| `remove-key <INDEX>` | 인덱스 기준 제거 |
| `key-list` | 등록 키 목록 (마스킹 표시) |
| `show` | 전체 설정 YAML 출력 |
| `set-ai <TOOL>` | 기본 AI 도구 설정 |
| `ai-add <NAME> <CMD>` | 커스텀 AI 도구 추가 (`{prompt}` 포함 필수) |
| `ai-remove <NAME>` | AI 도구 제거 |
| `ai-list` | AI 도구 목록 (설치 여부 포함) |
| `cache-clear` | 캐시 디렉토리 전체 삭제 |
| `cache-size` | 캐시 크기 확인 |
| `cache-clean` | TTL 초과 항목만 정리 |

---

## history / bookmark

```bash
# history
kosis hi                    # 최근 10개 이력
kosis hi --limit 20         # 최근 20개
kosis hi replay 3           # ID 3번 재실행
kosis hi clear              # 전체 이력 삭제 (확인 프롬프트)

# bookmark
kosis bm add 101 DT_1IN1502 --name "인구"
kosis bm ls
kosis bm remove "인구"       # 이름으로 제거
kosis bm remove 0           # 인덱스로 제거
```

---

## 검색 팁

- KOSIS 검색 API는 **통계표 이름(키워드)**만 검색 가능
- **숫자 코드로 검색 불가** → `kosis ls`로 탐색하거나 이름으로 검색
- **연도 포함 시 자동 제거** — 검색어에 연도(예: 2023)가 포함되면 자동으로 제거 후 재검색
- 검색 결과의 `STRT_PRD_DE` / `END_PRD_DE` 컬럼으로 수록 기간 확인

```bash
# ❌ 숫자 코드로 검색 불가
kosis s "11501"
# ✅ 이름으로 검색
kosis s "인구총조사" -n 10
# 연도 포함 → 자동 제거 후 재검색
kosis s "소비자물가 2023" -n 5
```

---

## 자주 쓰는 통계표 (검증됨)

```bash
# 인구총조사 (연간)
kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 5

# 미분양 현황 (연간, 분류코드 전체)
kosis d 116 DT_MLTM_2086 -c1 ALL -c2 ALL -i ALL -p Y -l 5

# 경제활동인구 (월간)
kosis d 101 DT_1DA7002S -c1 00 -i ALL -p M -l 6

# 국내총생산 - 검색 후 meta로 코드 확인
kosis s "국내총생산" -n 5
kosis m 101 DT_2OENA01
```

> ⚠ 통계표마다 분류코드 형식이 다릅니다. 반드시 `kosis m <ORG> <TBL>` 으로 코드를 먼저 확인하세요.

---

## 에러 코드

| 코드 | 의미 | 해결 방법 |
|------|------|----------|
| `20` | 필수 변수 누락 (objL) | 분류축 개수를 `meta`로 확인 → `-c1`~`-cN` 모두 채우기 |
| `21` | 잘못된 요청 변수 | 분류/항목 코드 확인 (`meta`로 재확인) |
| `30` | 데이터 없음 | 시점 확인, `-l 5`로 최근 데이터 먼저 조회 |
| `31` | 40,000셀 초과 | 자동 분할로도 **단일 시점이 4만 셀 초과**면 분류(`-c1`/`-c2`)·항목(`-i`) 범위를 좁혀라 ("전체×전체×다년도" 회피) |
| HTTP 429 | 분당 호출 제한 초과 | 자동 지수 백오프 재시도 (최대 3회). 잦으면 `config add-key`로 키 추가 |

---

## 워크플로우 (AI 표준)

```
1. kosis s "키워드"             → ORG_ID, TBL_ID 확인
2. kosis m <ORG> <TBL>          → 분류코드, 항목코드, 수록주기(prdSe) 확인
3. kosis d <ORG> <TBL> \        → 데이터 조회
     -c1 <분류코드> \
     -i <항목코드> \
     -p <prdSe값> \
     -l 5 \
     -f md                       → AI가 사용자에게 보여줄 때 md 형식
```

---

## 상세 문서

| 문서 | 내용 |
|------|------|
| [01-installation.md](references/01-installation.md) | 설치 및 초기 설정 |
| [02-data-paths.md](references/02-data-paths.md) | 두 가지 데이터 경로 (통계표/주요지표) |
| [03-statistics-query.md](references/03-statistics-query.md) | 통계표 조회 Step by Step |
| [04-indicators.md](references/04-indicators.md) | 주요지표 조회 (search/info/data/list) |
| [05-quick-query.md](references/05-quick-query.md) | 자연어 조회, 내장 사전, AI 모드 |
| [06-list-explain-bulk.md](references/06-list-explain-bulk.md) | 통계목록 탐색, 통계설명, 대용량 다운로드 |
| [07-output-formats.md](references/07-output-formats.md) | 출력 형식 (table/JSON/CSV/Markdown/Excel/SQLite/Parquet) |
| [08-utilities.md](references/08-utilities.md) | 편의 기능 (즐겨찾기, 이력, 캐시, AI 도구 설정) |
| [09-interactive-tui.md](references/09-interactive-tui.md) | 대화형 모드 및 TUI 대시보드 (키바인딩) |
| [10-auto-split.md](references/10-auto-split.md) | 자동 분할 조회 (4만 셀 초과 시 분할/병렬 처리) |
| [11-related-tables.md](references/11-related-tables.md) | 연관 통계표 찾기 |
| [12-examples.md](references/12-examples.md) | 자주 쓰는 통계표 및 실전 예시 |
| [13-reference.md](references/13-reference.md) | 레퍼런스 (API 매핑, 에러 코드, 트러블슈팅) |
| [14-chart.md](references/14-chart.md) | 차트 시각화 상세 |
| [15-ai-workflow.md](references/15-ai-workflow.md) | AI 표준 워크플로우 |
| [kosis-table-index.md](references/kosis-table-index.md) | 통계표 인덱스 — 5,128개 통계표 목록 (ORG_ID/TBL_ID/통계표명/분류경로, 주제별 29개 카테고리) |
| [kosis-table-params.md](references/kosis-table-params.md) | 통계표 파라미터 가이드 — 5,128개 통계표별 분류(-c1~c8 코드/명), 항목(-i 코드/명), 수록주기(-p) |
| [LEARNINGS.md](LEARNINGS.md) | **학습 로그(오답노트)** — 실패·함정·성공 쿼리 누적. 조회 전 읽고, 작업 후 추가하여 스킬이 진화한다 |
