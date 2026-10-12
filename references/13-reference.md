# 13. 레퍼런스

## 13.1 CLI 플래그 ↔ API 파라미터 매핑

| CLI 플래그 | API 파라미터 | 설명 | 예시 |
|-----------|------------|------|------|
| `-c1`~`-c8` | objL1~objL8 | 분류1~8 값 | `-c1 "00+11"` |
| `-i, --item` | itmId | 항목 코드 | `-i ALL` |
| `-p, --period` | prdSe | 수록주기 | `-p Y` (Y/M/Q/H·S/F — F 는 2년 이상 여러 해, 메타 이름 `3년` 등도 됨) |
| `-s, --start` | startPrdDe | 시작 시점 (분기 YYYYQQ·반기 YYYYHH, 5자리도 6자리로 바꿔 보냄) | `-s 2015`, `-s 202401` |
| `-e, --end` | endPrdDe | 종료 시점 | `-e 2024` |
| `-l, --latest` | newEstPrdCnt | 최근 N개 시점 | `-l 5` |
| `--periods` | (클라이언트 처리) | 비연속 시점 | `--periods "2020,2022"` |
| `--fields` | (클라이언트 필터링) | 출력 필드 선택 | `--fields "C1,C1_NM,DT"` |
| `--with-code` | (클라이언트 처리) | 코드 컬럼(C1~C8, ITM_ID) 함께 출력 | `--with-code` |
| `s --org` | orgId | 표를 만든 기관 — 서버에서 거름. 이름은 기관별 목록(`ls --view MT_OTITLE`)에서 코드로 바꿈(맞는 기관 5개 넘으면 오류) | `--org 101`, `--org 울산` |
| `s --page` / `--sort` | startCount / sort | 쪽 번호(1부터) / RANK(정확도)·DATE(최신 — 검색어 관련도가 거의 반영되지 않아 엉뚱한 표가 앞에 올 수 있다) | `--page 2 --sort DATE` |
| `s --stat` / `--region` | (클라이언트 필터링) | 받은 쪽 안에서만 거름 — 빈 결과면 `--page` 로 다음 쪽 | `--stat 광업제조업조사` |
| `s <검색어…>` | searchNm | 따옴표 없이 여러 단어를 줘도 한 검색어로 합친다 (`kosis s 1인당 지역내총생산`) | |
| `m --find` / `--under` / `--level` / `--axis` | type=ITM (1회) | 코드 고르기. `--axis item`·`class2`·분류 이름 일부. `--type` 없이 또는 `--type ITM` 과만 (다른 type 과 쓰면 exit 1). 축 번호는 OBJ_ID_SN. 맞는 코드가 없으면 exit 1. `--find` 는 공백·`~`/`-`·`ㆍ`/`·` 차이 무시 | `--axis item`, `--find 출하액`, `--find 15~29` |
| `m --type ITM --obj` | objId | ITM 원본에서 한 분류(OBJ_ID) 또는 ITEM 만 | `--type ITM --obj ITEM` |
| `rules list --id` / `--topic` / `--status` | (로컬 처리) | 규칙 id·표 ID 정확히 / topic / wrong·ok | `--id 101/DT_1FS1101` |
| `--pivot` | (클라이언트 처리) | 한 축의 값을 열로 펼친 비교표 (C1~C8, ITM, PRD, 분류 이름). ITM 이면 열 이름 `항목명(단위)` — 항목마다 단위가 달라도 한 행. parquet 저장과 함께 쓰면 exit 1 | `--pivot C1` |
| `d --sort` / `--top` | (클라이언트 처리) | 수치값으로 정렬 desc·asc(숫자 아닌 값은 맨 뒤, 같은 값은 원래 순서), `--top N` 은 정렬 뒤 앞 N행(경계 동률은 함께, stderr 안내). `--top` 만 쓰거나 `--pivot` 과 함께 쓰면 exit 1. 합계 행도 함께 정렬된다 | `--sort desc --top 3` |
| `d --no-total` | (클라이언트 처리) | 여러 코드를 준 축(ALL·*·+)에서 이름이 전국·계·합계·총계·소계·전체인 행을 뺀다. 고정 축은 안 봄. 이름이 다른 합계(예 '광업 및 제조업')는 못 잡음 | `-c2 ALL --no-total --sort desc --top 3` |
| `d --change` | (클라이언트 처리) | 같은 계열(분류 코드·항목) 안에서 `증감`·`증감률(%)` 열 추가 (`--api-keys` 면 DT_CHG·DT_CHG_PCT). yoy=연도만 1 뺀 시점(202608→202508), prev=직전 시점. 비교 시점도 받아야 함(월 yoy `-l 13`). 정렬·`--top` 전에 계산. `--pivot` 과 함께 쓰면 exit 1 | `--change yoy -l 13` |
| `d -cN level:N` | (조회 전) | 그 축 N단계 코드 전부로 바꿔 조회 (`m --level N` 의 `-c1` 줄과 같음). `00+level:2` 처럼 섞어도 되고 `!코드` 로 뺀다(`level:1+!03`). 바뀐 코드 이름은 stderr 로. 없는 단계면 exit 1 + 있는 단계 목록 (v0.5.27+) | `-c1 level:1 --no-total --sort desc --top 3` |
| `d` stderr 안내 | (클라이언트 처리) | 조회한 코드 이름, 결과에 없는 코드(+ 값이 하위에만 있으면 아래 코드), 일찍 끊긴 코드, 빈 마지막 시점, 부분 연도, 미래 시점(추계·전망), 값이 모두 0 인 항목, 주석 '잠정', 빈·이은 단위 칸. 표에 없는 축 번호(-c6 등)는 조회 전 exit 1. 같은 `-cN`·`-i` 를 거듭 주면 `+` 로 합침 (v0.5.27+) | `-c1 11 -c1 26` = `-c1 "11+26"` |
| `d --sort-by` | (클라이언트 처리) | `--sort` 기준: value(수치값, 기본)·change(증감)·pct(증감률)·ratio(비율). `--change` 와 함께면 기본이 증감이고, 비교 기준으로만 받은 앞 시점 행(증감 빈 칸)은 빠진다 (v0.5.27+). `--sort` 는 항목·시점 묶음마다 따로 정렬·`--top` 하고 `순위` 열(API 키 RANK, 동률 같은 순위)을 붙인다 | `-l 13 --change yoy --sort desc --top 3` |
| `ratio` | (클라이언트 처리, d 두 번) | 다른 두 표의 값을 이름·시점으로 맞춰 나눔. `--num`·`--den` 에 d 인자 그대로, `--scale`(100=%, 1=배수, 1000=천명당), `--sort desc|asc --top N`. 시도 이름이 표마다 달라도(강원도/강원특별자치도) 줄임말로 맞춤. 맞지 않은 행은 양쪽 모두 stderr. 이름이 겹치거나(여러 시도의 '중구') 항목이 둘 이상이면 exit 1. `--no-total` 로 전국 행 빼기. `--pivot`·`--fields`·`-o` 등은 --num/--den 에 못 넣음. 각 쪽 d 의 안내는 `[분자]`/`[분모]` 를 붙여 stderr 로 (v0.5.27+) | `kosis ratio --num "101 DT_1AG25203 -c1 … -c2 000 -i T00 -p F -l 1" --den "101 DT_1IN1502 -c1 … -i T100 -p Y -l 1" --sort desc --top 3` |
| `d --ratio` | (클라이언트 처리) | 같은 축 두 코드의 비율 `비율(%)` 열 (분자/분모 ×100, `--api-keys` 면 DT_RATIO_PCT). 축은 결과에서 찾고, 다른 축·시점이 같은 행끼리 나눈다. 분모 코드도 조회에 넣어야 한다. 분자 `ALL` 이면 그 축 모든 행을 분모 대비 %로. 분자 여러 코드(`Y70+Y75`)면 코드별 % 를 채우고 합계 % 는 stderr 로(구간을 더한 비율, 예 65세 이상). 고른 축은 stderr 에 알림. parquet 저장과 함께 쓰면 exit 1 | `-i T1+T2 --ratio T2/T1`, `-c1 ALL --ratio ALL/<합계코드>`, `--ratio Y70+Y75+Y80/000` |
| `d -p` 검사 | (조회 전) | 표에 없는 주기를 주면 exit 1 + 있는 주기 목록 (KOSIS 는 월간 표에 Y 를 주면 월별 행을 오류 없이 돌려준다) | |
| (열 이름) | LST_CHN_DE | v0.5.25 부터 `최종수정일` (예전 `비고`). `--fields 비고` 는 계속 받는다 | `--fields 시점,수치값,최종수정일` |
| `--api-keys` | (클라이언트 처리) | 열 이름을 API 키로 (표마다 같은 키: C1, C1_NM … ITM_NM, PRD_DE, DT, UNIT_NM). table·json·csv·md·xlsx·db 모두 같은 이름. `--fields`(한국어·API 키)·`--pivot`(C1·분류 이름)과 함께 써도 된다 | `--api-keys -f json` |
| `--user-id` | userStatsId | 사전 등록 통계표 | `--user-id "myid/..."` |

### 13.1.1 출력 필드 키 (코드 키 ↔ 이름 키)

KOSIS API는 레코드마다 **코드 키**와 **이름 키**를 함께 반환한다. 지도 결합·DB join 등 기계 처리에는 반드시 **코드 키**(`C1`, `ITM_ID`)를 사용하라 — 이름 키는 중복(예: '중구'가 여러 시도) 때문에 join 키로 부적합하다.

| 코드 키 | 이름 키 | 의미 |
|---------|---------|------|
| `C1` ~ `C8` | `C1_NM` ~ `C8_NM` | 분류 코드 / 이름 (행정구역 코드 등) |
| `ITM_ID` | `ITM_NM` | 항목 코드 / 이름 (예: T04=출하액) |
| `ORG_ID`, `TBL_ID` | — | 출처(기관·통계표) 추적 |
| — | `DT` | 수치값 |
| — | `PRD_SE` / `PRD_DE` / `UNIT_NM` | 수록주기 / 시점 / 단위 |

> 분류 축이 N개인 표는 `--class1`~`--classN`이 모두 필요하다. 빠지면 조회 전에 빠진 플래그와 축 이름을 알려 주고 종료 코드 1 (API의 `[20] objL` 대신).
> 파이프·파일로 나가는 table 출력은 값을 자르지 않는다 (터미널에서만 화면 폭에 맞춰 `..`로 줄임).

> ✅ **v0.5.0+**: `-f json`·`--with-code`·`--fields` 사용 시 분류·항목 코드(`C1~C8`, `ITM_ID`)가 자동 출력된다. JSON은 `"<분류축명> 코드"` 키로 이름 옆에 함께 나온다. table/csv 기본은 이름만 → 코드가 필요하면 `--with-code`. csv 의 UTF-8 BOM(Excel 용)은 `-o` 파일에만 붙고 stdout 에는 없다 (v0.5.27+).
> ⚠ **구버전(< v0.5.0) 함정**: `-f json` / `--fields "C1"`을 줘도 코드가 `null`로 나온다. 이 경우 `meta -f json` 역매핑(`references/16-geo-join.md` §16.4 폴백)을 사용하라.

---

## 13.2 에러 코드 및 트러블슈팅

### KOSIS API 에러 코드

| 에러 코드 | 메시지 | 원인 | 해결 방법 |
|----------|--------|------|----------|
| 10 | 인증키 오류 | API 키가 잘못됨 | `kosis config set-key`로 올바른 키 설정 |
| 11 | 호출 빈도 초과 | 분당 1,000회 초과 | 잠시 대기 후 재시도 |
| 12 | 허용되지 않은 IP | IP 제한 걸림 | KOSIS 웹에서 IP 등록 확인 |
| 20 | 필수 변수 누락 | 분류(-c1 등) 파라미터 누락 | `-c1`, `-i`, `-p` 등 필수 파라미터 확인 |
| 21 | 잘못된 요청 변수 | 파라미터 오류 | `meta`로 올바른 코드 확인 |
| 22 | DB 오류 | 서버 문제 | 잠시 후 재시도 |
| 30 | 데이터 미존재 | 해당 조건에 데이터 없음 | 시점/분류/항목 값 재확인 |
| 31 | 4만 셀 초과 | 요청 범위 과다 | 범위 축소 또는 자동 분할 사용 |
| 32 | 통계표 미제공 | API 미제공 통계표 | 다른 통계표 사용 |
| 40 | 1분간 호출가능건수(200건) 초과 | **키 하나당** 1분 200건 (2026-10-12 실측) | 1분 쉬고 재시도, 반복 작업은 키를 늘린다(`config add-key`) |

### 자주 겪는 문제

**Q: `API 오류 [30]` 발생**
```bash
# 원인 1: 시점 미지정
kosis d 101 DT_1F160622 -i T01 -p Y -c1 00 -c2 C          # ✗
kosis d 101 DT_1F160622 -i T01 -p Y -c1 00 -c2 C -l 5     # ✓

# 원인 2: 잘못된 분류/항목 코드
kosis m 101 DT_1F160622    # meta로 올바른 코드 확인
```

**Q: `API 오류 [21]` 발생**
```bash
# 원인: 분류/항목/주기 코드가 해당 통계표와 맞지 않음
kosis m <ORG_ID> <TBL_ID>  # meta로 올바른 코드 확인
```

**Q: 데이터가 일부만 나옴**
```bash
# 원인: 통계표가 특정 연도에만 데이터 보유 (통계 개편 등)
kosis m <ORG_ID> <TBL_ID> --type PRD    # 수록기간 확인
```

**Q: 대용량 조회가 느림**
```bash
# API 키를 추가하면 병렬 조회로 속도 향상
kosis config add-key <API_KEY_2>
kosis config add-key <API_KEY_3>
```

---

## 13.3 파일 저장 경로

| 항목 | 경로 | 설명 |
|------|------|------|
| 설정 파일 | `~/.kosis/config.yaml` | API 키, 기본 포맷, 캐시 |
| 즐겨찾기 | `~/.kosis/bookmarks.yaml` | 저장한 통계표 목록 |
| 조회 이력 | `~/.kosis/history.yaml` | 최근 100개 조회 기록 |
| 캐시 | `~/.kosis/cache/` | 메타/검색 캐시 (TTL: 24시간) |

> Windows에서는 `~` 대신 `%USERPROFILE%` 사용 (예: `C:\Users\사용자\.kosis\`)

---

## 13.4 주의사항

- **시점 필수**: `data` 조회 시 `--start`/`--end`, `--latest`, `--periods` 중 하나를 반드시 지정
- **meta 필수**: 통계표 코드를 모르면 데이터 조회 불가, `meta`로 먼저 확인
- **4만 셀 제한**: `ALL` 사용 시 초과 가능 → 자동 분할 처리 (API 키 추가 시 병렬)
- **HTTPS 전용**: HTTP 미지원 (2026.02.05부터 HTTP 종료)
- **분당 호출 제한**: 키 하나당 1분 200건 — 넘으면 `[40]` (2026-10-12 실측. 예전 "분당 1,000번"은 틀린 설명). HTTP 429 는 자동 재시도
- **캐시 정책**: 검색·메타·목록·설명만 캐시(기본 24시간, `cache_ttl_hours`), 데이터(`d`)는 항상 실시간. 새 메타가 필요하면 `kosis config cache-clear`
- **종료 코드·출력**: 인자·형식 오류와 조회 실패는 stderr 에 오류, 종료 코드 1 (`s`·`m`·`d`·`ex`·`ls`·`rules list`). stdout 은 데이터만 — 안내·경고(검색 범위, 빠진 항목)는 stderr
- **빠진 항목 경고**: `d` 는 요청한 항목 중 결과가 한 행도 없는 항목을 stderr 에 알린다 (항목마다 마지막 시점이 다를 수 있음 — rules `period.item-latest-differs`)
- **Windows 경로**: 파일 저장 시 경로 구분자 주의 (`/` 대신 `\`)
- **인코딩**: 출력 데이터는 UTF-8, Windows CMD에서 한글 깨짐 시 `chcp 65001` 실행

---

## 13.5 명령어 전체 목록 (빠른 참조)

| 명령어 | 별칭 | 설명 |
|--------|------|------|
| `kosis search <키워드>` | `s` | 통계표 검색 |
| `kosis meta <ORG> <TBL>` | `m` | 메타데이터 (분류/항목/수록정보) |
| `kosis data <ORG> <TBL>` | `d` | 데이터 조회 |
| `kosis list` | `ls` | 통계목록 트리 탐색 |
| `kosis explain <ORG> <TBL>` | `ex` | 통계 조사 설명·주석·원표/조사설명 링크 (`-f json`) |
| `kosis rules list [찾을 말]` | - | AI 에이전트용 배포 규칙(references/rules.yaml)·로컬 규칙(~/.kosis/rules.d) 목록 (`--kind`, `--id`, `--topic`, `--status`, `-f json`). 같은 id 가 로컬·배포에 있으면 둘 다(로컬 먼저) |
| `kosis rules add [파일]` | - | 규칙 한 항목(YAML, 파일 생략 시 표준입력)을 ~/.kosis/rules.d 에 저장. 형식 오류면 종료 코드 1, 비슷한 규칙이 있으면 알림 |
| `kosis rules sync` | `--init <URL>` | ~/.kosis/rules.d 를 비공개 git 저장소와 합치기 (commit → fetch·merge → push). 처음 한 번 `--init`. 같은 파일 충돌 시 종료 코드 1 |
| `kosis rules log [찾을 말]` | `-n`, `-f json` | rules.d 변경 이력 (최근 순, 맨 위 미동기화 변경) |
| `kosis rules revert <파일>` | `--to <커밋>` | 규칙 파일 하나를 이전 내용으로 (sync 전 변경 → 마지막 sync 상태, 아니면 직전 버전, 지운 파일은 살림) |
| `kosis rules validate` | - | 규칙 형식 점검 (문제 있으면 종료 코드 1) |
| `kosis bulk <userStatsId>` | - | 대용량 다운로드 (SDMX/XLS) |
| `kosis indicator search` | `ind s` | 주요지표 검색 |
| `kosis indicator info <ID>` | `ind info` | 지표 상세 정보 |
| `kosis indicator data <이름>` | `ind d` | 지표 수치 조회 |
| `kosis indicator list` | `ind ls` | 지표 목록 탐색 |
| `kosis chart` | - | 차트 시각화 (terminal/png/svg/pdf/html/excel/mermaid) |
| `kosis config set-key` | - | API 키 설정 |
| `kosis config show` | - | 전체 설정 표시 |
| `kosis bookmark add` | `bm add` | 즐겨찾기 추가 |
| `kosis history` | `hi` | 조회 이력 |
| `kosis update` | - | 최신 버전으로 self-update (바이너리+스킬, `--check`/`--force`) |
| `kosis` (인자 없음) | - | TUI 대시보드 |

---

## 13.6 도움말

모든 명령어에 `--help`를 붙이면 상세 사용법, 파라미터, 예제를 볼 수 있습니다.

```bash
kosis --help              # 전체 명령어 목록
kosis data --help         # data 명령어 상세
kosis ind --help          # 주요지표 명령어 상세
kosis config --help       # 설정 명령어 상세
kosis bulk --help         # 대용량 다운로드 상세
kosis bm --help           # 즐겨찾기 상세
kosis hi --help           # 이력 상세
```
