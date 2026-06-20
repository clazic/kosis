# Module Context

이 패키지는 stdout 출력과 파일 저장을 담당한다. 사용자가 보는 표, JSON, CSV, XLSX, SQLite, Parquet 결과가 여기서 결정된다.

# Tech Stack & Constraints

- formatter abstraction
- file extension based dispatch
- XLSX, SQLite, Parquet writer

Constraints:
- stdout 포맷과 `WriteToFile` 저장 경로를 섞지 않는다.
- 확장자 감지 결과와 실제 저장 포맷이 일치해야 한다.
- 파이프/TTY 출력 차이는 table/json에서 유지한다.

# Implementation Patterns

- `DetectFormat`과 `WriteToFile`를 저장 경로 단일 진입점으로 유지한다.
- formatter 추가/수정 시 `formatter_test.go`와 파일 저장 통합 테스트를 같이 확인한다.
- unsupported extension은 조기 실패시키고, 부분 저장 상태를 남기지 않는다.

# Testing Strategy

- `go test ./internal/output`
- 필요 시 전체: `go test ./...`
- 확인 포인트:
  - `.json`, `.csv`, `.xlsx`, `.db`, `.parquet` 저장
  - 무효 확장자 실패
  - table/json의 TTY/비TTY 차이

# Local Golden Rules

Do:
- 파일 저장 변경 시 실제 파일 존재 여부까지 검증한다.
- 컬럼 순서와 한국어 헤더 계약을 함부로 바꾸지 않는다.

Don't:
- `--output` 경로를 stdout 출력만 바꾸는 식으로 구현하지 않는다.
- 포맷 지원 범위를 help와 다르게 남기지 않는다.
