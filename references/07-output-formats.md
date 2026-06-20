# 7. 출력 형식

## 7.1 포맷별 사용법

| 형식 | 플래그/확장자 | 용도 | 비고 |
|------|-------------|------|------|
| 컬러 테이블 | `-f table` (기본) | 터미널 확인 | TTY 감지하여 자동 컬러 |
| JSON | `-f json` | AI/jq 파이프라인 | TTY면 pretty, 파이프면 compact |
| CSV | `-f csv` | 데이터 분석 | UTF-8, 쉼표 구분 |
| Markdown | `-f md` 또는 `-f markdown` | AI 출력, 문서 삽입 | GitHub Flavored Markdown 테이블 |
| Excel | `-o *.xlsx` | 보고서 | 시트명: 통계표명, 자동 열 너비 |
| SQLite | `-o *.db` | 대용량 SQL 분석 | 시점/지역 인덱스 자동 생성 |
| Parquet | `-o *.parquet` | Pandas, DuckDB | 컬럼형 저장, 압축 |

> `-f` 플래그는 stdout 출력 형식, `-o` 플래그는 파일 저장 (확장자로 형식 자동 감지)

## 7.2 파일 저장 예시

```bash
kosis d 101 DT_1IN1502 -c1 ALL -i ALL -p Y -s 2015 -e 2024 -o 인구.db
kosis d 301 DT_200Y101 -c1 ALL -i ALL -p Y -s 2020 -e 2024 -o gdp.xlsx
kosis d 101 DT_1DA7002S -c1 00 -i ALL -p M -l 12 -f json | jq '.[].DT'

# Markdown 테이블 (AI가 사용자에게 보여줄 때 권장)
kosis d 101 DT_1IN1502 -c1 "00+11+26" -i T100 -p Y -l 3 -f md
```

## 7.3 AI 에이전트 출력 규칙

- AI가 사용자에게 데이터를 보여줄 때 `-f md` (Markdown) 사용 권장
- **데이터 축소 금지**: API가 반환한 행/컬럼을 임의로 줄이거나 생략하지 말 것
- `--fields`를 AI가 임의로 추가하여 컬럼을 제한하지 말 것 (사용자 요청 시만)
- 데이터가 너무 많으면 전체를 보여주되, 사용자에게 "N건 중 일부를 표시합니다" 안내
