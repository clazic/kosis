# 13. 레퍼런스

## 13.1 CLI 플래그 ↔ API 파라미터 매핑

| CLI 플래그 | API 파라미터 | 설명 | 예시 |
|-----------|------------|------|------|
| `-c1`~`-c8` | objL1~objL8 | 분류1~8 값 | `-c1 "00+11"` |
| `-i, --item` | itmId | 항목 코드 | `-i ALL` |
| `-p, --period` | prdSe | 수록주기 | `-p Y` (Y/M/Q/H) |
| `-s, --start` | startPrdDe | 시작 시점 | `-s 2015` |
| `-e, --end` | endPrdDe | 종료 시점 | `-e 2024` |
| `-l, --latest` | newEstPrdCnt | 최근 N개 시점 | `-l 5` |
| `--periods` | (클라이언트 처리) | 비연속 시점 | `--periods "2020,2022"` |
| `--fields` | (클라이언트 필터링) | 출력 필드 선택 | `--fields "C1,C1_NM,DT"` |
| `--with-code` | (클라이언트 처리) | 코드 컬럼(C1~C8, ITM_ID) 함께 출력 | `--with-code` |
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

> ✅ **v0.5.0+**: `-f json`·`--with-code`·`--fields` 사용 시 분류·항목 코드(`C1~C8`, `ITM_ID`)가 자동 출력된다. JSON은 `"<분류축명> 코드"` 키로 이름 옆에 함께 나온다. table/csv 기본은 이름만 → 코드가 필요하면 `--with-code`.
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
| 설정 파일 | `~/.kosis/config.yaml` | API 키, AI 도구, 기본 포맷 |
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
- **분당 호출 제한**: 분당 1,000번 이내, 초과 시 429 에러 (자동 재시도)
- **캐시 정책**: 메타/검색 결과만 캐시, 데이터는 항상 실시간 조회
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
| `kosis explain <ORG> <TBL>` | `ex` | 통계 조사 설명 |
| `kosis bulk <userStatsId>` | - | 대용량 다운로드 (SDMX/XLS) |
| `kosis quick <요청>` | `q` | 자연어 원스텝 조회 |
| `kosis indicator search` | `ind s` | 주요지표 검색 |
| `kosis indicator info <ID>` | `ind info` | 지표 상세 정보 |
| `kosis indicator data <이름>` | `ind d` | 지표 수치 조회 |
| `kosis indicator list` | `ind ls` | 지표 목록 탐색 |
| `kosis chart` | - | 차트 시각화 (terminal/png/svg/pdf/html/excel/mermaid) |
| `kosis config set-key` | - | API 키 설정 |
| `kosis config show` | - | 전체 설정 표시 |
| `kosis bookmark add` | `bm add` | 즐겨찾기 추가 |
| `kosis history` | `hi` | 조회 이력 |
| `kosis` (인자 없음) | - | TUI 대시보드 |

---

## 13.6 도움말

모든 명령어에 `--help`를 붙이면 상세 사용법, 파라미터, 예제를 볼 수 있습니다.

```bash
kosis --help              # 전체 명령어 목록
kosis data --help         # data 명령어 상세
kosis ind --help          # 주요지표 명령어 상세
kosis q --help            # quick 명령어 상세
kosis config --help       # 설정 명령어 상세
kosis bulk --help         # 대용량 다운로드 상세
kosis bm --help           # 즐겨찾기 상세
kosis hi --help           # 이력 상세
```
