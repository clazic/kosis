# 3. 통계표 조회 (Step by Step)

## Step 1: 검색 — `search` (별칭: `s`)

```bash
kosis search "인구"              # 기본 20개 결과
kosis s "미분양" -n 50           # 결과 50개
kosis s "GDP" -f json            # JSON 형식 출력
kosis search                     # 대화형: 검색어 입력 대기
```

| 플래그 | 설명 | 기본값 |
|--------|------|--------|
| `-n, --limit` | 결과 수 | 20 |
| `-f, --format` | table, json | table |

출력 필드: `ORG_ID`, `ORG_NM`, `TBL_ID`, `TBL_NM`, `STRT_PRD_DE`, `END_PRD_DE`

## Step 2: 메타 확인 — `meta` (별칭: `m`)

```bash
kosis meta 101 DT_1IN1502           # 요약 (분류/항목/수록정보)
kosis m 101 DT_1IN1502 --type ITM   # 분류/항목 상세
kosis m 101 DT_1IN1502 --type PRD   # 수록기간 상세
kosis meta                           # 대화형: 검색 후 선택
```

| 플래그 | 설명 | 기본값 |
|--------|------|--------|
| `--type` | ITM, PRD, TBL, ORG, CMMT, UNIT, SOURCE, WGT, NCD | (요약 모드) |
| `-f, --format` | table, json | table |

**출력 해석:**
- `[분류]` → `--class1`~`--class8` 파라미터에 쓸 코드 목록
- `[항목]` → `--item` 파라미터에 쓸 코드 목록
- `[수록정보]` → `--period` 파라미터 코드 (Y/M/Q/H)

## Step 3: 데이터 조회 — `data` (별칭: `d`)

```bash
# 기본 형태
kosis d <ORG_ID> <TBL_ID> -c1 <분류값> -i <항목코드> -p <주기> [시점옵션]

# 연속 범위
kosis d 101 DT_1IN1502 -c1 "00" -i T100 -p Y -s 2015 -e 2024

# 최근 N개
kosis d 116 DT_MLTM_2086 -c1 ALL -c2 ALL -i ALL -p Y -l 5

# 비연속 시점 (원하는 시점만)
kosis d 101 DT_1IN1502 -c1 "00" -i T100 -p Y --periods "2015,2020,2025"

# 대화형
kosis data
```

### 필수 플래그

| 플래그 | 설명 | 예시 |
|--------|------|------|
| `-c1, --class1` | 분류1 값 | `00`, `ALL`, `"00+11+21"`, `"11*"` |
| `-i, --item` | 항목 코드 | `T01`, `ALL` |
| `-p, --period` | 수록주기 | `Y`(연), `M`(월), `Q`(분기), `H`(반기) |

### 시점 플래그 (택1, 필수)

| 플래그 | 설명 | 예시 |
|--------|------|------|
| `-s, --start` + `-e, --end` | 범위 지정 | `-s 2015 -e 2024` |
| `-l, --latest` | 최근 N개 | `-l 5` |
| `--periods` | 비연속 시점 | `--periods "2020,2022,2025"` |

> **시점 미지정 시 `API 오류 [30]` 발생**. 반드시 시점을 지정하세요.

### 선택 플래그

| 플래그 | 설명 | 기본값 |
|--------|------|--------|
| `-c2`~`-c8` | 분류2~8 값 | (미사용) |
| `-f, --format` | table, json, csv | table |
| `-o, --output` | 파일 저장 (.csv/.xlsx/.json/.db/.parquet) | (stdout) |
| `--fields` | 출력 필드 선택 (API 키 또는 한글 라벨) | (전체) |
| `--user-id` | 자료등록 방식 조회 | (미사용) |
| `--no-auto-split` | 자동 분할 비활성화 | false |

### 시점 형식

| 주기 | 코드 | 시점 형식 | 예시 |
|------|------|----------|------|
| 연간 | `Y` | `YYYY` | `2024` |
| 월간 | `M` | `YYYYMM` | `202401` |
| 분기 | `Q` | `YYYYQ` | `20241` (1분기) |
| 반기 | `H` | `YYYYH` | `20241` (상반기) |

```bash
# 연간 2020~2024
kosis d ... -p Y -s 2020 -e 2024

# 월간 2024년 1월~12월
kosis d ... -p M -s 202401 -e 202412

# 분기 2023년 1분기~2024년 4분기
kosis d ... -p Q -s 20231 -e 20244
```

### 분류값 규칙

| 상황 | 문법 | 예시 |
|------|------|------|
| 전체 선택 | `ALL` | `-c1 ALL` |
| 복수 선택 | `+` 구분 | `-c1 "00+11+21"` |
| 하위 전체 | `*` 접미사 | `-c1 "11*"` |

### --fields 사용법

API 키와 한글 라벨 모두 사용 가능합니다.

```bash
# API 키로 지정
kosis d 101 DT_1F160622 -c1 00 -c2 C -i T01 -p Y -l 3 --fields "C1_NM,C2_NM,DT"

# 한글 라벨로 지정 (meta에서 확인한 이름)
kosis d 101 DT_1F160622 -c1 00 -c2 C -i T01 -p Y -l 3 --fields "시도별,산업별,수치값"
```

API 키 ↔ 한글 기본 매핑:

| API 키 | 한글명 |
|--------|--------|
| C1_NM ~ C8_NM | (메타에 따라 동적: 시도별, 산업별 등) |
| ITM_NM | 항목 |
| PRD_SE | 수록주기 |
| PRD_DE | 시점 |
| DT | 수치값 |
| UNIT_NM | 단위 |
| LST_CHN_DE | 비고 |
