# 12. 자주 쓰는 통계표 & 실전 예시

## 12.1 자주 쓰는 통계표

| 통계표 | ORG_ID | TBL_ID | 주기 | 빠른 조회 예시 |
|--------|--------|--------|------|--------------|
| 인구(읍면동/5세별) | 101 | DT_1IN1502 | Y | `kosis d 101 DT_1IN1502 -c1 00 -i T100 -p Y -l 5` |
| 주민등록인구 | 101 | DT_1YL20651E | M | `kosis d 101 DT_1YL20651E -c1 00 -i ALL -p M -l 6` |
| 주택 미분양 | 116 | DT_MLTM_2086 | Y | `kosis d 116 DT_MLTM_2086 -c1 ALL -c2 ALL -i ALL -p Y -l 5` |
| 경제활동인구 | 101 | DT_1DA7002S | M | `kosis d 101 DT_1DA7002S -c1 00 -i ALL -p M -l 6` |
| GDP | 301 | DT_200Y101 | Y | `kosis d 301 DT_200Y101 -c1 ALL -i ALL -p Y -l 3` |

> 위 예시의 분류/항목 코드는 변경될 수 있으므로, `kosis meta`로 최신 코드를 확인하세요.

---

## 12.2 실전 예시

### 12.2.1 기본 워크플로우: 전국 미분양 (최근 5년)

```bash
# 1단계: 검색
kosis s "미분양"
# → ORG_ID=116, TBL_ID=DT_MLTM_2086

# 2단계: 메타 확인
kosis m 116 DT_MLTM_2086
# → 분류: 대분류(부문별/시도별/규모별), 구분(전국/서울특별시/…), 주기: Y(년)

# 3단계: 데이터 조회
kosis d 116 DT_MLTM_2086 -c1 ALL -c2 ALL -i ALL -p Y -l 5
```

### 12.2.2 파일 저장

```bash
# Excel
kosis d 301 DT_200Y101 -c1 ALL -i ALL -p Y -s 2020 -e 2024 -o gdp.xlsx

# CSV
kosis d 101 DT_1DA7002S -c1 00 -i ALL -p M -l 12 -o 경활.csv

# SQLite (대용량)
kosis d 101 DT_1IN1502 -c1 ALL -i ALL -p Y -s 2015 -e 2024 -o 인구.db

# Parquet (데이터 분석용)
kosis d 101 DT_1DA7002S -c1 ALL -i ALL -p M -s 202401 -e 202412 -o 경활.parquet
```

### 12.2.3 JSON 파이프라인

```bash
# 수치값만 추출
kosis d 101 DT_1DA7002S -c1 00 -i ALL -p M -l 12 -f json | jq '.[].DT'

# 특정 조건 필터
kosis d 101 DT_1IN1502 -c1 ALL -i T100 -p Y -l 1 -f json | jq '[.[] | select(.수치값 | tonumber > 1000000)]'

# 시점과 수치만 추출
kosis d 301 DT_200Y101 -c1 ALL -i ALL -p Y -l 5 -f json | jq '.[] | {시점, 수치값}'
```

### 12.2.4 복수 지역/항목 비교

```bash
# 서울+부산+대구 인구 비교
kosis d 101 DT_1IN1502 -c1 "00+11+21+22" -i T100 -p Y -l 5

# 전국 시도별 전체
kosis d 116 DT_MLTM_2086 -c1 ALL -c2 ALL -i ALL -p Y -l 3

# 서울 하위 구군 전체
kosis d 101 DT_1IN1502 -c1 "11*" -i T100 -p Y -l 1
```

### 12.2.5 비연속 시점 비교

```bash
# 5년 단위 인구 변화
kosis d 101 DT_1IN1502 -c1 "00" -i T100 -p Y --periods "2000,2005,2010,2015,2020,2025"

# 특정 월만 비교
kosis d 101 DT_1DA7002S -c1 00 -i ALL -p M --periods "202401,202407,202501"
```

### 12.2.6 자연어 한 줄 조회

```bash
kosis q "서울 미분양 최근 5년"
kosis q "GDP 2020~2024"
kosis q "인구 2015,2020,2025"
kosis q "GDP 최근 5년" --ai claude
```

### 12.2.7 주요지표 조회

```bash
kosis ind s "GDP"                # 지표 검색
kosis ind info 160               # 상세 정보
kosis ind d "GDP" -o gdp지표.xlsx  # 수치 조회 + 저장
```

### 12.2.8 차트 시각화

```bash
# 터미널에서 바로 확인
kosis d 101 DT_1IN1502 -c1 26 -i T100 -p Y -l 10 --chart line

# PNG 이미지 저장
kosis d 101 DT_1IN1502 -c1 26 -i T100 -p Y -l 10 --chart line --chart-format png -o 울산인구.png

# HTML 인터랙티브 + 자동 열기
kosis d 101 DT_1IN1502 -c1 26 -i T100 -p Y -l 10 --chart line --chart-format html -o 울산인구.html --open

# 지역별 바 차트 비교
kosis d 101 DT_1IN1502 -c1 "11+26+27" -i T100 -p Y -l 1 --chart bar --chart-format png -o 지역비교.png

# Excel에 데이터+차트 함께
kosis d 101 DT_1DA7002S -c1 00 -i ALL -p M -l 12 --chart line --chart-format excel -o 경활추이.xlsx

# 파이프 방식
kosis d 301 DT_200Y101 -c1 ALL -i ALL -p Y -l 10 -f json | kosis chart --type line --title "GDP 추이"
kosis d 101 DT_1IN1502 -c1 26 -i T100 -p Y -l 10 -f json | kosis chart --type line --format pdf -o 울산.pdf
```

### 12.2.9 특정 필드만 출력

```bash
# API 키로 지정
kosis d 101 DT_1IN1502 -c1 "00+11" -i T100 -p Y -l 3 --fields "C1_NM,PRD_DE,DT"

# 한글 라벨로 지정
kosis d 101 DT_1IN1502 -c1 "00+11" -i T100 -p Y -l 3 --fields "시도별,시점,수치값"
```
