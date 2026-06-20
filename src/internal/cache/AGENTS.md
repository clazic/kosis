# Module Context

이 패키지는 파일 기반 캐시를 관리한다. 검색, 메타, 목록 같은 반복 호출 성능과 사용자 체감 속도에 직접 영향을 준다.

# Tech Stack & Constraints

- 파일 시스템 기반 캐시
- SHA256 key hashing
- TTL 기반 만료

Constraints:
- 캐시 파일 쓰기는 원자성을 유지해야 한다.
- TTL 판정 기준은 파일 수정 시각 기반 정책을 임의로 바꾸지 않는다.
- 만료 파일 정리는 조회 성능을 과하게 해치지 않아야 한다.

# Implementation Patterns

- key는 해시 파일명으로 변환한다.
- `Set`은 임시 파일 후 rename 흐름을 유지한다.
- `Get`, `CleanExpired`, `Size`, `GetExpiredCount`는 같은 디렉토리 정책을 공유한다.

# Testing Strategy

- `go test ./internal/cache`
- 확인 포인트:
  - 캐시 쓰기/읽기
  - TTL 만료
  - Clear/CleanExpired 동작

# Local Golden Rules

Do:
- 파일 삭제 실패와 디렉토리 읽기 실패를 구분해 다룬다.
- 캐시 정책 변경 시 테스트 TTL 케이스를 같이 본다.

Don't:
- 만료 판단 정책을 조용히 바꾸지 않는다.
- atomic write 경로를 단순 `WriteFile` 하나로 축소하지 않는다.
