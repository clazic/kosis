# Module Context

`quick --ai`의 학습 저장소. 성공한 통계표·축 코드 선택을 기억해 다음 같은 키워드에 검색·Jev 호출 없이 재사용하고, 실행 오류가 난 통계표·주기·코드는 회피하며, 사용자가 확인 단계에서 교정한 정답을 저장한다.

# Storage

- 위치: `~/.kosis/learnings.yaml` (사용자 로컬, `kosis update`로 덮이지 않음). 초기화는 파일 삭제.
- 스킬의 `LEARNINGS.md`(사람·AI가 읽는 오답노트)와는 별개다. 이 파일은 CLI가 자동으로 읽고 쓴다.

# Testing Strategy

- `go test ./internal/learn`

# Local Golden Rules

Do:
- 경로는 `config` 디렉토리 기준으로 `filepath.Join`으로 만들고 크로스플랫폼을 고려한다.

Don't:
- 학습 저장소 읽기·쓰기 실패가 조회를 막게 하지 않는다 (실패는 무시하고 조회를 계속한다).
