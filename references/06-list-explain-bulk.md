# 6. 통계목록 탐색, 통계설명, 대용량 다운로드

## 6.1 통계목록 탐색 — `list` (별칭: `ls`)

트리 구조로 통계표를 탐색합니다.

```bash
kosis ls                              # 주제별 최상위
kosis ls --view MT_OTITLE             # 기관별
kosis ls --parent A_4                 # 인구총조사 하위
kosis ls --parent A11                 # 인구부문 → 연도별 표 목록
kosis ls -f json                      # JSON 출력
```

| 서비스뷰 코드 | 설명 |
|--------------|------|
| MT_ZTITLE | 주제별 (기본) |
| MT_OTITLE | 기관별 |
| MT_RTITLE | 국제통계 |
| MT_BUKHAN | 북한통계 |
| MT_GTITLE01 | e지방지표(주제) |
| MT_GTITLE02 | e지방지표(지역) |
| MT_TM1_TITLE | 대상별 |
| MT_TM2_TITLE | 이슈별 |
| MT_CHOSUN_TITLE | 광복이전 |
| MT_HANKUK_TITLE | 통계연감 |
| MT_STOP_TITLE | 작성중지 |
| MT_ETITLE | 영문 |

---

## 6.2 통계설명 — `explain` (별칭: `ex`)

조사 방법론, 목적, 대상 등을 확인합니다.

```bash
kosis ex 101 DT_1IN1502           # 직접 지정
kosis explain                      # 대화형: 검색 후 선택
```

---

## 6.3 대용량 다운로드 — `bulk`

KOSIS 웹에서 사전 등록한 통계표를 SDMX/XLS로 다운로드합니다.

```bash
kosis bulk "myid/101/DT_1IN1502/..." --type DSD -o data.sdmx
kosis bulk "myid/101/DT_1IN1502/..." -o data.xls
```

| 플래그 | 설명 | 기본값 |
|--------|------|--------|
| `--type` | SDMX 유형 (DSD, Generic 등) | DSD |
| `-o, --output` | 출력 파일 (필수) | - |

| 형식 | 셀 제한 |
|------|---------|
| SDMX | 40,000셀 |
| XLS | 200,000셀 |

> `userStatsId`는 https://kosis.kr/openapi/ 에서 사전 등록 필요
