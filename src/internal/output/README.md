# output 패키지

KOSIS CLI의 데이터 포맷팅 및 내보내기 기능을 제공합니다.

## 지원 형식

- **table**: 컬러 테이블 (터미널용, 기본값)
- **json**: JSON 형식 (TTY: pretty-print, 파이프: compact)
- **csv**: CSV 형식 (UTF-8 BOM 포함, Excel 호환)

향후 지원 예정: xlsx, sqlite, parquet

## 주요 기능

### 1. Formatter 인터페이스

모든 포매터는 다음 인터페이스를 구현합니다:

```go
type Formatter interface {
    Format(data []map[string]interface{}, opts FormatOptions) error
}
```

### 2. FormatOptions 구조체

```go
type FormatOptions struct {
    // 표시할 컬럼 목록 (비어있으면 모든 컬럼)
    Columns []string

    // 한글 컬럼명 변환 여부
    Korean bool

    // 최대 행 수 (0 = 무제한)
    MaxRows int

    // 출력 대상 (기본: os.Stdout)
    Writer io.Writer
}
```

### 3. 컬럼명 매핑

`ColKorean` 맵에 40개 이상의 표준 KOSIS 컬럼명 매핑이 정의되어 있습니다:

- ORG_ID → 기관ID
- TBL_NM → 통계표명
- C1_NM ~ C8_NM → 분류값명1~8
- ITM_NM → 항목명
- DT → 수치값
- 등등...

## 사용 예시

### 1. 테이블 포맷팅

```go
data := []map[string]interface{}{
    {"ORG_ID": "101", "TBL_NM": "인구통계", "DT": "1234567"},
    {"ORG_ID": "102", "TBL_NM": "주택현황", "DT": "9876543"},
}

opts := FormatOptions{
    Writer: os.Stdout,
    Korean: true,
    MaxRows: 50,
}

formatter, err := NewFormatter("table")
if err != nil {
    log.Fatal(err)
}
if err := formatter.Format(data, opts); err != nil {
    log.Fatal(err)
}
```

### 2. JSON 포맷팅

```go
opts := FormatOptions{
    Writer: os.Stdout,
    Korean: true,
}

formatter, err := NewFormatter("json")
if err != nil {
    log.Fatal(err)
}
if err := formatter.Format(data, opts); err != nil {
    log.Fatal(err)
}
```

### 3. CSV 내보내기

```go
opts := FormatOptions{
    Columns: []string{"ORG_ID", "TBL_NM", "DT"},
    Korean: true,
}

formatter, err := NewFormatter("csv")
if err != nil {
    log.Fatal(err)
}
if err := formatter.Format(data, opts); err != nil {
    log.Fatal(err)
}
```

### 4. 파일로 저장

```go
// 자동 형식 감지 (확장자로)
if err := WriteToFile(data, "output.csv", opts); err != nil {
    log.Fatal(err)
}
```

## 주요 특징

### TableFormatter
- 터미널 너비 감지 (기본 120)
- 컬럼 자동 폭 조절 (최대 40자)
- 파이프 감지: TTY가 아니면 유니코드 테두리/컬러 비활성화
- 50행 초과 시 "... (총 N건 중 50건 표시)" 표시
- 구분선: ─ (유니코드) 또는 - (파이프 모드)
- 헤더: 볼드 (TTY일 때)

### JSONFormatter
- TTY: `json.MarshalIndent` (pretty print, 2칸 인덴트)
- 파이프: `json.Marshal` (compact)
- 한글 필드명 변환 지원

### CSVFormatter
- UTF-8 BOM (0xEF, 0xBB, 0xBF) 포함
- Excel 한글 호환성 보장
- 표준 `encoding/csv` 라이브러리 사용
- 기본적으로 한글 컬럼 헤더 사용

## 유틸리티 함수

### IsTTY(w io.Writer) bool

주어진 writer가 터미널에 연결되어 있는지 확인합니다.

```go
if IsTTY(os.Stdout) {
    // 터미널 출력 모드
} else {
    // 파이프 출력 모드
}
```

### DetectFormat(outputPath string) string

파일 확장자로부터 형식을 자동 감지합니다.

```go
format := DetectFormat("data.csv") // "csv"
format := DetectFormat("data.json") // "json"
```

### NewFormatter(format string) (Formatter, error)

형식 문자열에 따라 적절한 포매터를 반환합니다.

```go
formatter, err := NewFormatter("json")
if err != nil {
    log.Fatal(err)
}
```

## 에러 처리

모든 Format 메서드는 에러를 반환합니다. 에러 처리 예시:

```go
if err := formatter.Format(data, opts); err != nil {
    return fmt.Errorf("formatting failed: %w", err)
}
```

## 테스트

전체 테스트 실행:

```bash
go test -v ./internal/output
```

테스트 커버리지:

```bash
go test -cover ./internal/output
```

## 향후 계획

- xlsx: Excel 파일 (최대 100만행)
- sqlite: SQLite 데이터베이스 (대용량)
- parquet: Apache Parquet (데이터 과학용)
